{
  lib,
  buildGoModule,
  buildNpmPackage,
  makeWrapper,
}:

let
  version = "0.1.0";

  frontend = buildNpmPackage {
    pname = "dashiboardie-frontend";
    inherit version;
    src = lib.fileset.toSource {
      root = ../frontend;
      fileset = lib.fileset.difference ../frontend (
        lib.fileset.unions [
          (lib.fileset.maybeMissing ../frontend/node_modules)
          (lib.fileset.maybeMissing ../frontend/dist)
        ]
      );
    };
    # Update after changing frontend/package-lock.json: set to lib.fakeHash,
    # build, and copy the hash from the error.
    npmDepsHash = "sha256-QV0JT4A6nG+hQ1HOty+DKAbMnH+mqXrcNoJAyOMfnDw=";
    installPhase = ''
      runHook preInstall
      cp -r dist $out
      runHook postInstall
    '';
  };
in
buildGoModule {
  pname = "dashiboardie";
  inherit version;
  src = ../backend;

  # No third-party Go dependencies.
  vendorHash = null;
  subPackages = [ "cmd/dashiboardie" ];
  ldflags = [
    "-s"
    "-w"
  ];

  nativeBuildInputs = [ makeWrapper ];

  # Serve the bundled frontend by default. Go's flag package lets a later
  # -static on the command line override this.
  postInstall = ''
    wrapProgram $out/bin/dashiboardie --add-flags "-static ${frontend}"
  '';

  passthru = { inherit frontend; };

  meta = {
    description = "Dashboard for podman, Quadlet, incus, NixOS container and systemd services";
    homepage = "https://github.com/nojan1/dashiboardie";
    mainProgram = "dashiboardie";
    platforms = lib.platforms.linux;
  };
}
