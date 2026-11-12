# NixOS VM test: run `nix flake check` or `nix build .#checks.x86_64-linux.module`.
{ pkgs, module }:

let
  # A tiny local image so the Quadlet can start without network access.
  image = pkgs.dockerTools.buildImage {
    name = "localhost/sleeper";
    tag = "latest";
    copyToRoot = [ pkgs.busybox ];
    # A bare `sleep` as PID 1 ignores SIGTERM, so stopping would end in
    # SIGKILL (exit 137) and leave the unit "failed" instead of "inactive".
    config.Cmd = [
      "sh"
      "-c"
      "trap 'exit 0' TERM; sleep infinity & wait"
    ];
  };
in
pkgs.testers.runNixOSTest {
  name = "dashiboardie";

  nodes.machine = {
    imports = [ module ];

    services.dashiboardie = {
      enable = true;
      systemdUnits = [ "hello" ];
      systemdUnitUrls.hello = "https://hello.example.com";
      systemdTimers = [
        "tick"
        "missing"
      ];
      wakeOnLan.nas = "02:00:00:00:00:01@127.0.0.1:40009";
    };

    systemd.services.tick = {
      serviceConfig.Type = "oneshot";
      script = "echo tick ran";
    };
    systemd.timers.tick = {
      wantedBy = [ "timers.target" ];
      timerConfig = {
        OnActiveSec = "2s";
        OnUnitActiveSec = "1h";
      };
    };
    systemd.services.broken = {
      wantedBy = [ "multi-user.target" ];
      serviceConfig.Type = "oneshot";
      script = "echo boom; exit 3";
    };

    virtualisation.podman.enable = true;
    environment.etc."containers/systemd/web.container".text = ''
      [Container]
      Image=docker-archive:${image}
      Label=traefik.http.routers.web.entrypoints=web
      Label=traefik.http.routers.web.rule=Host(`web.home`)

      [Install]
      WantedBy=multi-user.target
    '';

    systemd.services.hello = {
      wantedBy = [ "multi-user.target" ];
      script = "echo hello from the test unit; exec sleep infinity";
    };
    systemd.services.not-listed = {
      wantedBy = [ "multi-user.target" ];
      script = "exec sleep infinity";
    };

    environment.systemPackages = [
      pkgs.curl
      pkgs.jq
      pkgs.socat
    ];
  };

  testScript = ''
    import json

    machine.wait_for_unit("dashiboardie.service")
    machine.wait_for_unit("hello.service")
    machine.wait_for_open_port(8080)

    def services():
        out = machine.succeed("curl -sf http://127.0.0.1:8080/api/services")
        return json.loads(out)

    def find(res, source, id):
        return next((s for s in res["services"] if s["source"] == source and s["id"] == id), None)

    machine.wait_for_unit("web.service")

    with subtest("lists only the configured systemd unit plus the quadlet"):
        res = services()
        assert res["errors"] == [], res["errors"]
        ids = sorted(f'{s["source"]}/{s["id"]}' for s in res["services"])
        assert ids == ["quadlet/web.service", "systemd/hello.service"], ids
        assert find(res, "systemd", "hello.service")["status"] == "running", res

    with subtest("links from config and traefik labels"):
        res = services()
        assert find(res, "systemd", "hello.service")["url"] == "https://hello.example.com", res
        assert find(res, "quadlet", "web.service")["url"] == "http://web.home", res

    with subtest("uptime and restart counts"):
        res = services()
        hello = find(res, "systemd", "hello.service")
        assert hello["startedAt"] and hello["restarts"] == 0, hello
        assert find(res, "quadlet", "web.service")["startedAt"], res

    with subtest("failed units are reported and their logs readable"):
        machine.wait_until_succeeds(
            "curl -sf http://127.0.0.1:8080/api/units | jq -e '.failed | map(.unit) | index(\"broken.service\")'"
        )
        machine.succeed(
            "curl -sf 'http://127.0.0.1:8080/api/units/broken.service/logs?lines=20'"
            " | jq -e '.lines | map(select(contains(\"boom\"))) | length > 0'"
        )
        # Only failed units and configured timers are readable.
        code = machine.succeed(
            "curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:8080/api/units/not-listed.service/logs"
        )
        assert code == "404", code

    with subtest("timers show last run, result and next run"):
        machine.wait_until_succeeds(
            "curl -sf http://127.0.0.1:8080/api/units"
            " | jq -e '.timers[] | select(.name == \"tick\") | .lastRun and .nextRun and .result == \"success\"'"
        )
        timers = json.loads(machine.succeed("curl -sf http://127.0.0.1:8080/api/units"))["timers"]
        assert [t["name"] for t in timers] == ["tick", "missing"], timers
        assert timers[1]["found"] is False, timers
        machine.succeed(
            "curl -sf 'http://127.0.0.1:8080/api/units/tick.service/logs?lines=20'"
            " | jq -e '.lines | map(select(contains(\"tick ran\"))) | length > 0'"
        )

    with subtest("no reboot pending, history is sampled"):
        h = json.loads(machine.succeed("curl -sf http://127.0.0.1:8080/api/host"))
        assert h["reboot"] is not None and h["reboot"]["required"] is False, h["reboot"]
        assert h["uptimeSeconds"] > 0, h
        machine.wait_until_succeeds(
            "curl -sf http://127.0.0.1:8080/api/host | jq -e '.history.points | length >= 1'"
        )

    with subtest("logs can be followed live"):
        machine.succeed(
            "(timeout 8 curl -sN 'http://127.0.0.1:8080/api/services/systemd/hello.service/logs/follow?lines=1'"
            " > /tmp/follow || true) & sleep 3; systemctl restart hello; wait"
        )
        out = machine.succeed("cat /tmp/follow")
        # The backlog line plus what the restart logged.
        assert out.count("data: ") >= 3 and out.count("hello from the test unit") >= 1, out
        code = machine.succeed(
            "curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:8080/api/services/systemd/not-listed.service/logs/follow"
        )
        assert code == "404", code
        # The journalctl started for the stream must be gone again.
        machine.fail("pgrep -f 'journalctl.*-f -u hello.service'")

    with subtest("wake-on-lan sends a magic packet to configured hosts only"):
        machine.succeed("systemd-run --unit wolcap socat -u UDP-RECV:40009 CREATE:/tmp/wol.bin")
        machine.wait_until_succeeds("ss -uln | grep -q ':40009'")
        hosts = json.loads(machine.succeed("curl -sf http://127.0.0.1:8080/api/wol"))["hosts"]
        assert [h["name"] for h in hosts] == ["nas"], hosts
        machine.succeed("curl -sf -X POST http://127.0.0.1:8080/api/wol/nas")
        machine.wait_until_succeeds("test $(stat -c %s /tmp/wol.bin) -ge 102")
        head = machine.succeed("head -c 12 /tmp/wol.bin | od -An -tx1").split()
        assert head == ["ff"] * 6 + ["02", "00", "00", "00", "00", "01"], head
        code = machine.succeed(
            "curl -s -o /dev/null -w '%{http_code}' -X POST http://127.0.0.1:8080/api/wol/other"
        )
        assert code == "404", code

    with subtest("listing is fast"):
        # Regression: list-unit-files made this take >10s on small VMs.
        machine.succeed("sleep 3")  # let the cache expire
        took = float(machine.succeed("curl -sf -o /dev/null -w '%{time_total}' http://127.0.0.1:8080/api/services"))
        assert took < 2.0, f"/api/services took {took}s"

    with subtest("quadlet is controlled through systemd and listed while stopped"):
        q = find(services(), "quadlet", "web.service")
        assert q["status"] == "running" and q["unit"] == "web.service", q
        machine.succeed("curl -sf -X POST http://127.0.0.1:8080/api/services/quadlet/web.service/stop")
        try:
            machine.wait_until_succeeds("systemctl show -p ActiveState --value web.service | grep -qx inactive", timeout=60)
            # The container is removed (--rm), but the quadlet must still be listed.
            machine.wait_until_succeeds("test -z \"$(podman ps -aq)\"", timeout=60)
        except Exception:
            print(machine.execute("systemctl status --no-pager web.service; podman ps -a; journalctl -u web.service --no-pager | tail -20")[1])
            raise
        q = find(services(), "quadlet", "web.service")
        assert q is not None and q["status"] == "stopped", q
        machine.succeed("curl -sf -X POST http://127.0.0.1:8080/api/services/quadlet/web.service/start")
        machine.wait_for_unit("web.service")
        machine.wait_until_succeeds("podman ps --format '{{.Names}}' | grep -qx systemd-web")
        # The quadlet's container must not show up again as a plain podman entry.
        res = services()
        assert find(res, "podman", "systemd-web") is None, res

    with subtest("reports host resources"):
        h = json.loads(machine.succeed("curl -sf http://127.0.0.1:8080/api/host"))
        assert h["cpu"]["cores"] >= 1, h
        assert h["memory"]["totalBytes"] > 0, h
        mounts = [fs["mountpoint"] for fs in h["filesystems"]]
        assert "/" in mounts, mounts
        assert "/nix/store" not in mounts, mounts  # bind mount of / is deduplicated

    with subtest("serves the frontend"):
        machine.succeed("curl -sf http://127.0.0.1:8080/ | grep -q '<title>Dashiboardie</title>'")
        # Nix store files have a 1970 mtime; index.html must be revalidated by
        # content (ETag), not by Last-Modified, or browsers keep old versions.
        headers = machine.succeed("curl -sfI http://127.0.0.1:8080/").lower()
        assert "cache-control: no-cache" in headers, headers
        assert "etag:" in headers and "last-modified" not in headers, headers
        asset = machine.succeed("curl -sf http://127.0.0.1:8080/ | grep -o '/assets/[^\"]*\\.js' | head -1").strip()
        assert "immutable" in machine.succeed(f"curl -sfI http://127.0.0.1:8080{asset}").lower(), asset

    with subtest("restart changes the unit's main PID"):
        before = machine.succeed("systemctl show -p MainPID --value hello").strip()
        machine.succeed("curl -sf -X POST http://127.0.0.1:8080/api/services/systemd/hello.service/restart")
        machine.wait_until_succeeds(
            f"test \"$(systemctl show -p MainPID --value hello)\" != {before} && systemctl is-active hello"
        )

    with subtest("logs come from the journal"):
        machine.succeed(
            "curl -sf 'http://127.0.0.1:8080/api/services/systemd/hello.service/logs?lines=20'"
            " | jq -e '.lines | map(select(contains(\"hello from the test unit\"))) | length > 0'"
        )

    with subtest("units outside the allowlist are refused"):
        code = machine.succeed(
            "curl -s -o /dev/null -w '%{http_code}' -X POST"
            " http://127.0.0.1:8080/api/services/systemd/not-listed.service/stop"
        )
        assert code == "404", code
        machine.succeed("systemctl is-active not-listed")
  '';
}
