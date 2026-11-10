// Package incus lists and controls incus instances (containers and VMs) in
// the default project.
package incus

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"dashiboardie/internal/execx"
	"dashiboardie/internal/service"
	"dashiboardie/internal/systemctl"
)

type Provider struct {
	run execx.Runner
	cpu systemctl.CPUSampler
}

func New(run execx.Runner) *Provider { return &Provider{run: run} }

func (p *Provider) Sources() []service.Source { return []service.Source{service.SourceIncus} }

// instance is the subset of `incus list --format json` we use.
type instance struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Type   string `json:"type"`
	State  *struct {
		CPU struct {
			Usage uint64 `json:"usage"` // cumulative nanoseconds
		} `json:"cpu"`
		Memory struct {
			Usage uint64 `json:"usage"` // bytes
		} `json:"memory"`
	} `json:"state"`
}

func (p *Provider) List(ctx context.Context) ([]service.Service, error) {
	out, err := p.run.Output(ctx, "incus", "list", "--format", "json")
	if err != nil {
		return nil, err
	}
	var instances []instance
	if err := json.Unmarshal(out, &instances); err != nil {
		return nil, err
	}
	now := time.Now()
	svcs := make([]service.Service, 0, len(instances))
	for _, in := range instances {
		s := service.Service{
			ID:     in.Name,
			Source: service.SourceIncus,
			Name:   in.Name,
			Status: instanceStatus(in.Status),
		}
		if in.State != nil && s.Status == service.StatusRunning {
			mem := in.State.Memory.Usage
			s.Metrics = &service.Metrics{
				CPUPercent:  p.cpu.Percent(in.Name, in.State.CPU.Usage, now),
				MemoryBytes: &mem,
			}
		}
		svcs = append(svcs, s)
	}
	return svcs, nil
}

func instanceStatus(s string) service.Status {
	switch strings.ToLower(s) {
	case "running":
		return service.StatusRunning
	case "stopped":
		return service.StatusStopped
	case "error":
		return service.StatusFailed
	default: // frozen, starting, stopping, ...
		return service.StatusUnknown
	}
}

func (p *Provider) find(ctx context.Context, id string) error {
	svcs, err := p.List(ctx)
	if err != nil {
		return err
	}
	_, err = service.Find(svcs, service.SourceIncus, id)
	return err
}

func (p *Provider) Do(ctx context.Context, _ service.Source, id string, action service.Action) error {
	if err := p.find(ctx, id); err != nil {
		return err
	}
	_, err := p.run.Output(ctx, "incus", string(action), "--", id)
	return err
}

// Logs returns the instance console log, falling back to the instance's
// incus log (`incus info --show-log`) if the console log is unavailable.
func (p *Provider) Logs(ctx context.Context, _ service.Source, id string, lines int) ([]string, error) {
	if err := p.find(ctx, id); err != nil {
		return nil, err
	}
	out, err := p.run.Output(ctx, "incus", "console", "--show-log", "--", id)
	if err != nil {
		info, ierr := p.run.Output(ctx, "incus", "info", "--show-log", "--", id)
		if ierr != nil {
			return nil, err
		}
		out = afterLogHeader(info)
	}
	return tail(execx.Lines(out), lines), nil
}

// afterLogHeader strips the instance info that precedes the "Log:" section.
func afterLogHeader(out []byte) []byte {
	s := string(out)
	if i := strings.Index(s, "\nLog:\n"); i >= 0 {
		return []byte(strings.TrimLeft(s[i+len("\nLog:\n"):], "\n"))
	}
	return out
}

func tail(lines []string, n int) []string {
	if len(lines) > n {
		return lines[len(lines)-n:]
	}
	return lines
}
