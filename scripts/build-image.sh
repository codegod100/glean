#!/bin/bash
# Builds the glean container image with Nix (pkgs.dockerTools.streamLayeredImage,
# defined in flake.nix) on a remote builder over SSH -- this dev machine has no
# `nix` installed, so the actual build can't happen locally. Side-effecting (rsync
# + ssh) on purpose: buck2's genrule for :image just calls this script and stays
# pure/uninvolved in how the remote build actually happens.
#
# Usage: scripts/build-image.sh <srcdir> <output-tar-path>
set -euo pipefail
# $SRCDIR is populated by the genrule's `srcs = glob(...)` as a symlink farm
# mirroring the repo tree -- NOT this script's own location (which, after
# export_file, is a buck-out copy) and NOT $PWD (buck2 runs genrule cmds with
# cwd at a srcs scratch dir, not the project root).
DIR="$1"
OUT="$2"

NIX_VM="${GLEAN_NIX_VM:-nix-vm}"
REMOTE_DIR="${GLEAN_NIX_VM_DIR:-~/builds/glean}"

echo "==> syncing source to $NIX_VM:$REMOTE_DIR" >&2
ssh "$NIX_VM" "mkdir -p $REMOTE_DIR"
# -L dereferences $SRCDIR's symlinks (they point back at buck2's srcs farm,
# which doesn't exist on the remote host) so real file content gets copied.
rsync -aL --delete \
  --exclude .git \
  --exclude buck-out \
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
