{
  description = "glean: Go API + SvelteKit frontend, packaged as a container image via dockerTools";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
  };

  outputs = { self, nixpkgs }:
    let
      systems = [ "x86_64-linux" "aarch64-linux" "aarch64-darwin" ];
      forEachSystem = f: nixpkgs.lib.genAttrs systems (system:
        f (import nixpkgs { inherit system; }));

      # --- Go API binary -----------------------------------------------------
      # Mirrors the historical Dockerfile's CGO setup: mattn/go-sqlite3 vendors
      # its own sqlite3 amalgamation, but sqlite-vec-go-bindings' cgo code
      # expects a plain `sqlite3.h` on the include path. internal/db/include/
      # sqlite3.h is that header; go-sqlite3's own module dir is added too so
      # the u_intN_t compat defines line up with the same sqlite3 version.
      mkGlean = pkgs: pkgs.buildGoModule {
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
      mkWebDeps = pkgs: pkgs.stdenvNoCC.mkDerivation {
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

      mkFrontend = pkgs: webDeps: pkgs.stdenvNoCC.mkDerivation {
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
      mkEntrypoint = pkgs: glean: frontend: pkgs.writeShellScriptBin "glean-entrypoint" ''
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
      packages = forEachSystem (pkgs:
        let
          glean = mkGlean pkgs;
          webDeps = mkWebDeps pkgs;
          frontend = mkFrontend pkgs webDeps;
          entrypoint = mkEntrypoint pkgs glean frontend;
        in
        {
          inherit glean frontend webDeps;
          default = glean;
        }
        # dockerTools images are Linux-only; on darwin the attribute is simply
        # absent rather than a derivation that fails at build time.
        // nixpkgs.lib.optionalAttrs pkgs.stdenv.hostPlatform.isLinux {
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
        });

      # `nix run .#glean` runs the API; `nix run .#image > glean-image.tar`
      # streams the docker-archive tarball to stdout.
      apps = forEachSystem (pkgs:
        let
          system = pkgs.stdenv.hostPlatform.system;
          pkgsFor = self.packages.${system};
        in
        {
          default = { type = "app"; program = "${pkgsFor.glean}/bin/glean"; };
          glean = { type = "app"; program = "${pkgsFor.glean}/bin/glean"; };
        }
        // nixpkgs.lib.optionalAttrs pkgs.stdenv.hostPlatform.isLinux {
          # streamLayeredImage's output *is* the streaming script.
          image = { type = "app"; program = "${pkgsFor.image}"; };
        });

      # Everything the Makefile targets need: Go with cgo, bun/node for the
      # frontend, and the image/lexicon tooling.
      devShells = forEachSystem (pkgs: {
        default = pkgs.mkShell {
          packages = with pkgs; [
            go
            gcc
            gnumake
            bun
            nodejs_22
            sqlite
            golangci-lint
            rsync
            openssh
          ];

          # Same include flags the Makefile computes, so `go build -tags fts5`
          # works inside the shell without the module cache lookup.
          shellHook = ''
            export CGO_ENABLED=1
            export CGO_CFLAGS="-I$PWD/internal/db/include -I$(go env GOMODCACHE)/$(grep 'mattn/go-sqlite3' go.mod | awk '{print $1 "@" $2}')"
          '';
        };
      });

      # `nix flake check` builds the binary and the frontend and runs the Go
      # tests.
      checks = forEachSystem (pkgs:
        let
          system = pkgs.stdenv.hostPlatform.system;
        in
        {
          glean = self.packages.${system}.glean;
          frontend = self.packages.${system}.frontend;
          tests = (mkGlean pkgs).overrideAttrs (_: {
            pname = "glean-tests";
            doCheck = true;
          });
        });

      formatter = forEachSystem (pkgs: pkgs.nixpkgs-fmt);
    };
}
