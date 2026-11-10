package main

import (
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"strings"

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
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8080", "listen address")
	static := flag.String("static", "", "directory with the built frontend to serve (e.g. ../frontend/dist)")
	useMock := flag.Bool("mock", false, "use fake in-memory providers instead of the real sources")
	units := flag.String("systemd-units", "", "comma-separated systemd units to show")
	nixosDir := flag.String("nixos-containers-dir", nixos.DefaultConfigDir, "directory with NixOS container configs")
	flag.Parse()

	var providers []service.Provider
	if *useMock {
		providers = mock.All()
	} else {
		providers = realProviders(splitList(*units), *nixosDir)
	}

	mux := http.NewServeMux()
	// Host stats are always real, even with -mock.
	api.New(service.NewRegistry(providers...), &host.Reader{}).Register(mux)
	if *static != "" {
		mux.Handle("/", http.FileServer(http.Dir(*static)))
	}

	slog.Info("listening", "addr", *addr, "mock", *useMock)
	if err := http.ListenAndServe(*addr, mux); err != nil {
		slog.Error("server stopped", "err", err)
		os.Exit(1)
	}
}

func realProviders(units []string, nixosDir string) []service.Provider {
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
	add("systemctl", systemd.New(system, units))
	add("systemctl", nixos.New(system, nixosDir))
	add("podman", podman.New(run, podmanSC))
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
