// Package service defines the normalized service model shared by all
// providers and the Provider interface each source implements.
package service

import (
	"context"
	"errors"
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
}

// Provider is implemented by each service source.
type Provider interface {
	// Sources lists the sources this provider reports; actions on services
	// with any of them are routed back to it.
	Sources() []Source
	List(ctx context.Context) ([]Service, error)
	Do(ctx context.Context, id string, action Action) error
	Logs(ctx context.Context, id string, lines int) ([]string, error)
}

var (
	ErrNotFound       = errors.New("service not found")
	ErrNotImplemented = errors.New("not implemented")
)
