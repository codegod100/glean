"""Deploy the reader to Modal: the Nim binary, serving the Flutter web build.

One container, one port. The Nim server handles both the API and the client's
static files, so there is nothing to proxy and no second origin to arrange
CORS for.

Storage is the awkward part, as it always is on a serverless host. SQLite
runs in WAL mode, which wants a real POSIX filesystem with working shared
memory -- a Modal Volume is neither. So the live databases sit on the
container's local disk and the Volume is a snapshot store: restored on start,
refreshed periodically and on shutdown with `VACUUM INTO`, which copies a
live database consistently without stopping writers.

That makes this single-container and single-writer, with a data-loss window
bounded by SNAPSHOT_INTERVAL. For a personal reader whose contents can be
refetched from the feeds, that is a fair trade.
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

REPO = pathlib.Path(__file__).parent.parent

APP_NAME = "pulseboard"
PORT = 8080
PUBLIC_URL = "https://codegod100--pulseboard-serve.modal.run"

VOLUME_PATH = "/data"
SNAPSHOT_DIR = f"{VOLUME_PATH}/db"
LIVE_DIR = "/livedb"
DB_BASE = f"{LIVE_DIR}/pulseboard"

# The reader opens <base>_users and attaches <base>_articles.
DB_SUFFIXES = ("_users", "_articles")

# Worst case a crash costs, if the container dies without its exit handler.
SNAPSHOT_INTERVAL = 300

image = (
    modal.Image.debian_slim(python_version="3.12")
    .apt_install(
        "curl", "xz-utils", "gcc", "sqlite3", "libsqlite3-dev",
        "ca-certificates", "nodejs", "npm",
    )
    .run_commands(
        # choosenim is the supported installer and pins a known version;
        # Debian's Nim is far behind what this code needs.
        "curl -sSf https://nim-lang.org/choosenim/init.sh | sh -s -- -y",
    )
    .env({"PATH": "/root/.nimble/bin:/usr/local/bin:/usr/bin:/bin"})
    .add_local_dir(
        REPO / "spike" / "nim",
        remote_path="/src",
        copy=True,
        ignore=["nimcache", "pulseboard", "*_probe", "*.db*"],
    )
    # The Flutter web build is committed output rather than built here:
    # installing Flutter in the image would add gigabytes and minutes for
    # something the developer machine has already produced.
    .add_local_dir(REPO / "app" / "build" / "web", remote_path="/web", copy=True)
    .run_commands("cd /src/auth && npm ci --omit=dev")
    .run_commands("cd /src && nim c --threads:on -d:release -d:ssl --hints:off -o:/usr/local/bin/pulseboard pulseboard.nim")
)

volume = modal.Volume.from_name("pulseboard-data", create_if_missing=True)

app = modal.App(APP_NAME)


def _snapshot_once() -> None:
    """Copy each live database to the Volume, via a local staging file.

    VACUUM INTO journals as it writes, so it is pointed at local disk rather
    than the Volume -- the filesystem this whole scheme exists to keep SQLite
    off. Only the finished file is copied across.
    """
    os.makedirs(SNAPSHOT_DIR, exist_ok=True)
    for suffix in DB_SUFFIXES:
        live = f"{DB_BASE}{suffix}"
        if not os.path.exists(live):
            continue
        staged = f"{LIVE_DIR}/snapshot{suffix}"
        if os.path.exists(staged):
            os.remove(staged)
        subprocess.run(
            ["sqlite3", live, f"VACUUM INTO '{staged}'"], check=True, capture_output=True
        )
        shutil.copy2(staged, f"{SNAPSHOT_DIR}/pulseboard{suffix}")
        os.remove(staged)
    volume.commit()


def _restore() -> None:
    os.makedirs(LIVE_DIR, exist_ok=True)
    for suffix in DB_SUFFIXES:
        snap = f"{SNAPSHOT_DIR}/pulseboard{suffix}"
        if os.path.exists(snap):
            shutil.copy2(snap, f"{DB_BASE}{suffix}")
            print(f"[pulseboard] restored {snap}", flush=True)


def _snapshot_loop(stop: threading.Event) -> None:
    while not stop.wait(SNAPSHOT_INTERVAL):
        try:
            _snapshot_once()
        except Exception as exc:  # a failed snapshot must not kill the server
            print(f"[pulseboard] snapshot failed: {exc}", file=sys.stderr, flush=True)


@app.function(
    image=image,
    volumes={VOLUME_PATH: volume},
    # SQLite tolerates one writer, and the snapshot scheme assumes one
    # container owns the data. This is correctness, not tuning.
    max_containers=1,
    scaledown_window=900,
    timeout=24 * 60 * 60,
)
@modal.concurrent(max_inputs=50)
@modal.web_server(PORT, startup_timeout=120)
def serve() -> None:
    _restore()

    env = dict(os.environ)
    env["PULSEBOARD_DB"] = DB_BASE
    env["PULSEBOARD_PORT"] = str(PORT)
    env["PULSEBOARD_WEB"] = "/web"
    env["PULSEBOARD_PUBLIC_URL"] = PUBLIC_URL
    env["PULSEBOARD_OAUTH_ROOT"] = f"{VOLUME_PATH}/oauth"
    env["PULSEBOARD_OAUTH_HELPER"] = "/src/auth/atproto-oauth.mjs"

    proc = subprocess.Popen(["/usr/local/bin/pulseboard"], env=env)

    stop = threading.Event()
    threading.Thread(target=_snapshot_loop, args=(stop,), daemon=True).start()

    def shutdown(_signum, _frame):
        stop.set()
        proc.terminate()
        try:
            time.sleep(1)
            _snapshot_once()
            print("[pulseboard] final snapshot written", flush=True)
        except Exception as exc:
            print(f"[pulseboard] final snapshot failed: {exc}", file=sys.stderr, flush=True)
        sys.exit(0)

    signal.signal(signal.SIGTERM, shutdown)
    signal.signal(signal.SIGINT, shutdown)
