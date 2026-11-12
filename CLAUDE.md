# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

A simple server dashboard web app that shows the status of the "services" running on a host, all in one list. The backend is Go in `backend/` and the frontend is Vue 3 + TypeScript (Vite) in `frontend/`. The full requirements are in `REQUIREMENTS.md`.

## Toolchain

This is a NixOS machine and Go/Node are not installed globally. Run every command inside the dev shell: `nix develop` (or `nix-shell`) from the repo root, or `nix-shell ../shell.nix --run '<cmd>'` from a subdirectory. The flake's devShell reuses `shell.nix`.

## Nix packaging

- `flake.nix` exports:
  - `packages.<system>.default`
  - `overlays.default`
  - `nixosModules.default`, which provides `services.dashiboardie.*`
  - `checks`: the package plus a NixOS VM test (`nix/test.nix`)
- **Package** (`nix/package.nix`):
  - The frontend is built with `buildNpmPackage` and the backend with `buildGoModule` (`vendorHash = null`, because there are no Go dependencies).
  - The binary is wrapped with `-static <frontend>`.
  - After `frontend/package-lock.json` changes, update `npmDepsHash`: set it to `lib.fakeHash`, build, and copy the hash from the error.
- **Module** (`nix/module.nix`):
  - The service runs as root, which is needed for system unit actions, rootful podman and incus.
  - Each provider is enabled by putting its CLI on the unit's `path`. Podman and incus default to `virtualisation.{podman,incus}.enable`.
- **Flakes only see git-tracked files:** `git add` new files before `nix build` or `nix flake check`.

```sh
nix build                                        # package -> ./result
nix flake check                                  # Go tests + VM test (needs KVM)
nix build .#checks.x86_64-linux.module -L        # VM test only
```

## Commands

Backend (run from `backend/`):

