package service

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestDedupeQuadlets(t *testing.T) {
	in := []Service{
		{ID: "jellyfin", Source: SourceQuadlet, Unit: "jellyfin.service"},
		{ID: "jellyfin.service", Source: SourceSystemd, Unit: "jellyfin.service"},
		{ID: "nginx.service", Source: SourceSystemd, Unit: "nginx.service"},
		{ID: "redis", Source: SourcePodman},
	}
	got := dedupeQuadlets(in)
	if len(got) != 3 {
		t.Fatalf("want 3 services, got %d: %+v", len(got), got)
	}
	for _, s := range got {
		if s.Source == SourceSystemd && s.Unit == "jellyfin.service" {
			t.Fatalf("systemd duplicate of quadlet was not removed")
		}
	}
}

// countingProvider counts List calls and blocks each one until release is
// closed, so tests can hold a load in flight.
type countingProvider struct {
	calls   atomic.Int32
	release chan struct{}
}

func (p *countingProvider) Sources() []Source { return []Source{SourceMock} }
func (p *countingProvider) List(ctx context.Context) ([]Service, error) {
	p.calls.Add(1)
	<-p.release
	return []Service{{ID: "a", Source: SourceMock, Name: "a"}}, nil
}
func (p *countingProvider) Do(context.Context, Source, string, Action) error { return nil }
func (p *countingProvider) Logs(context.Context, Source, string, int) ([]string, error) {
	return nil, nil
}

func TestListSharesInflightAndCaches(t *testing.T) {
	p := &countingProvider{release: make(chan struct{})}
	r := NewRegistry(p)
	ctx := context.Background()

	var wg sync.WaitGroup
	for range 5 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if svcs, _ := r.List(ctx); len(svcs) != 1 {
				t.Errorf("want 1 service, got %d", len(svcs))
			}
		}()
	}
	// Let all five callers reach the in-flight load before releasing it.
	for p.calls.Load() == 0 {
		runtime.Gosched()
	}
	time.Sleep(50 * time.Millisecond)
	close(p.release)
	wg.Wait()
	if n := p.calls.Load(); n != 1 {
		t.Fatalf("concurrent List calls should share one load, provider called %d times", n)
	}

	r.List(ctx) // served from cache
	if n := p.calls.Load(); n != 1 {
		t.Fatalf("cached List should not call provider, called %d times", n)
	}

	if err := r.Do(ctx, SourceMock, "a", ActionRestart); err != nil {
		t.Fatal(err)
	}
	r.List(ctx) // cache invalidated by the action
	if n := p.calls.Load(); n != 2 {
		t.Fatalf("List after an action should reload, provider called %d times", n)
	}
}

func TestListReturnsWhenCallerGivesUp(t *testing.T) {
	p := &countingProvider{release: make(chan struct{})}
	defer close(p.release)
	r := NewRegistry(p)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if svcs, errs := r.List(ctx); svcs != nil || errs != nil {
		t.Fatalf("want nil result after ctx timeout, got %v %v", svcs, errs)
	}
}
