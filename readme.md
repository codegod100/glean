# Glean

A social RSS reader built on the AT Protocol. Sign in with your Bluesky/Atmosphere account, subscribe to feeds, and discover what like-minded readers are into.

**Try it at [glean.at](https://glean.at).**

Your subscriptions live as records on your PDS. You own them. If Glean goes away, your data doesn't.

## What you get

- RSS, Atom, and JSON Feed support with a keyboard-driven reading interface
- Highlights, notes, tags, and ratings on articles
- [margin.at](https://margin.at) annotations displayed alongside glean annotations
- A trending page showing what's popular across all users
- Feed and people recommendations based on reading overlap
- Daily digest with an AI-generated summary of your unread articles
- OPML import and export
- Sign in with Bluesky / Atmosphere account — no new account needed

## How recommendations work

Glean looks at what you and other users subscribe to, read, and like to suggest feeds, articles, and people you might enjoy.

**Feed suggestions** come from readers who share your subscriptions. If a lot of people who follow the same blogs as you also follow a blog you haven't seen, that blog shows up as a recommendation. The system also considers which articles you've liked, whether you follow the person on Bluesky, and how popular the feed is overall.

**People suggestions** are split into two groups: "Your network" shows people you already follow on Bluesky who share your reading habits, and "Discover new readers" surfaces readers you don't follow but who have overlapping subscriptions and likes.

**Dismissals** keep things tidy. If you dismiss a recommendation, it won't come back. If a suggestion sits ignored for more than 5 days, it's automatically removed so newer recommendations can take its place.

**Cold start.** If you're new and have fewer than five subscriptions, Glean shows feeds from people you follow on Bluesky alongside popular feeds from the community, so there's something to explore right away.

The system improves over time: as you subscribe to feeds and like articles, Glean learns which signals matter most to you and adjusts accordingly.

## Self-hosting

### Docker

```bash
docker run -p 3000:3000 -e GLEAN_SESSION_KEY=changeme -v glean-data:/data atcr.io/julien.rbrt.fr/glean:latest
```

### From source

The frontend is a SvelteKit app in `web/`; the Go binary serves a JSON API.
SvelteKit runs the SSR server (port 3000) and proxies `/api` to the Go API
(port 8080).

```bash
git clone https://github.com/anomalyco/glean.git
cd glean
make web-install   # install frontend deps (bun)
make build         # builds the frontend and the Go binary
```

Run both in dev:

```bash
make dev-api   # Go API on :8080
make dev-web   # SvelteKit dev server on :3000 (proxies /api to :8080)
```

Then open `http://localhost:3000`.

## Configuration

| Variable                     | Default                            | What it does                                                                                                                                    |
| ---------------------------- | ---------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------- |
| `GLEAN_SESSION_KEY`          | _(required)_                       | Secret key for signing session cookies (any random string)                                                                                      |
| `GLEAN_ADDR`                 | `:8080`                            | Listen address                                                                                                                                  |
| `GLEAN_DB`                   | `glean.db`                         | SQLite base path (`_users`, `_articles`, `_recs` suffixes)                                                                                      |
| `GLEAN_JETSTREAM`            | `wss://jetstream1.eurosky.network` | Jetstream WebSocket URL                                                                                                                         |
| `GLEAN_SYNC_INTERVAL`        | `8h`                               | PDS sync interval (Go duration: `24h`, `12h`, etc.)                                                                                             |
| `GLEAN_CLUSTER_INTERVAL`     | `1h`                               | Cluster recomputation interval (Go duration)                                                                                                    |
| `GLEAN_FETCH_INTERVAL`       | `15m`                              | Feed fetch scheduler tick interval (Go duration)                                                                                                |
| `GLEAN_COLLECTION_DIR_URL`   | _(empty)_                          | Collection directory URL for startup backfill                                                                                                   |
| `GLEAN_BACKFILL_CONCURRENCY` | `5`                                | Max concurrent backfill workers                                                                                                                 |
| `GLEAN_PLC_URL`              | `https://plc.eurosky.network`      | PLC directory URL for DID resolution                                                                                                            |
| `GLEAN_OAUTH_CLIENT_ID`      | _(empty)_                          | OAuth client-metadata URL; enables production OAuth (leave empty for localhost dev). Must resolve to this server's `/api/oauth/client-metadata` |
| `GLEAN_FRONTEND_URL`         | _(required)_                       | Public origin of the SvelteKit frontend (e.g. `https://glean.at`); `make dev` defaults this to `http://localhost:3000`                          |
| `GLEAN_EMBED_BASE_URL`       | _(empty)_                          | Embeddings API base URL (recommended, see below)                                                                                                |
| `GLEAN_EMBED_API_KEY`        | _(empty)_                          | API key for the embeddings endpoint                                                                                                             |
| `GLEAN_EMBED_MODEL`          | `text-embedding-3-small`           | Embedding model name                                                                                                                            |
| `GLEAN_EMBED_DIMENSION`      | `1536`                             | Embedding vector dimension                                                                                                                      |
| `GLEAN_LLM_BASE_URL`         | _(empty)_                          | LLM API base URL for language detection and digest summaries (see below)                                                                        |
| `GLEAN_LLM_API_KEY`          | _(empty)_                          | API key for the LLM endpoint                                                                                                                    |
| `GLEAN_LLM_MODEL`            | `gpt-4o-mini`                      | LLM model name                                                                                                                                  |
| `GLEAN_PPROF_ADDR`           | _(empty)_                          | Enable pprof profiling server (e.g. `:6060`, off by default)                                                                                    |

For production:

```bash
export GLEAN_OAUTH_CLIENT_ID=https://yourdomain.com/api/oauth/client-metadata
```

The OAuth callback is always served at `$GLEAN_FRONTEND_URL/api/auth/callback` (the frontend proxies it to the backend), so there is no separate redirect-URL setting.

The SvelteKit server reads `GLEAN_API_URL` (default `http://localhost:8080`) to
find the Go API and `PORT` for its listen address. The public origin lives in
`GLEAN_FRONTEND_URL`; SvelteKit's adapter-node `ORIGIN` is not needed here.

## Documentation

- [Technical specification](docs/specs.md) — architecture, database schema, AT Protocol lexicons, API endpoints, recommendations
- [Design system](docs/design.md)

## Stack

Go, SQLite, SvelteKit (SSR), TailwindCSS, AT Protocol OAuth.

## License

[MIT](license)
