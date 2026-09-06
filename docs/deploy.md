# Deploying

The Nix flake is the build. `nix build .#image` (flake.nix, dockerTools
`streamLayeredImage`) produces the container image, and the same command runs
locally and in CI, so a local build and a deployed build are the same artifact.

## Pipeline

Railway has no GitLab repo integration — it deploys from a GitHub repo, a local
directory, or a container image — so it cannot watch this repo. `.gitlab-ci.yml`
closes that gap on every push to `main`:

1. `build_image` builds `.#image` with Nix and pushes it to this project's own
   container registry as `$CI_REGISTRY_IMAGE:$CI_COMMIT_SHORT_SHA` (and
   `:latest`). skopeo pushes the docker-archive directly, so no Docker daemon
   is needed on the runner.
2. `deploy_railway` points the Railway service at that tag
   (`serviceInstanceUpdate`) and rolls it out (`serviceInstanceDeploy`).

The project is public, so the registry allows anonymous pulls and Railway needs
no registry credentials. Making it private again would require registry
credentials on the Railway side, and a paid plan — private registry sources are
a Pro feature.

### Required CI variable

`deploy_railway` needs `RAILWAY_TOKEN`, a Railway **project token** for the
`glean` project, set under Settings → CI/CD → Variables (protected, masked).
Without it the deploy job is skipped rather than failing: the image is still
built and published, and can be rolled out by hand.

The service and environment IDs are not secret and are inlined in
`.gitlab-ci.yml`.

## Deploying by hand

```
make image-push IMAGE=registry.gitlab.com/nandithebull/glean:<tag>
```

Then point the service at `<tag>` in the Railway dashboard, or re-run the
`deploy_railway` job.

## History

The service briefly built from source on `railway up` with Nixpacks. That
needed `CGO_ENABLED=1` (the sqlite cgo packages), a `NIXPACKS_PKGS` toolchain
list, and an overridden build command, because Railway ignores in-repo
`railway.toml` / `nixpacks.toml` here. It also deployed the working tree rather
than a commit. The image pipeline above replaces it; those service settings
have been removed.
