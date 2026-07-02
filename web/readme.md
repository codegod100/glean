# Glean web (SvelteKit)

SSR frontend for Glean. Talks to the Go JSON API (see `../internal/server`).

## Develop

```bash
bun install
# Start the Go API on :8080 first (make dev-api), then:
GLEAN_API_URL=http://localhost:8080 bun run dev
```

Open http://localhost:3000.

## How it talks to the API

- `src/hooks.server.ts` proxies every `/api/*` request to the Go server
  (`GLEAN_API_URL`), forwarding cookies and headers. The same hook also loads
  the current user for the layout.
- `src/lib/api.ts` wraps the endpoints. Server load functions use
  `endpointsFor(event.fetch)` so SvelteKit's per-request fetch resolves relative
  `/api` URLs during SSR; browser code uses the default `endpoints`.

## Build

```bash
bun run build   # adapter-node output in ./build
```
