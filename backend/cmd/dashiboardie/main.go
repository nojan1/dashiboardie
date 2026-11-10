package main

import (
	"flag"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"dashiboardie/internal/api"
	"dashiboardie/internal/providers/incus"
	"dashiboardie/internal/providers/mock"
	"dashiboardie/internal/providers/nixos"
	"dashiboardie/internal/providers/podman"
	"dashiboardie/internal/providers/systemd"
	"dashiboardie/internal/service"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8080", "listen address")
	static := flag.String("static", "", "directory with the built frontend to serve (e.g. ../frontend/dist)")
	useMock := flag.Bool("mock", false, "use fake in-memory providers instead of the real sources")
	units := flag.String("systemd-units", "", "comma-separated systemd units to show")
	flag.Parse()

	var providers []service.Provider
	if *useMock {
		providers = mock.All()
	} else {
		providers = []service.Provider{
			podman.New(),
			incus.New(),
			nixos.New(),
			systemd.New(splitList(*units)),
		}
	}

	mux := http.NewServeMux()
	api.New(service.NewRegistry(providers...)).Register(mux)
	if *static != "" {
		mux.Handle("/", http.FileServer(http.Dir(*static)))
	}

	slog.Info("listening", "addr", *addr, "mock", *useMock)
	if err := http.ListenAndServe(*addr, mux); err != nil {
		slog.Error("server stopped", "err", err)
		os.Exit(1)
	}
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
