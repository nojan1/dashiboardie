// Package podman lists and controls podman containers and Quadlets.
//
// Quadlets are discovered through systemd, not `podman ps`: Quadlet units
// run their containers with --rm, so a stopped Quadlet has no container.
// A Quadlet is a generated service unit whose SourcePath is a Quadlet file.
// Containers whose PODMAN_SYSTEMD_UNIT label points at a Quadlet unit are
// hidden from the plain podman list. The label alone does not mean a
// container is a Quadlet, because podman-compose sets it too.
//
// The provider sees whichever podman instance its user owns: rootful as root,
// rootless (with the user's systemd manager) otherwise.
package podman

import (
	"context"
	"encoding/json"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"

	"dashiboardie/internal/execx"
	"dashiboardie/internal/service"
	"dashiboardie/internal/systemctl"
)

// quadletExts are the Quadlet file types that produce a long-running service.
var quadletExts = []string{".container", ".pod", ".kube"}

type Provider struct {
	run execx.Runner
	sc  *systemctl.Client
	cpu systemctl.CPUSampler

	mu      sync.Mutex
	updates updateSet // from the last successful CheckUpdates
}

func New(run execx.Runner, sc *systemctl.Client) *Provider {
	return &Provider{run: run, sc: sc}
}

func (p *Provider) Sources() []service.Source {
	return []service.Source{service.SourcePodman, service.SourceQuadlet}
}

func (p *Provider) List(ctx context.Context) ([]service.Service, error) {
	return p.list(ctx, true)
}

// list collects quadlets and containers. withStats=false skips the slow
// `podman stats` call; Do and Logs only need the list to validate ids.
func (p *Provider) list(ctx context.Context, withStats bool) ([]service.Service, error) {
	quadlets, err := p.quadlets(ctx)
	if err != nil {
		return nil, err
	}
	containers, err := p.containers(ctx, quadlets, withStats)
	if err != nil {
		return nil, err
	}
	return append(quadlets, containers...), nil
}

func (p *Provider) quadlets(ctx context.Context) ([]service.Service, error) {
	generated, err := p.sc.GeneratedServices()
	if err != nil {
		return nil, err
	}
	var names []string
	urls := map[string]string{}
	for _, g := range generated {
		if !isQuadletSource(g.SourcePath) {
			continue
		}
		names = append(names, g.Name)
		// Links are best effort: an unreadable unit just gets no URL.
		if labels, err := quadletLabels(g.Path); err == nil {
			urls[g.Name] = traefikURL(labels)
		}
	}
	units, err := p.sc.Show(ctx, names...)
	if err != nil {
		return nil, err
	}
	updates := p.currentUpdates()
	var out []service.Service
	for _, u := range units {
		s := service.Service{
			ID:              u.ID,
			Source:          service.SourceQuadlet,
			Name:            strings.TrimSuffix(u.ID, ".service"),
			Status:          u.Status(),
			Metrics:         p.sc.Metrics(u),
			Unit:            u.ID,
			URL:             urls[u.ID],
			UpdateAvailable: updates.units[u.ID],
		}
		s.StartedAt, s.Restarts = u.Uptime()
		out = append(out, s)
	}
	return out, nil
}

func isQuadletSource(path string) bool {
	for _, ext := range quadletExts {
		if strings.HasSuffix(path, ext) {
			return true
		}
	}
	return false
}

// psEntry is the subset of `podman ps --format json` we use.
type psEntry struct {
	ID        string            `json:"Id"`
	Names     []string          `json:"Names"`
	State     string            `json:"State"`
	IsInfra   bool              `json:"IsInfra"`
	Labels    map[string]string `json:"Labels"`
	StartedAt int64             `json:"StartedAt"` // unix seconds
	Restarts  *int              `json:"Restarts"`
}

// statsEntry is `podman stats --format '{{json .ContainerStats}}'`, which
// (unlike --format json) gives raw numbers instead of formatted strings.
type statsEntry struct {
	ContainerID string `json:"ContainerID"`
	CPUNano     uint64 `json:"CPUNano"`
	MemUsage    uint64 `json:"MemUsage"`
}

