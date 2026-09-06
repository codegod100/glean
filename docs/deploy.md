# Deploying

The canonical build is the Nix flake (`flake.nix`): `nix build .#image` produces
the container image, and `make image-push IMAGE=<ref>` publishes it. That path
still works and is the reproducible one.

## Railway (production)

The `glean` service in the `glean` project builds **from source** on
`railway up`, rather than running a prebuilt image. Railway ignores in-repo
build config here — `railway.toml`, `nixpacks.toml` and a start script in
`scripts/` were all silently skipped — so the configuration lives in the
service settings and environment variables instead:

| Setting | Value |
|---|---|
| Builder | `NIXPACKS` (the Railpack default cannot build this repo) |
| Build command | `cd web && bun install && cd .. && make build` |
| Start command | `sh -c "GLEAN_API_URL=http://127.0.0.1:8080 GLEAN_ADDR=127.0.0.1:8080 /app/glean & exec node /app/web/build/index.js"` |
| `NIXPACKS_PKGS` | `bun nodejs gnumake gcc gawk gnugrep` |
| `CGO_ENABLED` | `1` |

Notes on why each is needed:

- **`CGO_ENABLED=1`** — `sqlite-vec-go-bindings/cgo` and `mattn/go-sqlite3` are
  cgo packages. The builder defaults to cgo off, which fails with "build
  constraints exclude all Go files".
- **`NIXPACKS_PKGS`** — the Go provider installs only Go. The frontend needs
  `bun` to build and `nodejs` to serve; `make build` needs make/grep/awk. This
  mirrors the dependency list in `.tangled/workflows`. Use `nodejs`, not
  `nodejs_22`: the pinned nixpkgs has no such attribute.
- **Build command** — the builder's default `go build -o out` skips both the
  `fts5` tag and the SvelteKit build. `make build` does both.
- **Start command** — mirrors the flake's `mkEntrypoint`: the Go API on
  loopback:8080 with the SvelteKit Node server in front on `$PORT`. It is
  inline rather than a script file because the runtime image does not carry
  `scripts/`.

Deploy with `railway up` from the repo root.
