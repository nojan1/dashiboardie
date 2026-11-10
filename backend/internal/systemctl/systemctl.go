// Package systemctl wraps the systemctl and journalctl CLIs. It is shared by
// the systemd, NixOS-container and Quadlet providers.
package systemctl

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"
	"time"

	"dashiboardie/internal/execx"
	"dashiboardie/internal/service"
)

// Client talks to either the system manager or, with User set, the calling
// user's manager (needed for rootless Quadlets).
type Client struct {
	Run  execx.Runner
	User bool
	cpu  CPUSampler
}

type Unit struct {
	ID           string
	Description  string
	LoadState    string
	ActiveState  string
	SubState     string
	SourcePath   string
	MemoryBytes  *uint64
	CPUUsageNSec *uint64
}

var showProps = "Id,Description,LoadState,ActiveState,SubState,SourcePath,MemoryCurrent,CPUUsageNSec"

func (c *Client) args(args ...string) []string {
	if c.User {
		return append([]string{"--user"}, args...)
	}
	return args
}

// Show returns the state of the given units, in the order requested.
func (c *Client) Show(ctx context.Context, units ...string) ([]Unit, error) {
	if len(units) == 0 {
		return nil, nil
	}
	args := c.args(append([]string{"show", "--no-pager", "-p", showProps, "--"}, units...)...)
	out, err := c.Run.Output(ctx, "systemctl", args...)
	if err != nil {
		return nil, err
	}
	return parseShow(string(out)), nil
}

// parseShow parses `systemctl show` output: blank-line separated blocks of
// Key=Value lines, one block per unit.
func parseShow(out string) []Unit {
	var units []Unit
	for _, block := range strings.Split(strings.TrimSpace(out), "\n\n") {
		if strings.TrimSpace(block) == "" {
			continue
		}
		var u Unit
		for _, line := range strings.Split(block, "\n") {
			k, v, ok := strings.Cut(line, "=")
			if !ok {
				continue
			}
			switch k {
			case "Id":
				u.ID = v
			case "Description":
				u.Description = v
			case "LoadState":
				u.LoadState = v
			case "ActiveState":
				u.ActiveState = v
			case "SubState":
				u.SubState = v
			case "SourcePath":
				u.SourcePath = v
			case "MemoryCurrent":
				u.MemoryBytes = parseUint(v)
			case "CPUUsageNSec":
				u.CPUUsageNSec = parseUint(v)
			}
		}
		units = append(units, u)
	}
	return units
}

// parseUint returns nil for "[not set]" and the 2^64-1 "infinity" sentinel.
func parseUint(v string) *uint64 {
	n, err := strconv.ParseUint(v, 10, 64)
	if err != nil || n == ^uint64(0) {
		return nil
	}
	return &n
}

// GeneratedServices lists service unit files created by a generator (this is
// how Quadlet units appear).
func (c *Client) GeneratedServices(ctx context.Context) ([]string, error) {
	out, err := c.Run.Output(ctx, "systemctl", c.args("list-unit-files", "--type=service", "--state=generated", "--no-legend", "--no-pager")...)
	// systemctl exits 1 with no output when no unit files match.
	var ee *execx.Error
	if errors.As(err, &ee) && ee.ExitCode == 1 && ee.Stderr == "" && len(out) == 0 {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var units []string
	for _, line := range execx.Lines(out) {
		if f := strings.Fields(line); len(f) > 0 {
			units = append(units, f[0])
		}
	}
	return units, nil
}

func (c *Client) Do(ctx context.Context, unit string, action service.Action) error {
	_, err := c.Run.Output(ctx, "systemctl", c.args(string(action), "--no-block", "--", unit)...)
	return err
}

func (c *Client) Journal(ctx context.Context, unit string, lines int) ([]string, error) {
	args := []string{"--no-pager", "-o", "short-iso", "-n", strconv.Itoa(lines), "-u", unit}
	if c.User {
		args = append([]string{"--user"}, args...)
	}
	out, err := c.Run.Output(ctx, "journalctl", args...)
	if err != nil {
		return nil, err
	}
	return execx.Lines(out), nil
}

// Status maps systemd's ActiveState onto the dashboard's status.
func (u Unit) Status() service.Status {
	switch u.ActiveState {
	case "active", "reloading", "refreshing":
		return service.StatusRunning
	case "inactive":
		return service.StatusStopped
	case "failed":
		return service.StatusFailed
	default: // activating, deactivating, maintenance
		return service.StatusUnknown
	}
}

// Metrics returns cgroup-based metrics, computing CPU% from the change in
// CPUUsageNSec since the previous call for the same unit.
func (c *Client) Metrics(u Unit) *service.Metrics {
	m := &service.Metrics{MemoryBytes: u.MemoryBytes}
	if u.CPUUsageNSec != nil {
		m.CPUPercent = c.cpu.Percent(u.ID, *u.CPUUsageNSec, time.Now())
	}
	if m.MemoryBytes == nil && m.CPUPercent == nil {
		return nil
	}
	return m
}

// CPUSampler turns cumulative CPU nanoseconds into a percentage of one core
// by diffing successive samples. The first sample for a key yields nil.
type CPUSampler struct {
	mu   sync.Mutex
	last map[string]sample
}

type sample struct {
	nsec uint64
	at   time.Time
}

func (s *CPUSampler) Percent(key string, nsec uint64, now time.Time) *float64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.last == nil {
		s.last = make(map[string]sample)
	}
	prev, ok := s.last[key]
	s.last[key] = sample{nsec, now}
	wall := now.Sub(prev.at)
	if !ok || wall <= 0 || nsec < prev.nsec {
		return nil
	}
	pct := float64(nsec-prev.nsec) / float64(wall.Nanoseconds()) * 100
	return &pct
}