func (p *Provider) containers(ctx context.Context, quadlets []service.Service, withStats bool) ([]service.Service, error) {
	out, err := p.run.Output(ctx, "podman", "ps", "--all", "--format", "json")
	if err != nil {
		return nil, err
	}
	var entries []psEntry
	if err := json.Unmarshal(out, &entries); err != nil {
		return nil, err
	}

	quadletUnits := make(map[string]bool, len(quadlets))
	for _, q := range quadlets {
		quadletUnits[q.Unit] = true
	}

	var svcs []service.Service
	anyRunning := false
	ids := make(map[string]int) // container ID -> index in svcs
	updates := p.currentUpdates()
	for _, e := range entries {
		if e.IsInfra || len(e.Names) == 0 || quadletUnits[e.Labels["PODMAN_SYSTEMD_UNIT"]] {
			continue
		}
		s := service.Service{
			ID:              e.Names[0],
			Source:          service.SourcePodman,
			Name:            e.Names[0],
			Status:          containerStatus(e.State),
			Unit:            e.Labels["PODMAN_SYSTEMD_UNIT"],
			URL:             traefikURL(e.Labels),
			Restarts:        e.Restarts,
			UpdateAvailable: updates.containers[e.Names[0]],
		}
		if s.Status == service.StatusRunning {
			anyRunning = true
			if e.StartedAt > 0 {
				t := time.Unix(e.StartedAt, 0).UTC()
				s.StartedAt = &t
			}
		}
		ids[e.ID] = len(svcs)
		svcs = append(svcs, s)
	}

	if withStats && anyRunning {
		// Metrics are best effort: a stats failure shouldn't hide the list.
		if stats, err := p.stats(ctx); err == nil {
			now := time.Now()
			for _, st := range stats {
				i, ok := ids[st.ContainerID]
				if !ok {
					continue
				}
				mem := st.MemUsage
				svcs[i].Metrics = &service.Metrics{
					CPUPercent:  p.cpu.Percent(st.ContainerID, st.CPUNano, now),
					MemoryBytes: &mem,
				}
			}
		}
	}
	return svcs, nil
}

func (p *Provider) stats(ctx context.Context) ([]statsEntry, error) {
	out, err := p.run.Output(ctx, "podman", "stats", "--no-stream", "--format", "{{json .ContainerStats}}")
	if err != nil {
		return nil, err
	}
	var stats []statsEntry
	for _, line := range execx.Lines(out) {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var st statsEntry
		if err := json.Unmarshal([]byte(line), &st); err != nil {
			return nil, err
		}
		stats = append(stats, st)
	}
	return stats, nil
}

func containerStatus(state string) service.Status {
	switch state {
	case "running":
		return service.StatusRunning
	case "exited", "stopped", "created", "configured", "initialized":
		return service.StatusStopped
	default: // paused, removing, stopping, unknown
		return service.StatusUnknown
	}
}

// find validates a client-supplied id against the current list.
func (p *Provider) find(ctx context.Context, src service.Source, id string) (service.Service, error) {
	svcs, err := p.list(ctx, false)
	if err != nil {
		return service.Service{}, err
	}
	return service.Find(svcs, src, id)
}

func (p *Provider) Do(ctx context.Context, src service.Source, id string, action service.Action) error {
	s, err := p.find(ctx, src, id)
	if err != nil {
		return err
	}
	if s.Source == service.SourceQuadlet {
		return p.sc.Do(ctx, s.Unit, action)
	}
	_, err = p.run.Output(ctx, "podman", string(action), "--", s.ID)
	return err
}

func (p *Provider) Logs(ctx context.Context, src service.Source, id string, lines int) ([]string, error) {
	s, err := p.find(ctx, src, id)
	if err != nil {
		return nil, err
	}
	if s.Source == service.SourceQuadlet {
		return p.sc.Journal(ctx, s.Unit, lines)
	}
	out, err := p.run.Combined(ctx, "podman", "logs", "--timestamps", "--tail", strconv.Itoa(lines), "--", s.ID)
	if err != nil {
		return nil, err
	}
	return execx.Lines(out), nil
}

func (p *Provider) FollowLogs(ctx context.Context, src service.Source, id string, lines int) (io.ReadCloser, error) {
	s, err := p.find(ctx, src, id)
	if err != nil {
		return nil, err
	}
	if s.Source == service.SourceQuadlet {
		return p.sc.FollowJournal(ctx, s.Unit, lines)
	}
	return p.run.Stream(ctx, "podman", "logs", "--follow", "--timestamps", "--tail", strconv.Itoa(lines), "--", s.ID)
}
