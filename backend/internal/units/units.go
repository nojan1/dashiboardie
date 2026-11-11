// Package units reports host-wide systemd state that isn't a "service" row:
// units in the failed state (all of them, not just the -systemd-units
// allowlist) and a configured list of timers with their last and next runs.
//
// Logs can be read for exactly those units: failed units, the configured
// timers and the services they trigger. The API is unauthenticated, so the
// unit name from the client is checked against a fresh overview first.
package units

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"dashiboardie/internal/service"
	"dashiboardie/internal/systemctl"
)

// Source is what the API needs; the mock package provides a fake one.
type Source interface {
	Overview(ctx context.Context) (Overview, error)
	Logs(ctx context.Context, unit string, lines int) ([]string, error)
	FollowLogs(ctx context.Context, unit string, lines int) (io.ReadCloser, error)
}

type Overview struct {
	Failed []systemctl.FailedUnit `json:"failed"`
	Timers []Timer                `json:"timers"`
}

type Timer struct {
	// Name is as configured (without .timer), Unit the timer unit.
	Name        string `json:"name"`
	Unit        string `json:"unit"`
	Description string `json:"description,omitempty"`
	// Service is the unit the timer triggers (logs are read from it).
	Service string `json:"service,omitempty"`
	// Found is false when no such timer is loaded.
	Found bool `json:"found"`
	// Active is false when the timer is stopped, so it won't fire.
	Active  bool       `json:"active"`
	LastRun *time.Time `json:"lastRun,omitempty"`
	NextRun *time.Time `json:"nextRun,omitempty"`
	// Running means the triggered service is running right now.
	Running bool `json:"running"`
	// Result is the triggered service's last result: "success", or
	// systemd's failure reason ("exit-code", "signal", "timeout", ...).
	// Empty if it hasn't run since boot: systemd keeps a service's result
	// in memory only, and a Persistent= timer reports a LastRun from its
	// stamp file even when the job never ran (the stamp is created when the
	// timer first appears).
	Result string `json:"result,omitempty"`
	// ExitStatus is the main process's exit code of the last run.
	ExitStatus *int `json:"exitStatus,omitempty"`
}

type Watcher struct {
	sc     *systemctl.Client
	timers []string // timer unit names
}

// New takes timer names, with or without the .timer suffix.
func New(sc *systemctl.Client, timers []string) *Watcher {
	w := &Watcher{sc: sc}
	for _, t := range timers {
		if !strings.HasSuffix(t, ".timer") {
			t += ".timer"
		}
		w.timers = append(w.timers, t)
	}
	return w
}

const (
	timerProps   = "Id,Description,LoadState,ActiveState,Unit"
	serviceProps = "Id,ActiveState,Result,ExecMainStatus,ExecMainStartTimestamp"
)

func (w *Watcher) Overview(ctx context.Context) (Overview, error) {
	failed, err := w.sc.FailedUnits(ctx)
	if err != nil {
		return Overview{}, err
	}
	timers, err := w.Timers(ctx)
	if err != nil {
		return Overview{}, err
	}
	if failed == nil {
		failed = []systemctl.FailedUnit{}
	}
	return Overview{Failed: failed, Timers: timers}, nil
}

func (w *Watcher) Timers(ctx context.Context) ([]Timer, error) {
	out := []Timer{}
	if len(w.timers) == 0 {
		return out, nil
	}
	shown, err := w.sc.ShowProps(ctx, timerProps, w.timers...)
	if err != nil {
		return nil, err
	}
	schedule, err := w.schedule(ctx)
	if err != nil {
		return nil, err
	}
	var services []string
	for i, m := range shown {
		if i >= len(w.timers) {
			break
		}
		t := Timer{
			Name:        strings.TrimSuffix(w.timers[i], ".timer"),
			Unit:        w.timers[i],
			Description: m["Description"],
			Found:       m["LoadState"] == "loaded",
			Active:      m["ActiveState"] == "active",
			Service:     m["Unit"],
			LastRun:     schedule[w.timers[i]].last,
			NextRun:     schedule[w.timers[i]].next,
		}
		if t.Description == t.Unit {
			t.Description = "" // systemd's fallback; nothing to add
		}
		out = append(out, t)
		if t.Found && t.Service != "" {
			services = append(services, t.Service)
		}
	}

	svcs, err := w.sc.ShowProps(ctx, serviceProps, services...)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]map[string]string, len(svcs))
	for _, m := range svcs {
		byID[m["Id"]] = m
	}
	for i := range out {
		m, ok := byID[out[i].Service]
		if !ok {
			continue
		}
		state := m["ActiveState"]
		out[i].Running = state == "active" || state == "activating" || state == "reloading"
		// systemd says Result=success for a service that never ran, so only
		// trust it when the service started this boot. A failure result is
		// real even without a start time (e.g. its ExecStart= didn't exist).
		started := systemctl.ParseTimestamp(m["ExecMainStartTimestamp"]) != nil
		if started || (m["Result"] != "" && m["Result"] != "success") {
			out[i].Result = m["Result"]
			if n, err := strconv.Atoi(m["ExecMainStatus"]); err == nil {
				out[i].ExitStatus = &n
			}
		}
	}
	return out, nil
}

// allowed reports whether logs of unit may be read: it must be failed right
// now, or be one of the configured timers or the service a timer triggers.
func (w *Watcher) allowed(ctx context.Context, unit string) error {
	for _, t := range w.timers {
		if t == unit {
			return nil
		}
	}
	ov, err := w.Overview(ctx)
	if err != nil {
		return err
	}
	for _, t := range ov.Timers {
		if t.Service != "" && t.Service == unit {
			return nil
		}
	}
	for _, f := range ov.Failed {
		if f.Unit == unit {
			return nil
		}
	}
	return service.ErrNotFound
}

func (w *Watcher) Logs(ctx context.Context, unit string, lines int) ([]string, error) {
	if err := w.allowed(ctx, unit); err != nil {
		return nil, err
	}
	return w.sc.Journal(ctx, unit, lines)
}

func (w *Watcher) FollowLogs(ctx context.Context, unit string, lines int) (io.ReadCloser, error) {
	if err := w.allowed(ctx, unit); err != nil {
		return nil, err
	}
	return w.sc.FollowJournal(ctx, unit, lines)
}

type runTimes struct{ last, next *time.Time }

// schedule gets last and next run times from `systemctl list-timers`.
// `systemctl show` can't provide them for every timer: NextElapseUSecRealtime
// is only set for OnCalendar= timers, and monotonic timers (OnBootSec=,
// OnUnitActiveSec=) report a time since boot instead. list-timers converts
// both to wall-clock microseconds.
func (w *Watcher) schedule(ctx context.Context) (map[string]runTimes, error) {
	out, err := w.sc.Run.Output(ctx, "systemctl", append([]string{"list-timers", "--all", "--no-pager", "-o", "json", "--"}, w.timers...)...)
	if err != nil {
		return nil, err
	}
	var entries []struct {
		Unit string `json:"unit"`
		Next *int64 `json:"next"`
		Last *int64 `json:"last"`
	}
	if err := json.Unmarshal(out, &entries); err != nil {
		return nil, fmt.Errorf("parse list-timers: %w", err)
	}
	usec := func(v *int64) *time.Time {
		if v == nil || *v <= 0 {
			return nil
		}
		t := time.UnixMicro(*v).UTC()
		return &t
	}
	m := make(map[string]runTimes, len(entries))
	for _, e := range entries {
		m[e.Unit] = runTimes{last: usec(e.Last), next: usec(e.Next)}
	}
	return m, nil
}
