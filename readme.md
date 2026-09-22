# glean

An RSS reader. Two pieces:

| | |
|---|---|
| [`spike/nim`](spike/nim) | the reader — a Nim binary serving a small JSON API |
| [`app`](app) | a Flutter client for it |

Run the reader, point the client at it:

```
cd spike/nim && nim c -d:ssl glean.nim && ./glean
```

It listens on `127.0.0.1:8080`, keeps a SQLite database under
`~/.local/share/glean/`, and serves a plain HTML interface at the root — so
it is usable on its own, without the Flutter client.

```
cd app && flutter run --dart-define=GLEAN_BASE_URL=http://127.0.0.1:8080
```

## Why it is split

A browser cannot fetch most feeds directly: almost none send the CORS
headers that would allow it. Something outside the browser has to do the
fetching, which is what the reader is. It also does the work that wants to
happen once rather than per client — parsing four feed formats, full-text
search, and pulling readable article text out of pages that only syndicate
an excerpt.

## Security

The reader has no accounts and no auth. It binds loopback deliberately:
anything that can reach the port can read and change everything, and
`/feeds` and `/fetch-content` will fetch any URL handed to them. **Do not
expose it to a network** without putting something in front of it.

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
