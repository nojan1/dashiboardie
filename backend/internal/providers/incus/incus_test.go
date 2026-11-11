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

// Reduced from `incus list --format json` on the test VM. Home Assistant OS
// reports docker0/hassio bridges besides its real NIC (matched by the
// volatile hwaddr); unifi has no volatile hwaddr and falls back to eth0.
const addressOutput = `[
  {"name": "homeassistant", "status": "Running", "type": "virtual-machine",
   "config": {"volatile.eth0.hwaddr": "02:00:00:b2:00:02"},
   "expanded_devices": {"eth0": {"type": "nic", "network": "lan"}},
   "state": {"network": {
     "docker0": {"hwaddr": "02:42:75:31:63:e9", "type": "broadcast", "addresses": [
       {"family": "inet", "address": "172.30.232.1", "scope": "global"}]},
     "enp5s0": {"hwaddr": "02:00:00:b2:00:02", "type": "broadcast", "addresses": [
       {"family": "inet6", "address": "fda6:17a0:8f66:26da:5a8f:25cf:ea7b:7f25", "scope": "global"},
       {"family": "inet", "address": "192.168.1.104", "scope": "global"},
       {"family": "inet6", "address": "fe80::342b:ed87:4dba:79cd", "scope": "link"}]},
     "hassio": {"hwaddr": "02:42:a0:a3:7c:fe", "type": "broadcast", "addresses": [
       {"family": "inet", "address": "172.30.32.1", "scope": "global"}]},
     "lo": {"hwaddr": "", "type": "loopback", "addresses": [
       {"family": "inet", "address": "127.0.0.1", "scope": "local"}]}}}},
  {"name": "unifi", "status": "Running", "type": "container",
   "config": {}, "expanded_devices": {"eth0": {"type": "nic"}},
   "state": {"network": {
     "eth0": {"hwaddr": "02:00:00:b2:00:01", "type": "broadcast", "addresses": [
       {"family": "inet", "address": "10.10.10.104", "scope": "global"}]},
     "lo": {"hwaddr": "", "type": "loopback", "addresses": [
       {"family": "inet", "address": "127.0.0.1", "scope": "local"}]}}}},
  {"name": "v6only", "status": "Running", "type": "container",
   "config": {"volatile.eth0.hwaddr": "02:00:00:00:00:03"},
   "state": {"network": {
     "eth0": {"hwaddr": "02:00:00:00:00:03", "type": "broadcast", "addresses": [
       {"family": "inet6", "address": "fe80::1", "scope": "link"},
       {"family": "inet6", "address": "2001:db8::3", "scope": "global"}]}}}},
  {"name": "stopped", "status": "Stopped", "type": "container", "config": {}, "state": null}
]`

func TestPrimaryAddress(t *testing.T) {
	f := &execx.Fake{Responses: map[string]string{"incus list --format json": addressOutput}}
	svcs, err := New(f).List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"homeassistant": "192.168.1.104",
		"unifi":         "10.10.10.104",
		"v6only":        "2001:db8::3",
		"stopped":       "",
	}
	for _, s := range svcs {
		if s.Address != want[s.ID] {
			t.Errorf("%s: got %q, want %q", s.ID, s.Address, want[s.ID])
		}
	}
}
