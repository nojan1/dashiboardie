// Package nixos lists and controls NixOS containers. Every container (declarative
// or imperative) has a config at /etc/nixos-containers/<name>.conf and runs
// as the systemd unit container@<name>.service, so this is a thin layer over
// systemctl.
package nixos

import (
	"context"
	"io"
	"os"
	"sort"
	"strings"

	"dashiboardie/internal/service"
	"dashiboardie/internal/systemctl"
)

const DefaultConfigDir = "/etc/nixos-containers"

type Provider struct {
	sc  *systemctl.Client
	dir string
}

func New(sc *systemctl.Client, configDir string) *Provider {
	return &Provider{sc: sc, dir: configDir}
}

func (p *Provider) Sources() []service.Source { return []service.Source{service.SourceNixOS} }

func unitFor(name string) string { return "container@" + name + ".service" }

func (p *Provider) names() ([]string, error) {
	entries, err := os.ReadDir(p.dir)
	if os.IsNotExist(err) {
		return nil, nil // no NixOS containers on this host
	}
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if name, ok := strings.CutSuffix(e.Name(), ".conf"); ok && !e.IsDir() {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names, nil
}

func (p *Provider) List(ctx context.Context) ([]service.Service, error) {
	names, err := p.names()
	if err != nil || len(names) == 0 {
		return nil, err
	}
	units := make([]string, len(names))
	for i, n := range names {
		units[i] = unitFor(n)
	}
	shown, err := p.sc.Show(ctx, units...)
	if err != nil {
		return nil, err
	}
	byUnit := make(map[string]systemctl.Unit, len(shown))
	for _, u := range shown {
		byUnit[u.ID] = u
	}
	out := make([]service.Service, 0, len(names))
	for _, name := range names {
		u := byUnit[unitFor(name)]
		s := service.Service{
			ID:      name,
			Source:  service.SourceNixOS,
			Name:    name,
			Status:  u.Status(),
			Metrics: p.sc.Metrics(u),
			Unit:    unitFor(name),
		}
		s.StartedAt, s.Restarts = u.Uptime()
		out = append(out, s)
	}
	return out, nil
}

func (p *Provider) unit(id string) (string, error) {
	names, err := p.names()
	if err != nil {
		return "", err
	}
	for _, n := range names {
		if n == id {
			return unitFor(n), nil
		}
	}
	return "", service.ErrNotFound
}

func (p *Provider) Do(ctx context.Context, _ service.Source, id string, action service.Action) error {
	unit, err := p.unit(id)
	if err != nil {
		return err
	}
	return p.sc.Do(ctx, unit, action)
}

func (p *Provider) Logs(ctx context.Context, _ service.Source, id string, lines int) ([]string, error) {
	unit, err := p.unit(id)
	if err != nil {
		return nil, err
	}
	return p.sc.Journal(ctx, unit, lines)
}

func (p *Provider) FollowLogs(ctx context.Context, _ service.Source, id string, lines int) (io.ReadCloser, error) {
	unit, err := p.unit(id)
	if err != nil {
		return nil, err
	}
	return p.sc.FollowJournal(ctx, unit, lines)
}
