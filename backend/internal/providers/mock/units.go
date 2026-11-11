package mock

import (
	"context"
	"io"
	"time"

	"dashiboardie/internal/service"
	"dashiboardie/internal/systemctl"
	"dashiboardie/internal/units"
)

// Units is a fake units.Source with a failed unit and a few timers.
type Units struct{}

func (Units) Overview(context.Context) (units.Overview, error) {
	at := func(d time.Duration) *time.Time {
		t := time.Now().Add(d).Truncate(time.Minute)
		return &t
	}
	return units.Overview{
		Failed: []systemctl.FailedUnit{
			{Unit: "backup.service", Description: "Nightly restic backup"},
			{Unit: "dev-disk-by\\x2dlabel-archive.mount", Description: "/mnt/archive"},
		},
		Timers: []units.Timer{
			{Name: "backup", Unit: "backup.timer", Service: "backup.service", Description: "Nightly restic backup",
				Found: true, Active: true, LastRun: at(-7 * time.Hour), NextRun: at(17 * time.Hour), Result: "exit-code", ExitStatus: ptr(1)},
			{Name: "nix-gc", Unit: "nix-gc.timer", Service: "nix-gc.service",
				Found: true, Active: true, LastRun: at(-2 * 24 * time.Hour), NextRun: at(5 * 24 * time.Hour), Result: "success", ExitStatus: ptr(0)},
			{Name: "podman-auto-update", Unit: "podman-auto-update.timer", Service: "podman-auto-update.service",
				Found: true, Active: true, LastRun: at(-3 * time.Minute), NextRun: at(21 * time.Hour), Running: true, Result: "success"},
			{Name: "certbot-renew", Unit: "certbot-renew.timer", Found: false},
		},
	}, nil
}

func (u Units) allowed(ctx context.Context, unit string) error {
	ov, _ := u.Overview(ctx)
	for _, f := range ov.Failed {
		if f.Unit == unit {
			return nil
		}
	}
	for _, t := range ov.Timers {
		if t.Unit == unit || (t.Service != "" && t.Service == unit) {
			return nil
		}
	}
	return service.ErrNotFound
}

func (u Units) Logs(ctx context.Context, unit string, lines int) ([]string, error) {
	if err := u.allowed(ctx, unit); err != nil {
		return nil, err
	}
	return (&Provider{}).Logs(ctx, "", unit, lines)
}

func (u Units) FollowLogs(ctx context.Context, unit string, lines int) (io.ReadCloser, error) {
	backlog, err := u.Logs(ctx, unit, lines)
	if err != nil {
		return nil, err
	}
	return fakeStream(ctx, unit, backlog), nil
}
