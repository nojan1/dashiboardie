package incus

import (
	"context"
	"errors"
	"strings"
	"testing"

	"dashiboardie/internal/execx"
	"dashiboardie/internal/service"
)

// Trimmed `incus list --format json` output.
const listOutput = `[
  {"name": "web", "status": "Running", "type": "container",
   "state": {"status": "Running", "cpu": {"usage": 123000}, "memory": {"usage": 52428800}}},
  {"name": "dev-vm", "status": "Stopped", "type": "virtual-machine",
   "state": {"status": "Stopped", "cpu": {"usage": 0}, "memory": {"usage": 0}}},
  {"name": "broken", "status": "Error", "type": "container", "state": null}
]`

func TestList(t *testing.T) {
	f := &execx.Fake{Responses: map[string]string{"incus list --format json": listOutput}}
	svcs, err := New(f).List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(svcs) != 3 {
		t.Fatalf("want 3, got %d", len(svcs))
	}
	if svcs[0].Status != service.StatusRunning || svcs[0].Metrics == nil || *svcs[0].Metrics.MemoryBytes != 52428800 {
		t.Errorf("web: %+v", svcs[0])
	}
	if svcs[1].Status != service.StatusStopped || svcs[1].Metrics != nil {
		t.Errorf("dev-vm: %+v", svcs[1])
	}
	if svcs[2].Status != service.StatusFailed {
		t.Errorf("broken: %+v", svcs[2])
	}
}

func TestDoAndLogs(t *testing.T) {
	f := &execx.Fake{
		Responses: map[string]string{
			"incus list --format json":        listOutput,
			"incus start -- dev-vm":           "",
			"incus info --show-log -- dev-vm": "Name: dev-vm\nStatus: STOPPED\n\nLog:\n\nline1\nline2\nline3\n",
		},
		Errors: map[string]error{
			"incus console --show-log -- dev-vm": errors.New("console log unavailable"),
		},
	}
	p := New(f)
	ctx := context.Background()

	if err := p.Do(ctx, service.SourceIncus, "dev-vm", service.ActionStart); err != nil {
		t.Fatal(err)
	}
	if err := p.Do(ctx, service.SourceIncus, "nope", service.ActionStart); !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
	lines, err := p.Logs(ctx, service.SourceIncus, "dev-vm", 2)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(lines, ",") != "line2,line3" {
		t.Fatalf("unexpected log lines: %q", lines)
	}
}
