"""Deploy Glean to Modal as a web Function.

Glean is two processes behind one port: the Go JSON API on loopback :8080 and
the SvelteKit (adapter-node) server on :3000, which proxies /api to it. Modal
exposes a single container port, so `@modal.web_server(3000)` fronts a launcher
that starts both, exactly like the Nix image's entrypoint does.

Storage is the awkward part. Glean's SQLite databases run in WAL mode, which
needs a real POSIX filesystem with working shared-memory locking -- a Modal
Volume is neither. So the live databases sit on the container's local disk and
the Volume is used as a snapshot store: restored on startup, and refreshed
periodically (and on shutdown) with `VACUUM INTO`, which takes a consistent
copy of a live database without stopping writers.

That makes this a single-container, single-writer deployment with a bounded
data-loss window (SNAPSHOT_INTERVAL). See deploy/modal/readme.md.
"""

import os
import pathlib
import shutil
import signal
import subprocess
import sys
import threading
import time

import modal

REPO_ROOT = pathlib.Path(__file__).parent.parent.parent

APP_NAME = "glean"

# Where the Volume is mounted, and where the live (WAL-mode) databases live.
VOLUME_PATH = "/data"
SNAPSHOT_DIR = f"{VOLUME_PATH}/db"
LIVE_DIR = "/livedb"
DB_BASE = f"{LIVE_DIR}/glean.db"

# Glean appends these suffixes to GLEAN_DB.
DB_SUFFIXES = ("_users", "_articles", "_recs")

# How often the live databases are snapshotted back to the Volume. This is the
# worst-case data-loss window if the container dies without running its exit
# handler.
SNAPSHOT_INTERVAL = 300

FRONTEND_PORT = 3000
API_PORT = 8080

image = (
    # The Go toolchain is the heavy dependency and go.mod pins go 1.26.2, so
    # start from the official Go image rather than installing it by hand.
    # add_python gives the container the Python that Modal's runtime needs.
    modal.Image.from_registry("golang:1.26-bookworm", add_python="3.12")
    .apt_install("curl", "unzip", "git", "ca-certificates", "sqlite3", "make")
    .run_commands(
        # SvelteKit's adapter-node output runs on Node; bun is only the
        # package manager / bundler, matching the Makefile.
        "curl -fsSL https://deb.nodesource.com/setup_22.x | bash -",
        "apt-get install -y nodejs",
        "curl -fsSL https://bun.sh/install | bash",
    )
    .add_local_dir(
        REPO_ROOT,
        remote_path="/src",
        copy=True,
        ignore=[
            ".git",
            "**/node_modules",
            "web/build",
            "web/.svelte-kit",
            "glean",
            "*.db*",
            "deploy/modal/__pycache__",
        ],
    )
    # `make build` computes the CGO_CFLAGS that mattn/go-sqlite3 and
    # sqlite-vec need (internal/db/include plus the go-sqlite3 module cache)
    # and applies the required `fts5` build tag. Reusing it keeps this
    # deployment honest about how the project actually builds.
    .run_commands(
        "cd /src && PATH=/root/.bun/bin:$PATH make web-install",
        "cd /src && PATH=/root/.bun/bin:$PATH make build",
    )
    .env({"PATH": "/root/.bun/bin:/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin"})
)

volume = modal.Volume.from_name("glean-data", create_if_missing=True)

# Holds GLEAN_SESSION_KEY and any OAuth / LLM / embedding credentials.
# Create with:
#   modal secret create glean-secrets GLEAN_SESSION_KEY=... GLEAN_FRONTEND_URL=...
secret = modal.Secret.from_name("glean-secrets")

app = modal.App(APP_NAME)


