package systemd

import (
	"context"
	"errors"
	"testing"

	"dashiboardie/internal/execx"
	"dashiboardie/internal/service"
	"dashiboardie/internal/systemctl"
)

const show = "systemctl show --no-pager -p Id,Description,LoadState,ActiveState,SubState,SourcePath,MemoryCurrent,CPUUsageNSec -- nginx.service backup.service"

func TestList(t *testing.T) {
	f := &execx.Fake{Responses: map[string]string{show: `Id=nginx.service
LoadState=loaded
ActiveState=active
MemoryCurrent=1024
CPUUsageNSec=5

Id=backup.service
LoadState=loaded
ActiveState=failed
MemoryCurrent=[not set]
CPUUsageNSec=[not set]
`}}
	// "nginx" without a suffix is normalized to nginx.service.
	p := New(&systemctl.Client{Run: f}, []string{"nginx", "backup.service"})
	svcs, err := p.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(svcs) != 2 {
		t.Fatalf("want 2, got %d", len(svcs))
	}
	if svcs[0].Name != "nginx" || svcs[0].Status != service.StatusRunning || svcs[0].Metrics == nil {
		t.Errorf("nginx: %+v", svcs[0])
	}
	if svcs[1].Status != service.StatusFailed || svcs[1].Metrics != nil {
		t.Errorf("backup: %+v", svcs[1])
	}
}

func TestDoRejectsUnlistedUnits(t *testing.T) {
	f := &execx.Fake{Responses: map[string]string{
		"systemctl restart --no-block -- nginx.service": "",
	}}
	p := New(&systemctl.Client{Run: f}, []string{"nginx.service"})
	ctx := context.Background()

	if err := p.Do(ctx, service.SourceSystemd, "sshd.service", service.ActionStop); !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("want ErrNotFound for unlisted unit, got %v", err)
	}
	if len(f.Calls) != 0 {
		t.Fatalf("no command should run for unlisted unit, ran %v", f.Calls)
	}
	if err := p.Do(ctx, service.SourceSystemd, "nginx.service", service.ActionRestart); err != nil {
		t.Fatal(err)
	}
}
