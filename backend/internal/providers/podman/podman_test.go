package podman

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"dashiboardie/internal/execx"
	"dashiboardie/internal/service"
	"dashiboardie/internal/systemctl"
)

const (
	// Only the Quadlet unit is shown; other generated units are filtered
	// out by SourcePath before systemctl is called.
	showCmd  = "systemctl --user show --no-pager --timestamp=unix -p Id,Description,LoadState,ActiveState,SubState,SourcePath,MemoryCurrent,CPUUsageNSec,ActiveEnterTimestamp,NRestarts -- jellyfin.service"
	psCmd    = "podman ps --all --format json"
	statsCmd = "podman stats --no-stream --format {{json .ContainerStats}}"
)

// Trimmed from real `podman ps --all --format json` output. The jellyfin
// container belongs to a Quadlet; openwebui is podman-compose, whose
// PODMAN_SYSTEMD_UNIT label must not make it look like a Quadlet.
const psOutput = `[
  {"Id": "aaa", "Names": ["systemd-jellyfin"], "State": "running", "IsInfra": false,
   "Labels": {"PODMAN_SYSTEMD_UNIT": "jellyfin.service"}},
  {"Id": "bbb", "Names": ["openwebui_openwebui_1"], "State": "exited", "IsInfra": false,
   "Labels": {"PODMAN_SYSTEMD_UNIT": "podman-compose@openwebui.service"}},
  {"Id": "ccc", "Names": ["redis"], "State": "running", "IsInfra": false, "Labels": null,
   "StartedAt": 1786213497, "Restarts": 2},
  {"Id": "ddd", "Names": ["efe340b616bc-infra"], "State": "running", "IsInfra": true, "Labels": {}}
]`

func newFake() *execx.Fake {
	return &execx.Fake{Responses: map[string]string{
		showCmd: `Id=jellyfin.service
ActiveState=active
SourcePath=/home/u/.config/containers/systemd/jellyfin.container
MemoryCurrent=536870912
CPUUsageNSec=100
`,
		psCmd: psOutput,
		statsCmd: `{"ContainerID":"aaa","CPUNano":1,"MemUsage":1}
{"ContainerID":"ccc","CPUNano":33854000,"MemUsage":17391616}
`,
	}}
}

// newProvider sets up a generator directory holding one Quadlet unit and one
// unrelated generated unit.
func newProvider(t *testing.T, f *execx.Fake) *Provider {
	t.Helper()
	gen := t.TempDir()
	files := map[string]string{
		"jellyfin.service":        "[Unit]\nSourcePath=/home/u/.config/containers/systemd/jellyfin.container\n",
		"other-generated.service": "[Unit]\nSourcePath=/etc/fstab\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(gen, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return New(f, &systemctl.Client{Run: f, User: true, GeneratorDirs: []string{gen}})
}

func TestList(t *testing.T) {
	svcs, err := newProvider(t, newFake()).List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]service.Service{}
	for _, s := range svcs {
		byID[string(s.Source)+"/"+s.ID] = s
	}
	if len(svcs) != 3 {
		t.Fatalf("want quadlet + 2 containers, got %+v", svcs)
	}
	q, ok := byID["quadlet/jellyfin.service"]
	if !ok || q.Status != service.StatusRunning || q.Metrics == nil || *q.Metrics.MemoryBytes != 536870912 {
		t.Errorf("quadlet: %+v", q)
	}
	if _, ok := byID["podman/systemd-jellyfin"]; ok {
		t.Error("quadlet's container should be hidden from the podman list")
	}
	if s := byID["podman/openwebui_openwebui_1"]; s.Status != service.StatusStopped {
		t.Errorf("podman-compose container should be listed as plain podman: %+v", s)
	}
	r := byID["podman/redis"]
	if r.Metrics == nil || *r.Metrics.MemoryBytes != 17391616 {
		t.Errorf("redis metrics: %+v", r.Metrics)
	}
	if r.StartedAt == nil || r.StartedAt.Unix() != 1786213497 || r.Restarts == nil || *r.Restarts != 2 {
		t.Errorf("redis uptime: %v %v", r.StartedAt, r.Restarts)
	}
}

func TestDoRoutesBySource(t *testing.T) {
	f := newFake()
	f.Responses["systemctl --user restart --no-block -- jellyfin.service"] = ""
	f.Responses["podman stop -- redis"] = ""
	p := newProvider(t, f)
	ctx := context.Background()

	if err := p.Do(ctx, service.SourceQuadlet, "jellyfin.service", service.ActionRestart); err != nil {
		t.Fatal(err)
	}
	if err := p.Do(ctx, service.SourcePodman, "redis", service.ActionStop); err != nil {
		t.Fatal(err)
	}
	// A podman id under the quadlet source (or vice versa) must not resolve.
	if err := p.Do(ctx, service.SourceQuadlet, "redis", service.ActionStop); !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
	if err := p.Do(ctx, service.SourcePodman, "--all", service.ActionStop); !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestDoSkipsStats(t *testing.T) {
	f := newFake()
	f.Responses["podman stop -- redis"] = ""
	if err := newProvider(t, f).Do(context.Background(), service.SourcePodman, "redis", service.ActionStop); err != nil {
		t.Fatal(err)
	}
	if f.Called(statsCmd) {
		t.Fatal("validating an action should not run podman stats")
	}
}

// Shaped like `podman auto-update --dry-run --format json` (podman 5).
const autoUpdateOutput = `[
  {"Unit": "jellyfin.service", "Container": "aaa (systemd-jellyfin)", "ContainerName": "systemd-jellyfin",
   "ContainerID": "aaa", "Image": "docker.io/jellyfin/jellyfin:latest", "Policy": "registry", "Updated": "pending"},
  {"Unit": "redis.service", "Container": "ccc (redis)", "ContainerName": "redis",
   "ContainerID": "ccc", "Image": "docker.io/library/redis:7", "Policy": "registry", "Updated": "false"}
]`

func TestUpdateChecks(t *testing.T) {
	f := newFake()
	f.Responses["podman auto-update --dry-run --format json"] = autoUpdateOutput
	p := newProvider(t, f)
	ctx := context.Background()
	if err := p.CheckUpdates(ctx); err != nil {
		t.Fatal(err)
	}
	svcs, err := p.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range svcs {
		want := s.ID == "jellyfin.service" // only the pending one
		if s.UpdateAvailable != want {
			t.Errorf("%s/%s: UpdateAvailable=%v", s.Source, s.ID, s.UpdateAvailable)
		}
	}

	// No auto-update-enabled containers: empty output, no updates.
	if set, err := parseAutoUpdate([]byte("\n")); err != nil || len(set.units) != 0 {
		t.Fatalf("empty output: %v %v", set, err)
	}
}

func TestFollowLogsValidates(t *testing.T) {
	f := newFake()
	f.Responses["podman logs --follow --timestamps --tail 5 -- redis"] = "a\nb\n"
	p := newProvider(t, f)
	ctx := context.Background()
	if _, err := p.FollowLogs(ctx, service.SourcePodman, "nope", 5); !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
	rc, err := p.FollowLogs(ctx, service.SourcePodman, "redis", 5)
	if err != nil {
		t.Fatal(err)
	}
	rc.Close()
}