def _snapshot_once() -> None:
    """Copy each live database to the Volume with VACUUM INTO, then commit.

    VACUUM INTO writes a consistent copy of the database as of a read
    transaction, so it is safe to run while Glean is serving traffic. It
    refuses to overwrite, hence the temp-file-and-rename.
    """
    os.makedirs(SNAPSHOT_DIR, exist_ok=True)
    for suffix in DB_SUFFIXES:
        live = f"{DB_BASE}{suffix}"
        if not os.path.exists(live):
            continue
        # VACUUM INTO writes to local disk, never straight to the Volume:
        # it journals as it goes, and the Volume is the filesystem this whole
        # scheme exists to avoid putting SQLite on. Only the finished file is
        # copied across.
        staged = f"{LIVE_DIR}/snapshot{suffix}"
        if os.path.exists(staged):
            os.remove(staged)
        subprocess.run(
            ["sqlite3", live, f"VACUUM INTO '{staged}'"],
            check=True,
            capture_output=True,
        )
        shutil.copy2(staged, f"{SNAPSHOT_DIR}/glean.db{suffix}")
        os.remove(staged)
    volume.commit()


def _restore() -> None:
    """Seed the local disk from the Volume's snapshot, if there is one."""
    os.makedirs(LIVE_DIR, exist_ok=True)
    for suffix in DB_SUFFIXES:
        snap = f"{SNAPSHOT_DIR}/glean.db{suffix}"
        if os.path.exists(snap):
            shutil.copy2(snap, f"{DB_BASE}{suffix}")
            print(f"[glean] restored {snap}", flush=True)


def _snapshot_loop(stop: threading.Event) -> None:
    while not stop.wait(SNAPSHOT_INTERVAL):
        try:
            _snapshot_once()
            print("[glean] snapshotted databases to volume", flush=True)
        except Exception as exc:  # a failed snapshot must not kill the server
            print(f"[glean] snapshot failed: {exc}", file=sys.stderr, flush=True)


@app.function(
    image=image,
    volumes={VOLUME_PATH: volume},
    secrets=[secret],
    # Single writer: SQLite tolerates exactly one process writing these files,
    # and the snapshot scheme assumes one container owns the data.
    max_containers=1,
    # Glean's background workers (Jetstream consumer, PDS sync, clustering,
    # feed fetch) only run while a container is alive, so keep one alive.
    min_containers=1,
    scaledown_window=1200,
    timeout=24 * 60 * 60,
    cpu=2,
    memory=4096,
)
# One container serves every request; without this Modal would queue requests
# behind a single in-flight input.
@modal.concurrent(max_inputs=200)
@modal.web_server(FRONTEND_PORT, startup_timeout=300)
def serve() -> None:
    _restore()

    env = dict(os.environ)
    env["GLEAN_DB"] = DB_BASE
    env["GLEAN_ADDR"] = f"127.0.0.1:{API_PORT}"
    env["GLEAN_API_URL"] = f"http://127.0.0.1:{API_PORT}"
    # web_server routes external traffic to the container's interface, so the
    # SvelteKit server must not bind loopback only.
    env["HOST"] = "0.0.0.0"
    env["PORT"] = str(FRONTEND_PORT)
    env.setdefault("GLEAN_FRONTEND_URL", "http://localhost:3000")

    api = subprocess.Popen(["/src/glean"], env=env, cwd="/src")
    web = subprocess.Popen(
        ["node", "build/index.js"], env=env, cwd="/src/web"
    )

    stop = threading.Event()
    threading.Thread(target=_snapshot_loop, args=(stop,), daemon=True).start()

    def shutdown(_signum, _frame):
        stop.set()
        for proc in (web, api):
            proc.terminate()
        # Best-effort final snapshot so a graceful scaledown loses nothing.
        try:
            time.sleep(1)
            _snapshot_once()
            print("[glean] final snapshot written", flush=True)
        except Exception as exc:
            print(f"[glean] final snapshot failed: {exc}", file=sys.stderr, flush=True)
        sys.exit(0)

    signal.signal(signal.SIGTERM, shutdown)
    signal.signal(signal.SIGINT, shutdown)
