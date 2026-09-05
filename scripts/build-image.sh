#!/bin/bash
# Builds the glean container image with Nix (pkgs.dockerTools.streamLayeredImage,
# defined in flake.nix). If `nix` is available locally the build happens here;
# otherwise the repo is rsync'd to a remote builder over SSH and built there,
# with the resulting docker-archive tarball streamed back.
#
# Usage: scripts/build-image.sh [output-tar-path]   (default: glean-image.tar)
set -euo pipefail

OUT="${1:-glean-image.tar}"
DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

if command -v nix >/dev/null 2>&1; then
  echo "==> building .#image locally" >&2
  nix build "$DIR#image" --print-out-paths --no-link -L | {
    read -r store_path
    echo "==> streaming image (docker-archive tar) to $OUT" >&2
    "$store_path" > "$OUT"
  }
  echo "==> wrote $OUT" >&2
  exit 0
fi

NIX_VM="${GLEAN_NIX_VM:-nix-vm}"
REMOTE_DIR="${GLEAN_NIX_VM_DIR:-~/builds/glean}"

echo "==> no local nix; syncing source to $NIX_VM:$REMOTE_DIR" >&2
ssh "$NIX_VM" "mkdir -p $REMOTE_DIR"
rsync -a --delete \
  --exclude .git \
  --exclude web/node_modules \
  --exclude web/.svelte-kit \
  --exclude web/build \
  -e ssh \
  "$DIR/" "$NIX_VM:$REMOTE_DIR/"

echo "==> building .#image on $NIX_VM" >&2
STORE_PATH="$(ssh "$NIX_VM" "cd $REMOTE_DIR && nix build .#image --print-out-paths --no-link -L")"

echo "==> streaming image (docker-archive tar) back from $NIX_VM" >&2
ssh "$NIX_VM" "$STORE_PATH" > "$OUT"

echo "==> wrote $OUT" >&2
