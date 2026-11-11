package systemctl

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

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

func TestGeneratedServices(t *testing.T) {
	gen := t.TempDir()
	late := t.TempDir()
	write := func(dir, name, content string) {
		t.Helper()
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// Shape of real quadlet output (podman 5.8).
	write(gen, "web.service", "[X-Container]\nImage=nginx\n\n[Unit]\nWants=network-online.target\nSourcePath=/etc/containers/systemd/web.container\n\n[Service]\nExecStart=podman run\n")
	write(gen, "home.mount", "[Mount]\nWhat=/dev/sda2\n")
	write(gen, "tmpl@.service", "[Unit]\nSourcePath=/etc/containers/systemd/tmpl@.container\n")
	write(late, "other.service", "[Unit]\nDescription=no source path\n")
	if err := os.MkdirAll(filepath.Join(gen, "default.target.wants"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../web.service", filepath.Join(gen, "default.target.wants", "web.service")); err != nil {
		t.Fatal(err)
	}

	missing := filepath.Join(t.TempDir(), "nope")
	c := &Client{GeneratorDirs: []string{missing, gen, late}}
	units, err := c.GeneratedServices()
	if err != nil {
		t.Fatal(err)
	}
	want := []GeneratedUnit{
		{Name: "other.service"},
		{Name: "web.service", SourcePath: "/etc/containers/systemd/web.container"},
	}
	if !reflect.DeepEqual(units, want) {
		t.Fatalf("got %+v, want %+v", units, want)
	}
}
