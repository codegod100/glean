# pulseboard — a small RSS reader

One binary, one SQLite database, no accounts.

```
nim c --threads:on -d:ssl pulseboard.nim
./pulseboard
```

Then open <http://127.0.0.1:8080>. The database lives under
`~/.local/share/pulseboard/`; override with `PULSEBOARD_PORT` and `PULSEBOARD_DB`.

## The interface

A two-pane reader: feeds on the left, articles on the right, the article in
place. Add a feed, refresh, search, read. Opening an article marks it read;
"Fetch full text" runs the scraper over the original page, for feeds that
only ship an excerpt.

One HTML file, no build step, no dependencies, baked into the binary with
`staticRead` so there is still only one file to copy.

## API

The interface is built on this, and it is usable on its own.

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

## A note on rendering feeds

Article bodies are markup from someone else's server, and the interface puts
them in a browser. Every body the API serves -- scraped *and* straight from
the feed -- goes through the same whitelist the scraper uses: `script`,
`style` and `iframe` never survive, attributes are allowed rather than
denied, and a link that does not point at http(s) is flattened to its text.

Titles and summaries are set with `textContent` in the page, for the same
reason. `innerHTML` is used in exactly one place: the sanitised body.

## What it is built on

Six modules under `reader/`:

| | |
|---|---|
| `feedfetcher` | HTTP with a retry policy that retries only 429 and 5xx |
| `feedparser`  | RSS 2.0, RDF, Atom and JSON Feed |
| `scraper`     | readability-style extraction, output sanitised |
| `sqlite`      | a thin wrapper: prepare, bind, step, transactions |
| `pulseboarddb`     | two attached databases and the schema |
| `feedstore` / `articlestore` | the queries |

Each carries its own checks:

```
nimble test      # no network needed
nimble testnet   # parses and scrapes real sites
```

Storage keeps a single hard-coded user. The schema began life in a
multi-user server, and satisfying that one column was cheaper than
rewriting every query around it.

## History

This started as a port of a much larger reader — one with accounts, ATProto
sync, social recommendations and a clustering engine. That half has been
deleted. What remains is the part that reads feeds.
