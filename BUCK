# A list of available rules and their signatures can be found here: https://buck2.build/docs/prelude/globals/

export_file(
    name = "build_image_sh",
    src = "scripts/build-image.sh",
)

# Builds the glean container image via Nix's dockerTools.streamLayeredImage
# (see flake.nix), executed on the remote `nix-vm` builder over SSH -- this
# machine has no `nix` installed. The genrule itself stays a thin, uninvolved
# wrapper; all the actual rsync/ssh side effects live in build-image.sh.
genrule(
    name = "image",
    out = "glean-image.tar",
    # Populates $SRCDIR with a symlink farm mirroring the whole repo tree
    # (minus buck-out/.git/node_modules) -- build-image.sh rsyncs it to the
    # remote nix-vm builder, since it needs the full source, not just this
    # target's declared deps.
    srcs = glob(
        ["**/*"],
        exclude = [
            "buck-out/**",
            ".git/**",
            "web/node_modules/**",
            "web/.svelte-kit/**",
            "web/build/**",
        ],
    ),
    cmd = "chmod +x $(location :build_image_sh) && $(location :build_image_sh) $SRCDIR $OUT",
)
