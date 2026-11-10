{
  description = "dashiboardie: a dashboard for podman, Quadlet, incus, NixOS container and systemd services";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

  outputs =
    { self, nixpkgs }:
    let
      systems = [
        "x86_64-linux"
        "aarch64-linux"
      ];
      forAllSystems = f: nixpkgs.lib.genAttrs systems (system: f nixpkgs.legacyPackages.${system});
      build = pkgs: pkgs.callPackage ./nix/package.nix { };
    in
    {
      packages = forAllSystems (pkgs: rec {
        dashiboardie = build pkgs;
        default = dashiboardie;
      });

      overlays.default = final: _prev: { dashiboardie = build final; };

      nixosModules.default = import ./nix/module.nix { defaultPackage = build; };

      checks = forAllSystems (pkgs: {
        package = self.packages.${pkgs.stdenv.hostPlatform.system}.default;
        module = import ./nix/test.nix {
          inherit pkgs;
          module = self.nixosModules.default;
        };
      });

      devShells = forAllSystems (pkgs: {
        default = import ./shell.nix { inherit pkgs; };
      });

      formatter = forAllSystems (pkgs: pkgs.nixfmt);
    };
}
