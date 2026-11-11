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
	showCmd  = "systemctl --user show --no-pager -p Id,Description,LoadState,ActiveState,SubState,SourcePath,MemoryCurrent,CPUUsageNSec -- jellyfin.service"
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
  {"Id": "ccc", "Names": ["redis"], "State": "running", "IsInfra": false, "Labels": null},
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
