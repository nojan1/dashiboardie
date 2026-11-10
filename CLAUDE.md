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
go run ./cmd/dashiboardie -systemd-units a.service,b.service
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
  - It lists every provider concurrently and merges the results.
  - It routes actions and logs to a provider by the `source` in the URL.
  - When one provider fails, it reports that in `errors` and still returns the other sources.
- **Quadlet precedence:**
  - The podman provider claims both the `podman` and `quadlet` sources. A container managed by a Quadlet should be reported as `quadlet`, with `Unit` set to its systemd unit.
  - The registry removes `systemd` entries whose `Unit` matches a Quadlet service.
  - Actions on Quadlet services should go through systemd.
- **Providers** live in `backend/internal/providers/`. `podman`, `incus`, `nixos` and `systemd` are still stubs that return `ErrNotImplemented`, which the API maps to HTTP 501. The systemd provider shows only the units passed in with `-systemd-units`. `mock` creates one in-memory provider per source and includes a systemd duplicate of a Quadlet, so the dedupe can be tested.
- **HTTP API** (`backend/internal/api`): uses Go 1.22+ `ServeMux` method and path patterns:
  - `GET /api/services` returns `{services, errors}`.
  - `POST /api/services/{source}/{id}/{start|stop|restart}` returns 204 on success.
  - `GET /api/services/{source}/{id}/logs?lines=N` returns `{lines}`.
- **No authentication, by design:** access is enforced by the reverse proxy. Don't add auth.
- **Metrics are optional:** a nil `Metrics` (or a nil field inside it) means the source can't provide them, and the UI shows `—`.
