// Package incus will list and control incus instances.
package incus

import (
	"context"

	"dashiboardie/internal/service"
)

type Provider struct{}

func New() *Provider { return &Provider{} }

func (p *Provider) Sources() []service.Source { return []service.Source{service.SourceIncus} }

func (p *Provider) List(ctx context.Context) ([]service.Service, error) {
	return nil, service.ErrNotImplemented
}

func (p *Provider) Do(ctx context.Context, id string, action service.Action) error {
	return service.ErrNotImplemented
}

func (p *Provider) Logs(ctx context.Context, id string, lines int) ([]string, error) {
	return nil, service.ErrNotImplemented
}