```sh
go run ./cmd/dashiboardie -mock                 # fake data for every source, on 127.0.0.1:8080
go run ./cmd/dashiboardie -systemd-units nginx,sshd -systemd-timers nix-gc   # real providers; see -h for all flags
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
  - It lists every provider concurrently, each with a 10s timeout, and merges the results. Concurrent callers share one in-flight load, a result under 2s old is reused (`cacheTTL`), and an action invalidates it. A provider taking over 2s is logged as `slow provider`.
  - It routes actions and logs to a provider by the `source` in the URL, and passes that source along.
  - When one provider fails, it reports that in `errors` and still returns the other sources.
- **Validating client IDs:** the API is unauthenticated, so `Do`/`Logs` must check the client-supplied ID against the provider's own list before running anything (`service.Find`, the systemd allowlist, or the NixOS config dir). Commands also pass `--` before the ID.
- **CLI-based providers:** providers shell out through `execx.Runner` (`backend/internal/execx`); they don't use client libraries.
  - Tests use `execx.Fake`, keyed by the exact command line, with fixtures trimmed from real CLI output.
  - `execx.Error` exposes the exit code and stderr.
  - `Stream` runs long-lived commands (`journalctl -f`, `podman logs --follow`) for live logs; closing the reader kills the process.
- **`systemctl` package** (`backend/internal/systemctl`): wraps `systemctl show`, actions and `journalctl`.
  - The systemd, NixOS and Quadlet providers all use it.
  - Actions use `--no-block`, so a slow stop doesn't hold the HTTP request open.
  - `show` runs with `--timestamp=unix`, so timestamps come back as `@<seconds>` (`ParseTimestamp`) instead of locale-formatted dates.
  - `Unit.Uptime()` gives `Service.StartedAt` (only while running) and `Service.Restarts` (systemd's `NRestarts`, i.e. automatic restarts only).
  - CPU % is computed by diffing cumulative CPU nanoseconds between successive `List` calls (`CPUSampler`), so the first sample of a service has no CPU figure. podman and incus use the same sampler.
- **Providers** (`backend/internal/providers/`):
  - `systemd`: only the units passed in with `-systemd-units`. A name without a suffix gets `.service` added.
  - `nixos`: containers are discovered from `*.conf` files in `/etc/nixos-containers` and controlled as `container@<name>.service`.
  - `podman` handles both the `podman` and `quadlet` sources:
    - Quadlets are found by reading the generator output directories (`/run/systemd/generator*`, or `$XDG_RUNTIME_DIR/systemd/generator*` when rootless) for service files whose `SourcePath=` ends in `.container`, `.pod` or `.kube`. They aren't found through `podman ps`, because Quadlet containers use `--rm` and a stopped Quadlet has no container. The `PODMAN_SYSTEMD_UNIT` label is set by podman-compose too, so it doesn't prove a container is a Quadlet.
    - A container whose label points at a Quadlet unit is hidden from the plain podman list.
    - Quadlet actions and logs go through systemctl and journalctl.
    - Podman is per-user: as root it sees rootful podman and the system manager; otherwise it sees rootless podman and `systemctl --user`.
    - Image updates: `podman auto-update --dry-run --format json` runs in the background every `-update-check-interval` (default 6h; it talks to registries), and entries with `Updated: "pending"` set `Service.UpdateAvailable`. Only containers with an auto-update policy (Quadlet `AutoUpdate=registry`) are checked.
    - Uptime comes from `podman ps` `StartedAt`/`Restarts` for containers, and from systemd for Quadlets.
  - `incus`: only the default project. Logs come from `incus console --show-log`, falling back to `incus info --show-log`. Uptime comes from `last_used_at`. Its logs can't be followed (it doesn't implement `service.LogFollower`).
  - `mock` (`-mock`): one in-memory provider per source. `mock.Units` fakes the units overview, and followed logs emit a line per second. It includes a systemd duplicate of a Quadlet so the registry's dedupe can be tested.
  - `main.go` skips any provider whose CLI isn't in `PATH`.
- **Quadlet dedupe:** the registry also removes `systemd` entries whose `Unit` matches a Quadlet. That covers a Quadlet unit that has also been listed in `-systemd-units`.
- **Never use `systemctl list-unit-files`:** PID 1 resolves every unit file on disk for it, which took 8.7s on a 4-core VM and blocks other systemctl calls meanwhile. Its `--state=generated` filter also misses Quadlets that have a NixOS-written unit file in `/etc/systemd/system`, which systemd reports as `bad`.
- **HTTP API** (`backend/internal/api`): uses Go 1.22+ `ServeMux` method and path patterns:
  - `GET /api/services` returns `{services, errors}`.
  - `POST /api/services/{source}/{id}/{start|stop|restart}` returns 204 on success.
  - `GET /api/services/{source}/{id}/logs?lines=N` returns `{lines}`.
  - `GET .../logs/follow?lines=N` streams the logs as Server-Sent Events (one `data:` per line), for providers implementing `service.LogFollower`. When the command ends by itself, an `end` event is sent, so the browser's EventSource doesn't reconnect. The frontend clears its lines on every `open`, since each reconnect resends the backlog.
  - `GET /api/units` returns `{failed, timers}`. `GET /api/units/{unit}/logs[/follow]` only allows units that are failed right now, the configured timers, and the services they trigger.
- **Units overview** (`backend/internal/units`): host-wide systemd state that isn't a service row.
  - All failed units come from `systemctl list-units --state=failed -o json`. This is cheap because it only walks loaded units, unlike `list-unit-files`.
  - The timers come from `-systemd-timers` (`services.dashiboardie.systemdTimers`). Each shows the timer's `LastTriggerUSec`/`NextElapseUSecRealtime` plus the triggered service's `Result`/`ExecMainStatus`. A result is only reported when the service started this boot (`ExecMainStartTimestamp`) or failed. systemd says `success` for a service that never ran, and a `Persistent=` timer reports a last run from its stamp file even if the job never ran: the stamp is created when the timer first appears.
- **Wake-on-LAN** (`backend/internal/wol`, `GET /api/wol`, `POST /api/wol/{name}`): hosts come from `-wol-hosts name=mac[@address[:port]]` (`services.dashiboardie.wakeOnLan`). Clients only send a configured name, never a MAC address or target. The default target is 255.255.255.255:9, which leaves through the default route's interface, so give a subnet broadcast address for other networks. The packet is sent 3 times with no reply, so the UI can only say "sent". With `-mock` and no hosts set, `mock.Waker` offers fake hosts and only logs. The frontend section is a collapsed `<details>` whose open state is kept in localStorage, and it is hidden when no hosts are configured.
- **No authentication, by design:** access is enforced by the reverse proxy. Don't add auth.
- **Metrics are optional:** a nil `Metrics` (or a nil field inside it) means the source can't provide them, and the UI shows `—`.
- **Host resources** (`backend/internal/host`, `GET /api/host`): the overview at the top of the page. It's independent of the providers and shows real data even with `-mock`.
  - `Reader.Start` samples CPU % and memory % every 10s in the background and keeps 1h in memory (`history`, for the frontend's sparklines; it resets on restart). While it runs, `/api/host` reports the latest sampled CPU %. Without `Start` (tests), CPU is diffed against the previous request.
  - Memory used is MemTotal minus MemAvailable.
  - Filesystems are read from `/proc/self/mountinfo`, filtered to an allowlist of real filesystem types and deduplicated by device, keeping the shortest mountpoint. This collapses `/nix/store` and btrfs subvolumes.
  - Each `statfs` call has a 2s timeout, so a hung NFS mount can't stall the response.
  - Disk % uses the same basis as `df`: used / (used + avail).
  - `reboot` compares the `kernel`, `initrd` and `kernel-modules` links of `/run/booted-system` and `/run/current-system`. It is nil when either is missing (not NixOS).
  - Because of this, the NixOS module must not use mount-namespacing options such as `ProtectHome` or `PrivateTmp`: they would change what the service sees mounted.
- **Frontend usage thresholds:** set in `usageLevel` in `frontend/src/format.ts`: 75% is warning and 90% is critical. Meters switch colour at those levels, and `UsageFlag` adds a text label, so the state is never shown by colour alone.
- **Service links (`Service.URL`, `Service.Address`):**
  - podman/Quadlet links come from Traefik docker-provider labels: the first `Host()` (plus `PathPrefix()`) of a router's `rule`. The scheme is https when the router has `tls`/`tls.certresolver` or an entrypoint named like `websecure`/`https`; `traefik.enable=false` suppresses the link.
  - Quadlet labels are read from the `[X-Container]` section that Quadlet copies into the generated unit, so links still show while the Quadlet is stopped and its container is gone.
  - incus shows an IP (`Address`), not a link: the global address on the NIC whose MAC matches `volatile.<nic>.hwaddr` or a nic device's `hwaddr`, then `eth0`, then any non-loopback interface; IPv4 is preferred. VMs such as Home Assistant OS report Docker bridges too, so "first IP" would be wrong.
  - systemd links are configured: `-systemd-units name=url,...`, or `services.dashiboardie.systemdUnitUrls` in the module. A systemd service's `ID` is the configured name and `Unit` is what systemd reports, which differ for aliases.
  - podman containers and Quadlets without a Traefik link get `Service.Ports`: their published TCP host ports, which the frontend links as `http://<dashboard hostname>:<port>`. It assumes http and that the dashboard is opened on the same host. Ports come from `podman ps` `Ports` for containers, and from `PublishPort=` in the `[X-Container]` section for Quadlets, so they show while a Quadlet is stopped. UDP, loopback-bound and random host ports are skipped.
  - The frontend renders only `http:`/`https:` URLs (`safeUrl` in `App.vue`), since labels and config are not fully trusted.
- **Static file caching** (`backend/internal/web`): don't serve the frontend with plain `http.FileServer`. Nix store files have a 1970 mtime, so `Last-Modified`-based caching makes browsers keep an old `index.html` for years, and revalidation always returns 304. `index.html` and other un-hashed files get `Cache-Control: no-cache` plus a content-hash `ETag` and no `Last-Modified`; Vite's hashed `/assets/*` get `immutable`.
