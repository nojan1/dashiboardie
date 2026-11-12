// Package service defines the normalized service model shared by all
// providers and the Provider interface each source implements.
package service

import (
	"context"
	"errors"
	"io"
	"time"
)

// Source identifies where a service comes from.
type Source string

const (
	SourcePodman  Source = "podman"
	SourceQuadlet Source = "quadlet"
	SourceIncus   Source = "incus"
	SourceNixOS   Source = "nixos"
	SourceSystemd Source = "systemd"
	SourceMock    Source = "mock"
)

// Status is the normalized run state of a service.
type Status string

const (
	StatusRunning Status = "running"
	StatusStopped Status = "stopped"
	StatusFailed  Status = "failed"
	StatusUnknown Status = "unknown"
)

// Action is a lifecycle operation on a service.
type Action string

const (
	ActionStart   Action = "start"
	ActionStop    Action = "stop"
	ActionRestart Action = "restart"
)

// Metrics are optional; a nil *Metrics means the source cannot provide them.
type Metrics struct {
	CPUPercent  *float64 `json:"cpuPercent,omitempty"`
	MemoryBytes *uint64  `json:"memoryBytes,omitempty"`
}

// Service is one row in the dashboard.
type Service struct {
	// ID is unique within a Source.
	ID      string   `json:"id"`
	Source  Source   `json:"source"`
	Name    string   `json:"name"`
	Status  Status   `json:"status"`
	Metrics *Metrics `json:"metrics,omitempty"`
	// Unit is the systemd unit backing this service, if any. The registry uses
	// it to drop systemd entries that duplicate a Quadlet-managed container.
	Unit string `json:"unit,omitempty"`
	// URL is a link to the service's web UI, if known (Traefik labels for
	// podman/Quadlet, configured for systemd units).
	URL string `json:"url,omitempty"`
	// Address is the service's primary IP, shown as text (incus instances).
	Address string `json:"address,omitempty"`
	// Ports are published TCP host ports, offered as links on the dashboard's
	// own host when there is no URL (podman containers and Quadlets).
	Ports []int `json:"ports,omitempty"`
	// StartedAt is when the service last started; set only while running.
	StartedAt *time.Time `json:"startedAt,omitempty"`
	// Restarts counts automatic restarts (systemd NRestarts, podman's
	// restart policy); nil if the source doesn't track them.
	Restarts *int `json:"restarts,omitempty"`
	// UpdateAvailable means a newer image exists in the registry (podman
	// auto-update dry run).
	UpdateAvailable bool `json:"updateAvailable,omitempty"`
}

// Provider is implemented by each service source.
type Provider interface {
	// Sources lists the sources this provider reports; actions on services
	// with any of them are routed back to it. The first one is used to label
	// List errors.
	Sources() []Source
	List(ctx context.Context) ([]Service, error)
	// Do and Logs must only act on services the provider itself lists: the
	// API is unauthenticated, so src/id come straight from the client.
	Do(ctx context.Context, src Source, id string, action Action) error
	Logs(ctx context.Context, src Source, id string, lines int) ([]string, error)
}

// LogFollower is implemented by providers that can stream logs live. The
// stream starts with the last `lines` lines, then follows; closing it stops
// the underlying command. It must validate src/id like Logs does.
type LogFollower interface {
	FollowLogs(ctx context.Context, src Source, id string, lines int) (io.ReadCloser, error)
}

// Find returns the service with the given source and id, or ErrNotFound.
// Providers use it to validate client-supplied ids against their own List.
func Find(svcs []Service, src Source, id string) (Service, error) {
	for _, s := range svcs {
		if s.Source == src && s.ID == id {
			return s, nil
		}
	}
	return Service{}, ErrNotFound
}

var (
	ErrNotFound       = errors.New("service not found")
	ErrNotImplemented = errors.New("not implemented")
)
