package systemctl

import (
	"context"
	"testing"
	"time"

	"dashiboardie/internal/execx"
	"dashiboardie/internal/service"
)

// Captured from `systemctl show` on systemd 260.
const showOutput = `Id=nix-daemon.service
Description=Nix Daemon
LoadState=loaded
ActiveState=active
SubState=running
SourcePath=
MemoryCurrent=1810071552
CPUUsageNSec=10671705502000

Id=does-not-exist.service
Description=does-not-exist.service
LoadState=not-found
ActiveState=inactive
SubState=dead
SourcePath=
MemoryCurrent=[not set]
CPUUsageNSec=[not set]
`

func TestParseShow(t *testing.T) {
	units := parseShow(showOutput)
	if len(units) != 2 {
		t.Fatalf("want 2 units, got %d", len(units))
	}
	u := units[0]
	if u.ID != "nix-daemon.service" || u.Status() != service.StatusRunning {
		t.Errorf("unexpected first unit: %+v", u)
	}
	if u.MemoryBytes == nil || *u.MemoryBytes != 1810071552 {
		t.Errorf("memory not parsed: %v", u.MemoryBytes)
	}
	missing := units[1]
	if missing.LoadState != "not-found" || missing.MemoryBytes != nil || missing.CPUUsageNSec != nil {
		t.Errorf("unexpected missing unit: %+v", missing)
	}
}

func TestCPUSampler(t *testing.T) {
	var s CPUSampler
	t0 := time.Unix(1000, 0)
	if p := s.Percent("a", 0, t0); p != nil {
		t.Fatalf("first sample should be nil, got %v", *p)
	}
	// 0.5s of CPU over 1s of wall time = 50%.
	p := s.Percent("a", 500_000_000, t0.Add(time.Second))
	if p == nil || *p != 50 {
		t.Fatalf("want 50%%, got %v", p)
	}
	// Counter reset (unit restarted) yields nil rather than a bogus value.
	if p := s.Percent("a", 10, t0.Add(2*time.Second)); p != nil {
		t.Fatalf("want nil after counter reset, got %v", *p)
	}
}

func TestGeneratedServicesNoMatches(t *testing.T) {
	cmd := "systemctl --user list-unit-files --type=service --state=generated --no-legend --no-pager"
	f := &execx.Fake{Errors: map[string]error{cmd: &execx.Error{Cmd: cmd, ExitCode: 1}}}
	units, err := (&Client{Run: f, User: true}).GeneratedServices(context.Background())
	if err != nil || len(units) != 0 {
		t.Fatalf("want no units and no error, got %v, %v", units, err)
	}
}
