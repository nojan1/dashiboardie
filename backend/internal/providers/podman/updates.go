package podman

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// Image update checks use `podman auto-update --dry-run`, which compares each
// auto-update-enabled container's image with the registry (or local storage)
// without pulling. Only containers with an auto-update policy are checked:
// for Quadlets that is `AutoUpdate=registry` (or `local`).
//
// The check talks to registries, so it runs in the background on a long
// interval rather than on every List.

// updateSet holds which units and containers have an update pending.
type updateSet struct {
	units      map[string]bool // systemd unit (Quadlets)
	containers map[string]bool // container name
}

// autoUpdateEntry is one element of `podman auto-update --format json`.
type autoUpdateEntry struct {
	Unit          string `json:"Unit"`
	ContainerName string `json:"ContainerName"`
	Updated       string `json:"Updated"` // "pending" in a dry run when an update exists
}

const updateCheckTimeout = 5 * time.Minute

func (p *Provider) currentUpdates() updateSet {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.updates
}

// CheckUpdates runs one dry-run update check and stores the result.
func (p *Provider) CheckUpdates(ctx context.Context) error {
	out, err := p.run.Output(ctx, "podman", "auto-update", "--dry-run", "--format", "json")
	if err != nil {
		return err
	}
	set, err := parseAutoUpdate(out)
	if err != nil {
		return err
	}
	p.mu.Lock()
	p.updates = set
	p.mu.Unlock()
	return nil
}

func parseAutoUpdate(out []byte) (updateSet, error) {
	set := updateSet{units: map[string]bool{}, containers: map[string]bool{}}
	if strings.TrimSpace(string(out)) == "" {
		return set, nil // no auto-update-enabled containers
	}
	var entries []autoUpdateEntry
	if err := json.Unmarshal(out, &entries); err != nil {
		return set, fmt.Errorf("parse podman auto-update: %w", err)
	}
	for _, e := range entries {
		if e.Updated != "pending" {
			continue
		}
		if e.Unit != "" {
			set.units[e.Unit] = true
		}
		if e.ContainerName != "" {
			set.containers[e.ContainerName] = true
		}
	}
	return set, nil
}

// StartUpdateChecks checks for image updates now and then every interval
// until ctx ends. Failures are logged and keep the previous result.
func (p *Provider) StartUpdateChecks(ctx context.Context, interval time.Duration) {
	go func() {
		for {
			cctx, cancel := context.WithTimeout(ctx, updateCheckTimeout)
			start := time.Now()
			if err := p.CheckUpdates(cctx); err != nil {
				slog.Warn("image update check failed", "err", err)
			} else {
				slog.Info("image update check done", "duration", time.Since(start).Round(time.Millisecond))
			}
			cancel()
			select {
			case <-ctx.Done():
				return
			case <-time.After(interval):
			}
		}
	}()
}
