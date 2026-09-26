# Deploying

Production deploys automatically after a pull request is merged into `main`. The
[`Deploy` workflow](../.github/workflows/deploy.yml) tests and builds the
Flutter web client from the merged commit, then deploys the complete application
to Modal. Direct pushes do not deploy; use the workflow's manual trigger when a
deployment is needed without a pull request.

Before merging the workflow for the first time, add these GitHub Actions
secrets to the repository (or to its `production` environment):

- `MODAL_TOKEN_ID`
- `MODAL_TOKEN_SECRET`

Create a Modal token with `modal token new`; use its token ID and secret for
the two values. The workflow can also be run manually from the Actions tab.

## Manual deployment

```
cd app && flutter build web --release --dart-define=PULSEBOARD_BASE_URL=
cd ..
modal deploy deploy/modal_app.py
```

Then open the deployed URL and sign in with an AT Protocol handle. The OAuth
callback is `<your-app>.modal.run/auth/callback`; the public client metadata is
served at `/oauth-client-metadata.json`.

## One container, one port

The Nim binary serves the API *and* the Flutter web build, so there is
nothing to proxy and no second origin to arrange CORS for. The client is
built with an empty base URL and uses relative paths.

The web build is generated before Modal builds the image: GitHub Actions does
this automatically, while a manual deployment must run the Flutter command
above first. Installing Flutter in the Modal image would add gigabytes and
minutes to the image build.

## Authentication

Every UI and API route except health and the OAuth endpoints requires an
authenticated browser session. OAuth protocol state is kept under
`/data/oauth`; application sessions live in the users SQLite database and
expire after 12 hours. The OAuth access credential is revoked immediately
after the DID has been verified.

The session is an HttpOnly cookie, not a bearer token, so the client has
nothing to attach. The users database is copied to the Volume within seconds
of a login, rather than on the five-minute schedule, so a session survives a
redeploy. When a session is missing or has expired, the API answers 401 and the
web client sends the browser back to `/auth/login`.

## Storage, and what it costs

SQLite runs in WAL mode, which wants a real POSIX filesystem with working
shared memory. A Modal Volume is neither. So the live databases sit on the
container's local disk and the Volume is a snapshot store: restored on start,
refreshed every five minutes and on shutdown with `VACUUM INTO`, which copies
a live database consistently without stopping writers.

The consequences, plainly:

- **Up to five minutes of reads can be lost** if a container dies without
  running its exit handler — an OOM, a preemption, a hard kill.
- **`max_containers=1` is correctness, not tuning.** Two containers would be
  two SQLite writers over two divergent copies, and the Volume's
  last-write-wins would quietly discard one.
- **Redeploys are not zero-downtime**, for the same reason.

For a personal reader whose contents can be refetched from the feeds, that is
a fair trade. For anything where the data is the point, it is not.
