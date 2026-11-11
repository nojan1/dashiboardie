// Package mock provides in-memory providers for frontend development
// (enable with -mock), so the UI can be exercised without podman, incus,
// etc. on the dev machine. All() returns one fake provider per real source.
package mock

import (
	"context"
	"fmt"
	"sync"
	"time"

	"dashiboardie/internal/service"
)

type Provider struct {
	src  service.Source
	mu   sync.Mutex
	svcs []service.Service
}

func ptr[T any](v T) *T { return &v }

func metrics(cpu float64, memMiB uint64) *service.Metrics {
	return &service.Metrics{CPUPercent: ptr(cpu), MemoryBytes: ptr(memMiB << 20)}
}

// All returns a fake provider for every real source.
func All() []service.Provider {
	return []service.Provider{
		newProvider(service.SourceQuadlet,
			service.Service{ID: "jellyfin", Name: "jellyfin", Status: service.StatusRunning, Unit: "jellyfin.service", Metrics: metrics(3.2, 512), URL: "https://jellyfin.example.com"},
		),
		newProvider(service.SourcePodman,
			service.Service{ID: "redis", Name: "redis", Status: service.StatusRunning, Metrics: metrics(0.4, 24)},
			service.Service{ID: "bazarr", Name: "bazarr", Status: service.StatusRunning, Metrics: metrics(1.1, 180), URL: "http://bazarr.home"},
		),
		newProvider(service.SourceIncus,
			service.Service{ID: "dev-vm", Name: "dev-vm", Status: service.StatusStopped},
			service.Service{ID: "homeassistant", Name: "homeassistant", Status: service.StatusRunning, Address: "192.168.1.104"},
		),
		newProvider(service.SourceNixOS,
			service.Service{ID: "builder", Name: "builder", Status: service.StatusRunning},
		),
		newProvider(service.SourceSystemd,
			service.Service{ID: "nginx.service", Name: "nginx", Status: service.StatusRunning, Unit: "nginx.service", URL: "https://nginx.example.com/status"},
			service.Service{ID: "backup.service", Name: "backup", Status: service.StatusFailed, Unit: "backup.service"},
			// Duplicate of the Quadlet above; the registry should hide it.
			service.Service{ID: "jellyfin.service", Name: "jellyfin", Status: service.StatusRunning, Unit: "jellyfin.service"},
		),
	}
}

func newProvider(src service.Source, svcs ...service.Service) *Provider {
	for i := range svcs {
		svcs[i].Source = src
	}
	return &Provider{src: src, svcs: svcs}
}

func (p *Provider) Sources() []service.Source { return []service.Source{p.src} }

func (p *Provider) List(ctx context.Context) ([]service.Service, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]service.Service(nil), p.svcs...), nil
}

func (p *Provider) Do(ctx context.Context, _ service.Source, id string, action service.Action) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	for i := range p.svcs {
		if p.svcs[i].ID != id {
			continue
		}
		switch action {
		case service.ActionStart, service.ActionRestart:
			p.svcs[i].Status = service.StatusRunning
		case service.ActionStop:
			p.svcs[i].Status = service.StatusStopped
		}
		return nil
	}
	return service.ErrNotFound
}

func (p *Provider) Logs(ctx context.Context, _ service.Source, id string, lines int) ([]string, error) {
	now := time.Now()
	n := min(lines, 20)
	out := make([]string, n)
	for i := range n {
		ts := now.Add(time.Duration(i-n) * time.Second).Format(time.RFC3339)
		out[i] = fmt.Sprintf("%s %s: mock log line %d", ts, id, i+1)
	}
	return out, nil
}
