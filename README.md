# dashiboardie 🍱

> All your server's services. One page. Zero drama.

Your homelab is a buffet: some Podman containers here, a few Quadlets there, an
Incus VM running Home Assistant in the corner, a NixOS container nobody
remembers creating, and that one systemd service holding everything together
with duct tape. **dashiboardie** puts the whole spread on a single plate.

## What's on the menu

- 🗂️ **One list to rule them all.** Podman, Quadlets, Incus, NixOS containers
  and hand-picked systemd services, side by side. Search it, filter it, love it.
- ▶️ **Start, stop, restart.** Poke your services right from the browser.
  No SSH, no `sudo systemctl` muscle memory needed.
- 📜 **Logs, live.** Peek at recent logs, or hit *Follow* and watch them scroll
  by like it's the Matrix.
- 📈 **How's the box doing?** CPU, memory and disks at a glance, with
  sparklines covering the last hour. Uptime per service, plus a little ↻ when
  something keeps restarting itself (we see you).
- 🚨 **Things that need you.** Failed units float to the top, a banner tells
  you when a reboot is due for a new kernel, and containers with a newer image
  waiting get an **↑ update** badge.
- ⏰ **Timers.** Did the nightly backup actually run? When's the next one? Did
  it fail? Now you know.
- 💤 **Wake-on-LAN.** Rise and shine! Wake up the NAS or the gaming PC from a
  tucked-away section at the bottom of the page.
- 🔗 **Links to everything.** Jump straight to a service's web UI, picked up
  from Traefik labels, published ports or your own config.
- 🌗 **Easy on the eyes.** Nord-flavoured, light and dark, and works fine
  on your phone too.

## Get it running

dashiboardie is a NixOS-first kind of dish. Add the flake, flip it on:

```nix
{
  inputs.dashiboardie.url = "github:nojan1/dashiboardie";

  outputs = { nixpkgs, dashiboardie, ... }: {
    nixosConfigurations.myserver = nixpkgs.lib.nixosSystem {
      modules = [
        dashiboardie.nixosModules.default
        {
          services.dashiboardie = {
            enable = true;
            systemdUnits = [ "nginx" "postgresql" ];
            systemdUnitUrls.nginx = "https://nginx.example.com";
            systemdTimers = [ "nix-gc" "restic-backups-daily" ];
            wakeOnLan.nas = "aa:bb:cc:dd:ee:ff";
          };
        }
      ];
    };
  };
}
```

Podman and Incus are picked up automatically when they're enabled on the host.
The dashboard listens on `127.0.0.1:8080` by default.

### ⚠️ A word about the bouncer

dashiboardie has **no login**, and that's on purpose. It's meant to sit behind
a reverse proxy that decides who gets in (Authelia, Tailscale, a VPN, whatever
you like). Don't put it on the open internet as-is, unless you want strangers
restarting your Jellyfin.

## Hacking on it

The backend is Go and the frontend is Vue. Want to play without touching a
real server? There's a mock mode full of fake services:

```sh
nix develop
cd backend && go run ./cmd/dashiboardie -mock     # terminal 1
cd frontend && npm install && npm run dev         # terminal 2
```

Then open the URL Vite prints and press all the buttons. Nothing will catch
fire. Probably.

The nitty-gritty lives in [`CLAUDE.md`](CLAUDE.md), and the original wishlist
in [`REQUIREMENTS.md`](REQUIREMENTS.md).

---

<sub>Made with ☕, Go, Vue and an unreasonable number of containers.</sub>
