// Package podman will list and control podman containers. Containers managed by a Quadlet
// should be reported with Source quadlet and Unit set to their systemd unit;
// lifecycle actions for those should go through systemd.
package podman

import (
	"context"

	"dashiboardie/internal/service"
)

type Provider struct{}

func New() *Provider { return &Provider{} }

func (p *Provider) Sources() []service.Source {
	return []service.Source{service.SourcePodman, service.SourceQuadlet}
}

func (p *Provider) List(ctx context.Context) ([]service.Service, error) {
	return nil, service.ErrNotImplemented
}

func (p *Provider) Do(ctx context.Context, id string, action service.Action) error {
	return service.ErrNotImplemented
}

func (p *Provider) Logs(ctx context.Context, id string, lines int) ([]string, error) {
	return nil, service.ErrNotImplemented
}
