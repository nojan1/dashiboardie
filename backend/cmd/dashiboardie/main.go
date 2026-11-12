package main

import (
	"context"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"slices"
	"strings"
	"time"

	"dashiboardie/internal/api"
	"dashiboardie/internal/execx"
	"dashiboardie/internal/host"
	"dashiboardie/internal/providers/incus"
	"dashiboardie/internal/providers/mock"
	"dashiboardie/internal/providers/nixos"
	"dashiboardie/internal/providers/podman"
	"dashiboardie/internal/providers/systemd"
	"dashiboardie/internal/service"
	"dashiboardie/internal/systemctl"
	"dashiboardie/internal/units"
	"dashiboardie/internal/web"
	"dashiboardie/internal/wol"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8080", "listen address")
	static := flag.String("static", "", "directory with the built frontend to serve (e.g. ../frontend/dist)")
	useMock := flag.Bool("mock", false, "use fake in-memory providers instead of the real sources")
	unitList := flag.String("systemd-units", "", "comma-separated systemd units to show, each optionally with a link: nginx,jellyfin=https://jellyfin.example.com")
	nixosDir := flag.String("nixos-containers-dir", nixos.DefaultConfigDir, "directory with NixOS container configs")
	timers := flag.String("systemd-timers", "", "comma-separated systemd timers to show with their last and next run: backup,nix-gc")
	updateEvery := flag.Duration("update-check-interval", 6*time.Hour, "how often to check registries for newer podman images (podman auto-update --dry-run); 0 disables")
	wolList := flag.String("wol-hosts", "", "comma-separated hosts to offer Wake-on-LAN for, as name=mac or name=mac@broadcast[:port]: nas=aa:bb:cc:dd:ee:ff@192.168.1.255")
	flag.Parse()

	wolHosts, err := wol.Parse(splitList(*wolList))
	if err != nil {
		slog.Error("bad -wol-hosts", "err", err)
		os.Exit(2)
	}
	waker := wol.New(wolHosts)

	ctx := context.Background()
	var providers []service.Provider
	var unitSource units.Source
	if *useMock {
		providers = mock.All()
		unitSource = mock.Units{}
		if len(wolHosts) == 0 {
			waker = mock.Waker()
		}
	} else {
		providers = realProviders(ctx, splitList(*unitList), *nixosDir, *updateEvery)
		unitSource = units.New(&systemctl.Client{Run: execx.Exec{}}, splitList(*timers))
	}

	// Host stats are always real, even with -mock. Sample every 10s and keep
	// an hour for the graphs.
	hostReader := &host.Reader{}
	hostReader.Start(ctx, 10*time.Second, 360)

	mux := http.NewServeMux()
	api.New(service.NewRegistry(providers...), hostReader, unitSource, waker).Register(mux)
	if *static != "" {
		mux.Handle("/", web.Handler(*static))
	}

	slog.Info("listening", "addr", *addr, "mock", *useMock)
	if err := http.ListenAndServe(*addr, mux); err != nil {
		slog.Error("server stopped", "err", err)
		os.Exit(1)
	}
}

func realProviders(ctx context.Context, unitSpecs []string, nixosDir string, updateEvery time.Duration) []service.Provider {
	run := execx.Exec{}
	system := &systemctl.Client{Run: run}
	// Podman is per-user: rootful when running as root, otherwise rootless,
	// whose Quadlets live in the user's systemd manager.
	podmanSC := &systemctl.Client{Run: run, User: os.Geteuid() != 0}

	var providers []service.Provider
	add := func(bin string, p service.Provider) {
		if _, err := exec.LookPath(bin); err != nil {
			slog.Info("provider disabled: command not found", "source", p.Sources()[0], "command", bin)
			return
		}
		providers = append(providers, p)
	}
	add("systemctl", systemd.New(system, unitSpecs))
	add("systemctl", nixos.New(system, nixosDir))
	pm := podman.New(run, podmanSC)
	add("podman", pm)
	if updateEvery > 0 && slices.Contains(providers, service.Provider(pm)) {
		pm.StartUpdateChecks(ctx, updateEvery)
	}
	add("incus", incus.New(run))
	return providers
}

func splitList(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}
