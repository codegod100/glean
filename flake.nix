{
  description = "glean: Go API + SvelteKit frontend, packaged as a container image via dockerTools";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
  };

  outputs = { self, nixpkgs }:
    let
      system = "x86_64-linux";
      pkgs = import nixpkgs { inherit system; };
      lib = pkgs.lib;

      # --- Go API binary -----------------------------------------------------
      # Mirrors the Dockerfile's CGO setup: mattn/go-sqlite3 vendors its own
      # sqlite3 amalgamation, but sqlite-vec-go-bindings' cgo code expects a
      # plain `sqlite3.h` on the include path. internal/db/include/sqlite3.h
      # is that header; go-sqlite3's own module dir is added too so the
      # u_intN_t compat defines line up with the same sqlite3 version.
      glean = pkgs.buildGoModule {
        pname = "glean";
        version = "0.0.1";
        src = ./.;
        vendorHash = "sha256-EYJD4hBs4RT9Qwccqw0G58Hdn7TtJKfSAL4CHQVLXO0=";

        env.CGO_ENABLED = "1";
        tags = [ "fts5" ];
        ldflags = [ "-s" "-w" ];

        # buildGoModule builds with `-mod=vendor` against a `vendor/` tree it
        # generates itself (that's what vendorHash actually pins), so the
        # go-sqlite3 header lives under vendor/, not $(go env GOMODCACHE).
        preBuild = ''
          export CGO_CFLAGS="-I$PWD/internal/db/include -I$PWD/vendor/github.com/mattn/go-sqlite3 -Du_int8_t=uint8_t -Du_int16_t=uint16_t -Du_int64_t=uint64_t"
        '';

        doCheck = false;
      };

      # --- SvelteKit frontend (adapter-node) ---------------------------------
      # bun install needs network access, which the nix sandbox forbids for a
      # normal derivation -- split into a fixed-output "fetch deps" step
      # (network allowed, output hash pinned) and a sandboxed "build" step
      # that only ever sees the already-fetched node_modules.
      webDeps = pkgs.stdenvNoCC.mkDerivation {
        pname = "glean-web-deps";
        version = "0.0.1";
        src = ./web;
        nativeBuildInputs = [ pkgs.bun pkgs.cacert ];

        buildPhase = ''
          export HOME=$TMPDIR
          export SSL_CERT_FILE="${pkgs.cacert}/etc/ssl/certs/ca-bundle.crt"
          bun install --frozen-lockfile
        '';
        installPhase = ''
          mkdir -p $out
          cp -r node_modules $out/node_modules
        '';

        outputHashMode = "recursive";
        outputHashAlgo = "sha256";
        outputHash = "sha256-q/VJbH1rWCsxf3A3RYb22t2RrtjUm4xCCrCZkLkAFFY=";
      };

      frontend = pkgs.stdenvNoCC.mkDerivation {
        pname = "glean-web";
        version = "0.0.1";
        src = ./web;
        # nodejs_22 is needed so patchShebangs below has a `node` binary on
        # PATH to point node_modules/.bin scripts' shebangs at.
        nativeBuildInputs = [ pkgs.bun pkgs.nodejs_22 ];

        buildPhase = ''
          export HOME=$TMPDIR
          cp -r ${webDeps}/node_modules .
          chmod -R u+w node_modules
          # node_modules/.bin scripts have `#!/usr/bin/env node` shebangs;
          # /usr/bin/env doesn't exist inside the nix sandbox, so patch them
          # to point at the real node on the store path.
          patchShebangs node_modules
          bun run build
        '';
        installPhase = ''
          mkdir -p $out
          cp -r build $out/build
          cp package.json $out/package.json
        '';
      };

      # --- Container assembly -------------------------------------------------
      # Runs both processes the same way the Dockerfile's CMD did: the Go API
      # in the background on loopback:8080, the SvelteKit Node server in front
      # on $PORT, proxying /api to it.
      entrypoint = pkgs.writeShellScriptBin "glean-entrypoint" ''
        set -euo pipefail
        export GLEAN_ADDR="127.0.0.1:8080"
        export GLEAN_API_URL="http://127.0.0.1:8080"
        ${glean}/bin/glean &
        GO_PID=$!
        trap 'kill "$GO_PID" 2>/dev/null || true' INT TERM
        exec ${pkgs.nodejs_22}/bin/node ${frontend}/build/index.js
      '';
    in
    {
      packages.${system} = {
        inherit glean frontend webDeps;

        image = pkgs.dockerTools.streamLayeredImage {
          name = "glean";
          tag = "latest";
          contents = [ pkgs.cacert pkgs.tzdata pkgs.dockerTools.fakeNss entrypoint ];
          config = {
            Cmd = [ "${entrypoint}/bin/glean-entrypoint" ];
            Env = [ "PORT=3000" ];
            ExposedPorts = { "3000/tcp" = { }; };
          };
        };
      };
    };
}
