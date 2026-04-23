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
- OPML import and export
- Sign in with Bluesky / Atmosphere account — no new account needed

## Self-hosting

### Docker

```bash
docker run -p 8080:8080 -v glean-data:/data atcr.io/julien.rbrt.fr/glean:latest
```

### From source

```bash
git clone https://github.com/anomalyco/glean.git
cd glean
make build
./glean
```

Then open `http://localhost:8080`.

## Configuration

| Variable                   | Default                    | What it does                                              |
| -------------------------- | -------------------------- | --------------------------------------------------------- |
| `GLEAN_ADDR`               | `:8080`                    | Listen address                                            |
| `GLEAN_DB`                 | `glean.db`                 | SQLite base path (`_users`, `_articles`, `_recs` suffixes) |
| `GLEAN_JETSTREAM`          | `wss://jetstream.glean.at` | Jetstream WebSocket URL                                   |
| `GLEAN_SYNC_INTERVAL`      | `1h`                       | PDS sync interval (Go duration: `30m`, `2h30m`, etc.)     |
| `GLEAN_CLUSTER_INTERVAL`   | `10m`                      | Cluster recomputation interval (Go duration)              |
| `GLEAN_FETCH_INTERVAL`     | `5m`                       | Feed fetch scheduler tick interval (Go duration)           |
| `GLEAN_COLLECTION_DIR_URL` | _(empty)_                  | Collection directory URL for startup backfill              |
| `GLEAN_BACKFILL_CONCURRENCY` | `5`                      | Max concurrent backfill workers                            |
| `GLEAN_PLC_URL`            | `https://didplc.glean.at`  | PLC directory URL for DID resolution                      |
| `GLEAN_OAUTH_CLIENT_ID`    | _(empty)_                  | OAuth client metadata URL (leave empty for localhost dev) |
| `GLEAN_OAUTH_REDIRECT_URL` | _(empty)_                  | OAuth redirect URL (leave empty for localhost dev)        |

For production:

```bash
export GLEAN_OAUTH_CLIENT_ID=https://yourdomain.com/oauth/client-metadata
export GLEAN_OAUTH_REDIRECT_URL=https://yourdomain.com/auth/callback
```

## Documentation

- [Technical specification](docs/specs.md) — architecture, database schema, AT Protocol lexicons, API endpoints, recommendations
- [Design system](docs/design.md)

## Stack

Go, SQLite, htmx, TailwindCSS, AT Protocol OAuth.

## License

[MIT](license)
