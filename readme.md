# pulseboard

An RSS reader. Two pieces:

| | |
|---|---|
| [`spike/nim`](spike/nim) | the reader — a Nim binary serving a small JSON API |
| [`app`](app) | a Flutter client for it |

Run the reader, point the client at it:

```
cd spike/nim && nim c --threads:on -d:ssl pulseboard.nim && ./pulseboard
```

It listens on `127.0.0.1:8080`, keeps a SQLite database under
`~/.local/share/pulseboard/`, and serves a plain HTML interface at the root — so
it is usable on its own, without the Flutter client.

```
cd app && flutter run --dart-define=PULSEBOARD_BASE_URL=http://127.0.0.1:8080
```

## Why it is split

A browser cannot fetch most feeds directly: almost none send the CORS
headers that would allow it. Something outside the browser has to do the
fetching, which is what the reader is. It also does the work that wants to
happen once rather than per client — parsing four feed formats, full-text
search, and pulling readable article text out of pages that only syndicate
an excerpt.

## Security

The reader requires AT Protocol OAuth. It uses the official ATProto client for
discovery, PKCE, PAR, and DPoP, then immediately revokes the OAuth credential:
Pulseboard keeps only the verified DID in a short-lived, opaque web session.
Subscriptions and read state are scoped to that DID.

Article bodies are markup from other people's servers, so every body the API
serves — scraped and straight-from-the-feed alike — goes through a
whitelist: `script`, `style` and `iframe` never survive, attributes are
allowed rather than denied, and links that are not http(s) are flattened to
text.

## History

This began as a port of a social RSS reader built on the AT Protocol — Go
backend, SvelteKit frontend, accounts, annotations, a recommendation engine.
That has been removed; the [git history](../../commits/main) has it if it is
ever wanted. What is left is the part that reads feeds.

Two bugs found in the original along the way were fixed there before it went:
a retry policy that retried every failure regardless of status, and a decayed
overlap count that truncated a real value to zero.
