# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

A simple server dashboard web app that shows the status of the "services" running on a host, all in one list. The backend is Go in `backend/` and the frontend is Vue 3 + TypeScript (Vite) in `frontend/`. The full requirements are in `REQUIREMENTS.md`.

## Toolchain

This is a NixOS machine and Go/Node are not installed globally. Run every command inside the dev shell: either `nix-shell` from the repo root, or `nix-shell ../shell.nix --run '<cmd>'` from a subdirectory. Use `shell.nix`, not a flake: a flake in a non-git directory copies the whole tree, including `node_modules`, into the store.

## Commands

Backend (run from `backend/`):

```sh
go run ./cmd/dashiboardie -mock                 # fake data for every source, on 127.0.0.1:8080
go run ./cmd/dashiboardie -systemd-units nginx,sshd   # real providers; see -h for all flags
go run ./cmd/dashiboardie -static ../frontend/dist   # also serve the built frontend
go test ./...
go test ./internal/service -run TestDedupeQuadlets   # single test
go vet ./... && gofmt -l .
```

Frontend (run from `frontend/`):

```sh
npm run dev     # Vite dev server; proxies /api to 127.0.0.1:8080
npm run build   # vue-tsc type-check + production build into dist/
```

For UI work, run the backend with `-mock` and `npm run dev` side by side.

## Architecture

- **Provider model** (`backend/internal/service/service.go`): each source implements `Provider` (`Sources`, `List`, `Do`, `Logs`) and returns the normalized `Service` struct. `frontend/src/api.ts` mirrors these types by hand, so keep the two in sync.
- **Registry** (`backend/internal/service/registry.go`):
  - It lists every provider concurrently, each with a 10s timeout, and merges the results.
  - It routes actions and logs to a provider by the `source` in the URL, and passes that source along.
  - When one provider fails, it reports that in `errors` and still returns the other sources.
- **Validating client IDs:** the API is unauthenticated, so `Do`/`Logs` must check the client-supplied ID against the provider's own list before running anything (`service.Find`, the systemd allowlist, or the NixOS config dir). Commands also pass `--` before the ID.
- **CLI-based providers:** providers shell out through `execx.Runner` (`backend/internal/execx`); they don't use client libraries.
  - Tests use `execx.Fake`, keyed by the exact command line, with fixtures trimmed from real CLI output.
  - `execx.Error` exposes the exit code and stderr.
- **`systemctl` package** (`backend/internal/systemctl`): wraps `systemctl show`, actions and `journalctl`.
  - The systemd, NixOS and Quadlet providers all use it.
  - Actions use `--no-block`, so a slow stop doesn't hold the HTTP request open.
  - CPU % is computed by diffing cumulative CPU nanoseconds between successive `List` calls (`CPUSampler`), so the first sample of a service has no CPU figure. podman and incus use the same sampler.
- **Providers** (`backend/internal/providers/`):
  - `systemd`: only the units passed in with `-systemd-units`. A name without a suffix gets `.service` added.
  - `nixos`: containers are discovered from `*.conf` files in `/etc/nixos-containers` and controlled as `container@<name>.service`.
  - `podman` handles both the `podman` and `quadlet` sources:
    - Quadlets are found through systemd: they're `generated` service units whose `SourcePath` ends in `.container`, `.pod` or `.kube`. They aren't found through `podman ps`, because Quadlet containers use `--rm` and a stopped Quadlet has no container. The `PODMAN_SYSTEMD_UNIT` label is set by podman-compose too, so it doesn't prove a container is a Quadlet.
    - A container whose label points at a Quadlet unit is hidden from the plain podman list.
    - Quadlet actions and logs go through systemctl and journalctl.
    - Podman is per-user: as root it sees rootful podman and the system manager; otherwise it sees rootless podman and `systemctl --user`.
  - `incus`: only the default project. Logs come from `incus console --show-log`, falling back to `incus info --show-log`.
  - `mock` (`-mock`): one in-memory provider per source. It includes a systemd duplicate of a Quadlet so the registry's dedupe can be tested.
  - `main.go` skips any provider whose CLI isn't in `PATH`.
- **Quadlet dedupe:** the registry also removes `systemd` entries whose `Unit` matches a Quadlet. That covers a Quadlet unit that has also been listed in `-systemd-units`.
- **systemctl quirk:** `list-unit-files` exits 1 with no output when nothing matches, and `GeneratedServices` treats that as an empty result.
- **HTTP API** (`backend/internal/api`): uses Go 1.22+ `ServeMux` method and path patterns:
  - `GET /api/services` returns `{services, errors}`.
  - `POST /api/services/{source}/{id}/{start|stop|restart}` returns 204 on success.
  - `GET /api/services/{source}/{id}/logs?lines=N` returns `{lines}`.
- **No authentication, by design:** access is enforced by the reverse proxy. Don't add auth.
- **Metrics are optional:** a nil `Metrics` (or a nil field inside it) means the source can't provide them, and the UI shows `—`.
