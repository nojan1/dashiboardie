package service

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"
)

const (
	// listTimeout bounds each provider so one hung CLI cannot stall the dashboard.
	listTimeout = 10 * time.Second
	// cacheTTL lets several browser tabs (or a burst of refreshes) share one
	// round of CLI calls instead of each starting their own.
	cacheTTL = 2 * time.Second
	// slowProvider is when a provider's List gets logged as slow.
	slowProvider = 2 * time.Second
)

// Registry aggregates all providers into a single list.
type Registry struct {
	all       []Provider
	providers map[Source]Provider // routing for actions and logs

	mu       sync.Mutex
	inflight *listing // the load currently running, if any
	cached   *listing // the last finished load
}

type listing struct {
	done     chan struct{}
	svcs     []Service
	errs     []ProviderError
	finished time.Time
}

func NewRegistry(providers ...Provider) *Registry {
	r := &Registry{all: providers, providers: make(map[Source]Provider)}
	for _, p := range providers {
		for _, src := range p.Sources() {
			r.providers[src] = p
		}
	}
	return r
}

// ProviderError reports that one source failed to list; the others still show.
type ProviderError struct {
	Source Source `json:"source"`
	Error  string `json:"error"`
}

// List returns the merged service list. Concurrent callers share a single
// in-flight load, and a load younger than cacheTTL is reused. If ctx ends
// first, List returns nil without cancelling the shared load.
func (r *Registry) List(ctx context.Context) ([]Service, []ProviderError) {
	r.mu.Lock()
	if c := r.cached; c != nil && time.Since(c.finished) < cacheTTL {
		r.mu.Unlock()
		return c.svcs, c.errs
	}
	l := r.inflight
	if l == nil {
		l = &listing{done: make(chan struct{})}
		r.inflight = l
		go r.load(l)
	}
	r.mu.Unlock()

	select {
	case <-l.done:
		return l.svcs, l.errs
	case <-ctx.Done():
		return nil, nil
	}
}

// invalidate drops the cached list so the next List sees fresh state (used
// after an action changes something). A load already in flight may predate
// the action, so it is detached too: its waiters still get its result, but it
// won't be cached or handed to new callers.
func (r *Registry) invalidate() {
	r.mu.Lock()
	r.cached = nil
	r.inflight = nil
	r.mu.Unlock()
}

func (r *Registry) load(l *listing) {
	l.svcs, l.errs = r.listAll(context.Background())
	l.finished = time.Now()
	r.mu.Lock()
	if r.inflight == l { // not detached by invalidate
		r.inflight = nil
		r.cached = l
	}
	r.mu.Unlock()
	close(l.done)
}

// listAll queries every provider concurrently, merges the results, and removes
// systemd entries whose unit is already represented by a Quadlet service.
func (r *Registry) listAll(ctx context.Context) ([]Service, []ProviderError) {
	type result struct {
		src  Source
		svcs []Service
		err  error
	}
	results := make(chan result, len(r.all))
	var wg sync.WaitGroup
	for _, p := range r.all {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(ctx, listTimeout)
			defer cancel()
			start := time.Now()
			svcs, err := p.List(ctx)
			if d := time.Since(start); d > slowProvider {
				slog.Warn("slow provider", "source", p.Sources()[0], "duration", d.Round(time.Millisecond), "err", err)
			}
			results <- result{p.Sources()[0], svcs, err}
		}()
	}
	wg.Wait()
	close(results)

	var all []Service
	var errs []ProviderError
	for res := range results {
		if res.err != nil {
			errs = append(errs, ProviderError{Source: res.src, Error: res.err.Error()})
			continue
		}
		all = append(all, res.svcs...)
	}

	all = dedupeQuadlets(all)
	sort.Slice(all, func(i, j int) bool {
		if all[i].Source != all[j].Source {
			return all[i].Source < all[j].Source
		}
		return all[i].Name < all[j].Name
	})
	sort.Slice(errs, func(i, j int) bool { return errs[i].Source < errs[j].Source })
	return all, errs
}

func dedupeQuadlets(svcs []Service) []Service {
	quadletUnits := make(map[string]bool)
	for _, s := range svcs {
		if s.Source == SourceQuadlet && s.Unit != "" {
			quadletUnits[s.Unit] = true
		}
	}
	out := svcs[:0]
	for _, s := range svcs {
		if s.Source == SourceSystemd && quadletUnits[s.Unit] {
			continue
		}
		out = append(out, s)
	}
	return out
}

func (r *Registry) provider(src Source) (Provider, error) {
	p, ok := r.providers[src]
	if !ok {
		return nil, fmt.Errorf("unknown source %q: %w", src, ErrNotFound)
	}
	return p, nil
}

func (r *Registry) Do(ctx context.Context, src Source, id string, action Action) error {
	p, err := r.provider(src)
	if err != nil {
		return err
	}
	err = p.Do(ctx, src, id, action)
	r.invalidate()
	return err
}

func (r *Registry) Logs(ctx context.Context, src Source, id string, lines int) ([]string, error) {
	p, err := r.provider(src)
	if err != nil {
		return nil, err
	}
	return p.Logs(ctx, src, id, lines)
}
