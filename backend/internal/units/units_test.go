package units

import (
	"context"
	"errors"
	"testing"

	"dashiboardie/internal/execx"
	"dashiboardie/internal/service"
	"dashiboardie/internal/systemctl"
)

const (
	failedCmd   = "systemctl list-units --state=failed --no-pager -o json"
	timersCmd   = "systemctl show --no-pager --timestamp=unix -p " + timerProps + " -- backup.timer nix-gc.timer missing.timer"
	scheduleCmd = "systemctl list-timers --all --no-pager -o json -- backup.timer nix-gc.timer missing.timer"
	servicesCmd = "systemctl show --no-pager --timestamp=unix -p " + serviceProps + " -- backup.service nix-gc.service"
)

// Captured from systemd 260 and trimmed.
func newFake() *execx.Fake {
	return &execx.Fake{Responses: map[string]string{
		failedCmd: `[{"unit":"broken.service","load":"loaded","active":"failed","sub":"failed","description":"Broken thing"}]`,
		timersCmd: `Id=backup.timer
Description=Nightly backup
LoadState=loaded
ActiveState=active
Unit=backup.service

Id=nix-gc.timer
Description=nix-gc.timer
LoadState=loaded
ActiveState=active
Unit=nix-gc.service

Id=missing.timer
Description=missing.timer
LoadState=not-found
ActiveState=inactive
Unit=
`,
		// nix-gc is monotonic and never ran: "last" is 0. Missing timers are left out.
		scheduleCmd: `[{"next":1794449700000000,"left":1794449700000000,"last":1794384243092245,"passed":45610303736,"unit":"backup.timer","activates":"backup.service"},{"next":1794425934978599,"left":1794425934978599,"last":0,"passed":0,"unit":"nix-gc.timer","activates":"nix-gc.service"}]`,
		servicesCmd: `Id=backup.service
ActiveState=failed
Result=exit-code
ExecMainStatus=2
ExecMainStartTimestamp=@1794384243

Id=nix-gc.service
ActiveState=inactive
Result=success
ExecMainStatus=0
ExecMainStartTimestamp=
`,
	}}
}

func TestOverview(t *testing.T) {
	w := New(&systemctl.Client{Run: newFake()}, []string{"backup", "nix-gc.timer", "missing"})
	ov, err := w.Overview(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(ov.Failed) != 1 || ov.Failed[0].Unit != "broken.service" || ov.Failed[0].Description != "Broken thing" {
		t.Errorf("failed: %+v", ov.Failed)
	}
	if len(ov.Timers) != 3 {
		t.Fatalf("timers: %+v", ov.Timers)
	}
	b := ov.Timers[0]
	if b.Name != "backup" || !b.Found || b.Service != "backup.service" || b.Result != "exit-code" ||
		b.ExitStatus == nil || *b.ExitStatus != 2 || b.LastRun == nil || b.NextRun == nil || b.Description != "Nightly backup" {
		t.Errorf("backup: %+v", b)
	}
	// Never ran: no result, even though systemd says "success".
	if g := ov.Timers[1]; g.Result != "" || g.LastRun != nil || g.NextRun == nil || g.Description != "" {
		t.Errorf("nix-gc: %+v", g)
	}
	if m := ov.Timers[2]; m.Found || m.Service != "" {
		t.Errorf("missing: %+v", m)
	}
}

func TestLogsAllowlist(t *testing.T) {
	f := newFake()
	for _, u := range []string{"broken.service", "backup.service", "backup.timer"} {
		f.Responses["journalctl --no-pager -o short-iso -n 10 -u "+u] = "line\n"
	}
	w := New(&systemctl.Client{Run: f}, []string{"backup", "nix-gc", "missing"})
	ctx := context.Background()
	for _, u := range []string{"broken.service", "backup.service", "backup.timer"} {
		if _, err := w.Logs(ctx, u, 10); err != nil {
			t.Errorf("%s: %v", u, err)
		}
	}
	if _, err := w.Logs(ctx, "sshd.service", 10); !errors.Is(err, service.ErrNotFound) {
		t.Errorf("sshd: want ErrNotFound, got %v", err)
	}
}
