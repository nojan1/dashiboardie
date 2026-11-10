# NixOS VM test: run `nix flake check` or `nix build .#checks.x86_64-linux.module`.
{ pkgs, module }:

pkgs.testers.runNixOSTest {
  name = "dashiboardie";

  nodes.machine = {
    imports = [ module ];

    services.dashiboardie = {
      enable = true;
      systemdUnits = [ "hello" ];
    };

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

    with subtest("lists only the configured unit"):
        res = services()
        ids = [s["id"] for s in res["services"]]
        assert ids == ["hello.service"], ids
        assert res["services"][0]["status"] == "running", res

    with subtest("serves the frontend"):
        machine.succeed("curl -sf http://127.0.0.1:8080/ | grep -q '<title>Dashiboardie</title>'")

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
