# NixOS module for dashiboardie. Import it via the flake's
# nixosModules.default; the package defaults to the flake's own build.
{ defaultPackage }:
{
  config,
  lib,
  pkgs,
  ...
}:

let
  cfg = config.services.dashiboardie;
in
{
  options.services.dashiboardie = {
    enable = lib.mkEnableOption "dashiboardie service dashboard";

    package = lib.mkOption {
      type = lib.types.package;
      default = defaultPackage pkgs;
      defaultText = lib.literalExpression "dashiboardie.packages.\${system}.default";
      description = "The dashiboardie package to use.";
    };

    listenAddress = lib.mkOption {
      type = lib.types.str;
      default = "127.0.0.1:8080";
      description = ''
        Address to listen on. There is no authentication, so keep this on
        localhost and put an access-controlled reverse proxy in front.
      '';
    };

    systemdUnits = lib.mkOption {
      type = lib.types.listOf lib.types.str;
      default = [ ];
      example = [
        "nginx"
        "postgresql.service"
      ];
      description = ''
        systemd units to show and control. Units not listed here are never
        touched. A name without a suffix gets `.service` added.
      '';
    };

    podman = lib.mkOption {
      type = lib.types.bool;
      default = config.virtualisation.podman.enable;
      defaultText = lib.literalExpression "config.virtualisation.podman.enable";
      description = "Show rootful podman containers and Quadlets.";
    };

    incus = lib.mkOption {
      type = lib.types.bool;
      default = config.virtualisation.incus.enable;
      defaultText = lib.literalExpression "config.virtualisation.incus.enable";
      description = "Show incus instances.";
    };

    extraArgs = lib.mkOption {
      type = lib.types.listOf lib.types.str;
      default = [ ];
      description = "Extra command-line arguments.";
    };
  };

  config = lib.mkIf cfg.enable {
    systemd.services.dashiboardie = {
      description = "dashiboardie service dashboard";
      wantedBy = [ "multi-user.target" ];
      after = [ "network.target" ];

      # Providers are enabled based on which CLIs are on PATH.
      path = [
        config.systemd.package
      ]
      ++ lib.optional cfg.podman config.virtualisation.podman.package
      ++ lib.optional cfg.incus config.virtualisation.incus.package;

      serviceConfig = {
        ExecStart = lib.escapeShellArgs (
          [
            (lib.getExe cfg.package)
            "-addr"
            cfg.listenAddress
          ]
          ++ lib.optionals (cfg.systemdUnits != [ ]) [
            "-systemd-units"
            (lib.concatStringsSep "," cfg.systemdUnits)
          ]
          ++ cfg.extraArgs
        );
        # Runs as root: starting/stopping system units, rootful podman and
        # incus all require it.
        Restart = "on-failure";
        NoNewPrivileges = true;
        ProtectHome = true;
        PrivateTmp = true;
      };
    };
  };
}
