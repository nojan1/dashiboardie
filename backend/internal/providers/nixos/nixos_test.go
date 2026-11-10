package nixos

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

func TestList(t *testing.T) {
	dir := t.TempDir()
	for _, f := range []string{"web.conf", "db.conf", "README"} {
		if err := os.WriteFile(filepath.Join(dir, f), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	f := &execx.Fake{Responses: map[string]string{
		"systemctl show --no-pager -p Id,Description,LoadState,ActiveState,SubState,SourcePath,MemoryCurrent,CPUUsageNSec -- container@db.service container@web.service": `Id=container@db.service
ActiveState=inactive

Id=container@web.service
ActiveState=active
MemoryCurrent=2048
`,
		"systemctl stop --no-block -- container@web.service": "",
	}}
	p := New(&systemctl.Client{Run: f}, dir)
	ctx := context.Background()

	svcs, err := p.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(svcs) != 2 || svcs[0].ID != "db" || svcs[0].Status != service.StatusStopped ||
		svcs[1].ID != "web" || svcs[1].Status != service.StatusRunning {
		t.Fatalf("unexpected list: %+v", svcs)
	}

	if err := p.Do(ctx, service.SourceNixOS, "web", service.ActionStop); err != nil {
		t.Fatal(err)
	}
	if err := p.Do(ctx, service.SourceNixOS, "../../sshd", service.ActionStop); !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestMissingConfigDir(t *testing.T) {
	p := New(&systemctl.Client{Run: &execx.Fake{}}, filepath.Join(t.TempDir(), "nope"))
	svcs, err := p.List(context.Background())
	if err != nil || len(svcs) != 0 {
		t.Fatalf("want empty list and no error, got %v, %v", svcs, err)
	}
}
