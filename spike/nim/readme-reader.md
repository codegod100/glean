# glean — a small RSS reader

One binary, one SQLite database, no accounts.

```
nim c -d:ssl glean.nim
./glean
```

Listens on `127.0.0.1:8080` and stores its database under
`~/.local/share/glean/`. Override with `GLEAN_PORT` and `GLEAN_DB`.

## API

```
GET    /feeds                       subscriptions, with unread counts
POST   /feeds          url=…        subscribe and fetch immediately
DELETE /feeds?url=…                 unsubscribe

GET    /articles                    unread, newest first
GET    /articles?status=all|read    …or everything, or just the read ones
GET    /articles?feed=…             …scoped to one feed
GET    /articles?q=…                full-text search (FTS5)
GET    /articles/:id                one article

POST   /read           id=…         mark read (undo=1 to reverse)
POST   /read-all       [feed=…]     mark a feed, or everything, read
POST   /refresh                     fetch every subscribed feed
POST   /fetch-content  id=…         scrape the full article text

GET    /unread                      unread count
```

Parameters are read from the query string or a form body, whichever the
caller used.

## What it does not do

No accounts, no sessions, no CSRF, no ATProto. It runs on your machine and
reads your feeds, and that assumption is what keeps it a few hundred lines
rather than a few thousand.

The one visible consequence: **do not expose it to a network.** It binds
loopback deliberately. Anything that can reach the port can read and change
everything, because there is nobody to distinguish from anybody else.

## What it is built on

The modules under `atproto/` — fetching with a sane retry policy, parsing
RSS/RDF/Atom/JSON Feed, SQLite with FTS5, and a readability-style extractor
that sanitises what it returns. Those came out of a larger port and carry
their own tests (`nimble test`); this file is a few hundred lines of routing
on top.

Storage keeps a single hard-coded user, because the schema came from a
multi-user server and it was cheaper to satisfy that than to rewrite it.
