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

  # Units from both options, each as "name" or "name=url" for -systemd-units.
  unitSpecs = map (u: if cfg.systemdUnitUrls ? ${u} then "${u}=${cfg.systemdUnitUrls.${u}}" else u) (
    lib.unique (cfg.systemdUnits ++ lib.attrNames cfg.systemdUnitUrls)
  );
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

    systemdUnitUrls = lib.mkOption {
      type = lib.types.attrsOf lib.types.str;
      default = { };
      example = {
        jellyfin = "https://jellyfin.example.com";
      };
      description = ''
        Links shown for systemd units, keyed by unit name as written in
        `systemdUnits`. Units listed here are shown even if they are missing
        from `systemdUnits`.
      '';
    };

    systemdTimers = lib.mkOption {
      type = lib.types.listOf lib.types.str;
      default = [ ];
      example = [
        "restic-backups-daily"
        "nix-gc"
      ];
      description = ''
        systemd timers to show with their last run, its result and the next
        run. A name without a suffix gets `.timer` added. Logs can be read
        for these timers and the services they trigger.
      '';
    };

    updateCheckInterval = lib.mkOption {
      type = lib.types.str;
      default = "6h";
      example = "1h";
      description = ''
        How often to check registries for newer images of podman containers
        and Quadlets (`podman auto-update --dry-run`), as a Go duration.
        Only containers with an auto-update policy are checked (for
        Quadlets, `AutoUpdate=registry`). `"0"` disables the check.
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
    assertions = [
      {
        assertion = lib.all (s: !(lib.hasInfix "," s)) (unitSpecs ++ cfg.systemdTimers);
        message = "services.dashiboardie: systemd unit and timer names and URLs must not contain commas.";
      }
    ];

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
          ++ lib.optionals (unitSpecs != [ ]) [
            "-systemd-units"
            (lib.concatStringsSep "," unitSpecs)
          ]
          ++ lib.optionals (cfg.systemdTimers != [ ]) [
            "-systemd-timers"
            (lib.concatStringsSep "," cfg.systemdTimers)
          ]
          ++ [
            "-update-check-interval"
            cfg.updateCheckInterval
          ]
          ++ cfg.extraArgs
        );
        # Runs as root: starting/stopping system units, rootful podman and
        # incus all require it.
        Restart = "on-failure";
        NoNewPrivileges = true;
        # No ProtectHome/PrivateTmp: they give the service its own mount
        # namespace, which would hide or change the filesystems the host
        # resource overview reports.
      };
    };
  };
}
