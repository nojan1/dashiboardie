// Package systemd lists and controls an explicitly configured allowlist of
// systemd units. Units outside the allowlist are never touched.
package systemd

import (
	"context"
	"strings"

	"dashiboardie/internal/service"
	"dashiboardie/internal/systemctl"
)

type Provider struct {
	sc    *systemctl.Client
	units []string
}

func New(sc *systemctl.Client, units []string) *Provider {
	normalized := make([]string, len(units))
	for i, u := range units {
		if !strings.Contains(u, ".") {
			u += ".service"
		}
		normalized[i] = u
	}
	return &Provider{sc: sc, units: normalized}
}

func (p *Provider) Sources() []service.Source { return []service.Source{service.SourceSystemd} }

func (p *Provider) List(ctx context.Context) ([]service.Service, error) {
	units, err := p.sc.Show(ctx, p.units...)
	if err != nil {
		return nil, err
	}
	out := make([]service.Service, 0, len(units))
	for _, u := range units {
		s := service.Service{
			ID:     u.ID,
			Source: service.SourceSystemd,
			Name:   strings.TrimSuffix(u.ID, ".service"),
			Status: u.Status(),
			Unit:   u.ID,
		}
		if u.LoadState == "not-found" {
			s.Status = service.StatusUnknown
		} else {
			s.Metrics = p.sc.Metrics(u)
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
