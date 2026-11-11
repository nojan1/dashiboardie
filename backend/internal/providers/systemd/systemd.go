// Package systemd lists and controls an explicitly configured allowlist of
// systemd units. Units outside the allowlist are never touched.
package systemd

import (
	"context"
	"fmt"
	"io"
	"strings"

	"dashiboardie/internal/service"
	"dashiboardie/internal/systemctl"
)

type Provider struct {
	sc    *systemctl.Client
	units []string
	urls  map[string]string // unit -> configured link
}

// New takes unit specs of the form "name" or "name=url". A name without a
// suffix gets ".service" added.
func New(sc *systemctl.Client, specs []string) *Provider {
	p := &Provider{sc: sc, urls: map[string]string{}}
	for _, spec := range specs {
		u, url, _ := strings.Cut(spec, "=")
		if !strings.Contains(u, ".") {
			u += ".service"
		}
		p.units = append(p.units, u)
		if url != "" {
			p.urls[u] = url
		}
	}
	return p
}

func (p *Provider) Sources() []service.Source { return []service.Source{service.SourceSystemd} }

func (p *Provider) List(ctx context.Context) ([]service.Service, error) {
	units, err := p.sc.Show(ctx, p.units...)
	if err != nil {
		return nil, err
	}
	if len(units) != len(p.units) {
		return nil, fmt.Errorf("systemctl show returned %d units, expected %d", len(units), len(p.units))
	}
	out := make([]service.Service, 0, len(units))
	for i, u := range units {
		// `systemctl show` answers in request order. Keep the configured name
		// as the ID (it's what the allowlist checks), since an alias reports
		// the canonical unit as u.ID.
		name := p.units[i]
		s := service.Service{
			ID:     name,
			Source: service.SourceSystemd,
			Name:   strings.TrimSuffix(name, ".service"),
			Status: u.Status(),
			Unit:   u.ID,
			URL:    p.urls[name],
		}
		if u.LoadState == "not-found" {
			s.Status = service.StatusUnknown
		} else {
			s.Metrics = p.sc.Metrics(u)
			s.StartedAt, s.Restarts = u.Uptime()
		}
		out = append(out, s)
	}
	return out, nil
}

func (p *Provider) allowed(id string) bool {
	for _, u := range p.units {
		if u == id {
			return true
		}
	}
	return false
}

func (p *Provider) Do(ctx context.Context, _ service.Source, id string, action service.Action) error {
	if !p.allowed(id) {
		return service.ErrNotFound
	}
	return p.sc.Do(ctx, id, action)
}

func (p *Provider) Logs(ctx context.Context, _ service.Source, id string, lines int) ([]string, error) {
	if !p.allowed(id) {
		return nil, service.ErrNotFound
	}
	return p.sc.Journal(ctx, id, lines)
}

func (p *Provider) FollowLogs(ctx context.Context, _ service.Source, id string, lines int) (io.ReadCloser, error) {
	if !p.allowed(id) {
		return nil, service.ErrNotFound
	}
	return p.sc.FollowJournal(ctx, id, lines)
}
