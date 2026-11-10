// Package systemd will list and control an explicitly configured allowlist of systemd units.
package systemd

import (
	"context"

	"dashiboardie/internal/service"
)

type Provider struct {
	units []string
}

func New(units []string) *Provider { return &Provider{units: units} }

func (p *Provider) Sources() []service.Source { return []service.Source{service.SourceSystemd} }

func (p *Provider) List(ctx context.Context) ([]service.Service, error) {
	return nil, service.ErrNotImplemented
}

func (p *Provider) Do(ctx context.Context, id string, action service.Action) error {
	return service.ErrNotImplemented
}

func (p *Provider) Logs(ctx context.Context, id string, lines int) ([]string, error) {
	return nil, service.ErrNotImplemented
}
