# Deploying to Modal

This is an **additional** target alongside the Nix → image → Railway pipeline in
[`docs/deploy.md`](../../docs/deploy.md), which is unchanged and remains the
primary deployment.

```
modal deploy deploy/modal/modal_app.py
```

Live at <https://codegod100--glean-serve.modal.run>.

## How it works

Modal runs Python, so `deploy/modal/modal_app.py` builds a container image that
carries the Go toolchain, bun and Node, and runs the project's own `make build`
inside it — same `fts5` build tag, same `CGO_CFLAGS`, so the Modal build is the
build the Makefile describes rather than a reimplementation of it.

At runtime a `@modal.web_server(3000)` Function starts both processes the way
the Nix entrypoint does: the Go API on `127.0.0.1:8080`, the SvelteKit
`adapter-node` server on `0.0.0.0:3000` in front of it, proxying `/api`.

## Storage

Glean's SQLite databases run in WAL mode. WAL needs a real POSIX filesystem
with working shared-memory locking, and a Modal Volume is neither — so the
Volume cannot hold the live databases.

Instead the live databases sit on the container's local disk (`/livedb`) and the
Volume (`glean-data`, at `/data`) is a **snapshot store**:

- on container start, the newest snapshot is copied to local disk;
- every `SNAPSHOT_INTERVAL` (300s) and on `SIGTERM`, each database is copied
  back with `VACUUM INTO`, which takes a consistent copy while Glean keeps
  serving, and the Volume is committed.

The consequence is a bounded data-loss window: if a container dies without
running its exit handler, up to five minutes of writes are gone. Lower
`SNAPSHOT_INTERVAL` to trade I/O for a tighter window.

## Why one container

`max_containers=1` is not tuning, it is a correctness requirement. Two
containers would mean two SQLite writers over two independent copies of the
data, and the Volume's last-write-wins semantics would silently discard one of
them. `min_containers=0` lets that container scale to zero when idle. Glean's
background workers — the Jetstream websocket consumer and the PDS sync /
clustering / feed-fetch loops — only run while a container exists, so they
pause once it scales down and resume when the next request wakes one.

## Secrets

`GLEAN_SESSION_KEY` and `GLEAN_FRONTEND_URL` live in the `glean-secrets` Modal
Secret, never in the repo:

```
modal secret create glean-secrets \
  GLEAN_SESSION_KEY=$(openssl rand -hex 32) \
  GLEAN_FRONTEND_URL=https://<workspace>--glean-serve.modal.run
```

`GLEAN_FRONTEND_URL` must be the URL users actually see — the OAuth callback is
derived from it. Add the optional keys from [`.env.example`](../../.env.example)
to the same Secret to enable them:

- `GLEAN_OAUTH_CLIENT_ID` — required for real ATProto login. Until it is set,
  `/api/oauth/client-metadata` returns `{"error":"localhost client"}` and only
  the logged-out views work.
- `GLEAN_EMBED_*` — content-based recommendations.
- `GLEAN_LLM_*` — language detection and digests.

## Caveats

Modal's model is autoscaling ephemeral containers. Glean is a stateful,
single-writer, always-on server. The mismatch is real and this deployment
manages it rather than resolving it:

- **One container, scaled to zero when idle.** No horizontal scaling, and
  with `min_containers=0` the first request after an idle period pays a cold
  start while the background workers are stopped in between.
- **Snapshot durability, not continuous durability.** See the window above.
- **Container churn costs data.** Any restart that skips the exit handler — a
  hard kill, an OOM, a preemption — loses the interval.
- **Redeploys are not zero-downtime.** `max_containers=1` prevents Modal from
  bringing up a replacement to shift traffic across.

If Glean's data grows past what fits comfortably on local disk, or writes need
to survive every crash, the answer is a networked database rather than a
different Modal configuration.
