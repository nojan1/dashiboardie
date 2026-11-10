// Package nixos will list and control declarative NixOS containers (container@<name>.service).
package nixos

import (
	"context"

	"dashiboardie/internal/service"
)

type Provider struct{}

func New() *Provider { return &Provider{} }

func (p *Provider) Sources() []service.Source { return []service.Source{service.SourceNixOS} }

func (p *Provider) List(ctx context.Context) ([]service.Service, error) {
	return nil, service.ErrNotImplemented
}

func (p *Provider) Do(ctx context.Context, id string, action service.Action) error {
	return service.ErrNotImplemented
}

func (p *Provider) Logs(ctx context.Context, id string, lines int) ([]string, error) {
	return nil, service.ErrNotImplemented
}
