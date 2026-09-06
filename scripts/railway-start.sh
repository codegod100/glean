#!/usr/bin/env bash
# Runs the same two processes as the flake's image entrypoint (flake.nix,
# mkEntrypoint): the Go API on loopback:8080 with the SvelteKit Node server in
# front on $PORT, proxying /api to it. Used when Railway builds from source
# instead of running the prebuilt Nix image.
set -euo pipefail

export GLEAN_ADDR="127.0.0.1:8080"
export GLEAN_API_URL="http://127.0.0.1:8080"

./glean &
GO_PID=$!
trap 'kill "$GO_PID" 2>/dev/null || true' INT TERM

exec node web/build/index.js
