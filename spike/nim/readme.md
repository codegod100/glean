# Nim port spike

Two questions had to be answered before committing to a Nim rewrite of the Go
backend. Neither is about volume -- 14k lines is a lot of typing, not a lot of
risk. The risk is that Go has libraries Nim does not, and two of them sit under
everything.

Run both:

```
cd spike/nim
nimble test      # everything that needs no network
nimble testnet   # the probes that talk to real servers
nimble testall   # both
nimble schema    # regenerate atproto/schema.nim from the Go source
```

Reusable code lives in `atproto/`; the `*_probe.nim` files are the checks.
They are split by whether they touch the network: a failure against
`bsky.social` can mean that server is having a bad day rather than that the
port is broken, and a suite that cannot tell those apart stops being
believed. Individual probes still run on their own:

```
nim c -r -d:ssl <name>_probe.nim
```

## 1. sqlite-vec + FTS5 — works

Glean needs FTS5 for article search and sqlite-vec for embedding similarity.
Both are C-level concerns, so the question was never "does Nim have a SQLite
library" but whether the same extension links and registers the same way.

`sqlite_probe.nim` compiles the same `sqlite-vec.c` the Go bindings vendor,
calls `sqlite3_vec_init` directly, and exercises `vec_version()`, an FTS5
`MATCH`, a `vec0` virtual table, float32 blob binding, and a KNN query.
All pass; KNN returns the expected nearest neighbour.

**One trap, and it is a nasty one.** sqlite-vec must be compiled with
`-DSQLITE_CORE`. Without it the file builds as a *loadable extension* whose
entry point takes its SQLite API pointer from the caller — pass `nil` there and
it segfaults inside `sqlite3_vec_init` rather than returning an error. Nim's
`{.compile: (file, flags).}` tuple form did not reliably apply the define, so
it lives in `nim.cfg` where it cannot be forgotten.

## 2. ES256 / DPoP — works

This was the real risk. Glean gets OAuth from `bluesky-social/indigo`; Nim has
no ATProto library, so a port has to build the DPoP layer itself, and
everything downstream of sign-in depends on it.

`dpop_probe.nim` generates a P-256 key, builds the public JWK, computes an
RFC 7638 thumbprint, and signs a DPoP proof JWT (RFC 9449).

It uses **bearssl** rather than OpenSSL for one specific reason: JOSE requires
the signature as raw `R||S`, 64 bytes, and most crypto libraries hand you DER.
`br_ecdsa_sign_raw` emits `R||S` directly, so there is no DER-unwrapping step
to get subtly wrong.

Self-verification would only prove internal consistency, so the token goes to
`verify/verify.go`, which re-implements the verifier from Go's standard
library — the same stack the current server runs on. It checks the header
claims, that the signature is 64 raw bytes and not DER-wrapped, that the
signature verifies against the JWK, that a tampered payload is *rejected*, and
that the thumbprint matches when recomputed independently.

19/19 checks pass, and the whole cycle passes across 30 independently generated
keypairs — worth confirming, because ECDSA values with leading zeros are a
classic source of intermittent length bugs in `R||S` encoding.

## 3. The OAuth flow — works against a live PDS

`atproto/identity.nim` and `atproto/oauth.nim` implement resolution and the
client flow, ported from the shape of indigo's `atproto/auth/oauth`.
`oauth_probe.nim` drives it end to end against real servers:

    handle -> DID -> PDS -> auth server -> metadata -> PAR -> authorize URL

Against `bsky.social` all 15 checks pass, including a **real PAR that the
production auth server accepts** and returns a `request_uri` for. That single
result exercises the DPoP signature, the nonce retry and PKCE at once — a
server-side validation no amount of local testing substitutes for.

Two things worth recording:

**The nonce retry is load-bearing, and the probe proves it.** A PDS rejects the
first DPoP-signed request of a session with `400 use_dpop_nonce` and supplies a
`DPoP-Nonce` header to retry with. Since `sendAuthRequest` retries
transparently, the probe separately fires an unnonced request and asserts the
rejection — otherwise a server that never demanded a nonce would look identical
to a working retry.

**Order of validation caught me out.** That check first sent a stub body and got
`invalid_request`, not `use_dpop_nonce`: bsky.social validates request
parameters *before* the nonce. The body has to be fully valid to observe the
nonce requirement at all.

The flow stops at the authorization URL, which is correct — the next step is a
human granting consent in a browser. `exchangeCode` and `refresh` are
implemented against that callback but are consequently untested.

## 4. XRPC signing — proof accepted by a real PDS

`atproto/xrpc.nim` makes authenticated calls with a DPoP-bound access token:
`Authorization: DPoP <token>` (not Bearer), a per-request proof, and the retry
logic for both failure modes.

Resource-server proofs are **not** the same as auth-server proofs, even when
one host fills both roles. A PDS request carries `ath` (base64url SHA-256 of
the access token, which is what binds the proof to that one credential) and
`iss`; the PAR and token requests that mint the token carry neither. The nonce
handshake also differs: a PDS answers **401** with `WWW-Authenticate: DPoP
error="use_dpop_nonce"`, where the auth server used **400** and a JSON body.
A session therefore tracks two independent nonces.

`xrpc_probe.nim` cannot get a consented token, so it signs a request with a
deliberately invalid one and reads what a real PDS says. Against
`bsky.social`'s PDS:

    WWW-Authenticate: DPoP algs="...", error="invalid_token",
                      error_description="Malformed token"

That is the informative result. `invalid_token` rather than
`invalid_dpop_proof` means the PDS parsed and accepted the proof — signature,
claims, `ath`, `htu`, `htm` — and rejected only the credential. The challenge
scheme coming back as `DPoP` confirms the Authorization scheme too, and the
response carried a host nonce.

`ath` is pinned against a value computed with `openssl dgst` rather than
checked for self-consistency, since a self-consistent hash would pass any
internal assertion and still be refused by every server.

Two bugs the probe caught, both of which would have failed every authenticated
call in ways that are hard to read from the outside:

- `htm` was built as `($httpMethod)[4..^1]`, assuming the enum stringified to
  `HttpGet`. It stringifies to `GET`, so this was a range error, not a
  mis-signed claim — loud, but only once something ran.
- `hostOf` returned an empty host for a scheme-less input like
  `public.api.bsky.app`, because `parseUri` files that under `path`. URLs came
  out as `https:///xrpc/...`.

A third mistake was in the probe rather than the code: the first version
pointed at `com.atproto.repo.listRecords`, which is **public**. The PDS served
it with a 200 while ignoring the bogus credential entirely — a passing test
that tested nothing. `com.atproto.server.getSession` has to evaluate the
credential.

## 5. Jetstream — works, verified against the live firehose

`atproto/jetstream.nim` consumes the ATProto firehose as JSON over a
websocket, using `websock` (already vendored via chronos). Jetstream is public
and unauthenticated, so unlike the OAuth and XRPC probes this one checks the
real thing end to end: 17/17 against the live network.

Two capabilities that matter to Glean are confirmed rather than assumed:

**Server-side filtering.** The probe subscribes to one collection and asserts
that nothing else arrives. This is worth checking explicitly because the
failure mode is invisible: an unfiltered subscription is the entire network,
so a consumer that loses its query parameters still receives a healthy-looking
torrent of events that happen to be the wrong ones.

**Cursor replay.** Rewinding to an earlier `time_us` must actually rewind, not
quietly resume live — that is how a consumer catches up after downtime. The
probe replays from a cursor it saw and asserts the events *overlap* what it
already received (25 of 25). Accepting the parameter proves nothing on its
own.

Two traps:

- **websock's `Uri` overload silently drops the query string.** It forwards
  only `uri.path`, so every Jetstream filter would vanish and the consumer
  would subscribe to the whole firehose while looking fine. `connect` uses the
  host/path form and assembles path-plus-query itself.
- Jetstream wants `wantedCollections` **repeated per value**, not one
  comma-joined string. A joined value matches no collection at all.

`websock` pulls in chronicles, which does not compile unless a logging stream
is configured and resolves it in the *importing* module's scope — hence the
`import chronicles` in `jetstream.nim` that nothing appears to use, and the
`chronicles_enabled=off` in `nim.cfg`.

## 6. Reconnect and backoff — works

`atproto/jetstream_run.nim` keeps a subscription alive. Three concerns, all
about not losing events rather than about uptime:

- **Resume from a rewound cursor.** Cursors are persisted in batches, so the
  last one written always trails the last one seen. Reconnecting rewinds past
  that gap and accepts re-delivery: duplicates are cheap when handling is
  idempotent, a hole in the record is not.
- **Rotate connections on a timer.** Server-side filters are fixed for the
  life of a connection, so a subscription that never reconnects never learns
  about newly-known DIDs. Glean's Go consumer rotates every 15 minutes for
  this reason, not for robustness.
- **Back off with jitter.** The Go consumer waits a flat 5s, which is fine
  against one healthy server and unkind to one that is down.

A healthy server proves nothing here, so `reconnect_probe.nim` runs a local
websocket server that hangs up after three frames. Over two seconds the
supervisor made 12 connections and received 36 events across them.

The probe checks that the cursor **reaches the server**, not merely that it
advances locally: a consumer that tracks a cursor it never sends looks
identical from the inside and silently loses the gap on every reconnect. 11
of 12 connections carried one, and the first correctly did not — it starts
live.

One bug worth recording. `delayFor` short-circuited `attempt <= 0` straight
to the initial delay, skipping jitter on the first retry. That is precisely
the moment jitter exists for: one server restart drops every consumer at
once, and un-jittered first retries bring them all back simultaneously.

## 7. Session persistence — works

`atproto/sqlite.nim` is a thin wrapper over the library the storage spike
already proved (sqlite-vec linked in, WAL on). `atproto/store.nim` keeps the
three things a restart must not lose, in the same tables the Go backend uses:
`oauth_auth_requests`, `oauth_sessions`, `jetstream_cursor`. Rows hold JSON,
as on the Go side, so adding a session field does not mean a migration.

`atproto/cursor_store.nim` bridges the store and the supervisor, batching
writes: the cursor advances on every event, often hundreds a second, and a
synchronous write per event would make the database the bottleneck for a
firehose. The stored cursor therefore trails the live one — which is exactly
the gap the supervisor's rewind exists to cover.

The check that earns its keep is the DPoP key. A missing row is loud; a key
that round-trips *almost* correctly is not — every request after a restart
would fail authentication with nothing pointing at storage. So the reloaded
key is checked by **signing with it and verifying against the original public
point**, with a second key as a negative control. Comparing fields, or
re-signing and comparing bytes, would prove neither thing: ECDSA is
randomised.

Two other properties worth pinning:

- Taking a pending auth request **consumes** it. An authorization code is
  single-use, and leaving the row behind invites a replayed callback.
- Cursors never move backwards. A late event that rewound the stored cursor
  would rewind the subscription on the next reconnect.

The public half of the key is stored rather than re-derived on load. Deriving
it needs a curve operation, and a stored key that cannot be read back without
one is a restart that silently drops every session.

**These rows are key material.** A session holds the DPoP private key and the
refresh token; together they are the account. The database file wants the
protection of a password store, not of a cache.

## 8. The database layer — foundation done

`atproto/gleandb.nim` opens Glean's storage the way the Go backend does:
`<base>_users` as `main`, with `_articles` and `_recs` **attached**, so a
query can join across them while each file still checkpoints and vacuums
alone — and so the derived recommendation tables can be dropped and rebuilt
without touching anything a user wrote. The suffixes match, so a database
written by either implementation is readable by the other.

Pragmas are repeated per schema rather than set once, which is the detail
worth knowing: `PRAGMA` applies to one database at a time, so setting WAL on
`main` leaves `articles` journalling the slow way. The probe asserts WAL on
all three rather than trusting the call.

`exp()` and `log()` are registered as SQL functions, as the Go connection
does, and they are load-bearing: the clustering SQL calls `EXP` and `LOG` in
seven places for time decay and popularity normalisation. SQLite only ships
maths functions when built with `SQLITE_ENABLE_MATH_FUNCTIONS`, so without
these registrations every recommendation query fails at runtime rather than
at startup.

`log()` returns NULL for zero and negatives where Go's `math.Log` returns
`-inf` and NaN. The call sites all pass `1 + count`, so the argument is never
below 1 and the two agree in practice -- but it is a divergence, noted here
rather than left to be found.

**The schema is extracted, not retyped.** `tools/extract_schema.py` pulls all
44 DDL statements out of `internal/db/db.go` into `atproto/schema.nim`.
Hand-transcribing them would drift, and a column differing by a default or a
CHECK constraint surfaces months later as a violation nobody can place.

The probe leans on things that fail quietly: an ATTACH that did not happen, a
pragma that did not take, FTS5 triggers that never fire. That last one is the
sharpest — the index is maintained by triggers, so if they were missed,
search returns nothing while every other query looks healthy. Insert, update
and delete are each checked against the index.

### Scope

This is the foundation, not the whole layer. Glean's `internal/db` is ~2,800
lines of non-test code; what is ported here is the connection, the schema and
the primitives, plus a cross-database join, FTS5 and vec0 proven end to end.

The remaining query surface — `article.go`, `feed.go`, `social.go`,
`retention.go` and friends — is mechanical against this base, and large. It
is the first part of the port where the work is volume rather than risk.

## 9. Article and feed queries

`atproto/feedstore.nim` and `atproto/articlestore.nim` port the core of
`internal/db/feed.go` and `internal/db/article.go`, with `sqlite.nim` grown to
carry them: typed nullable row access, transactions, and prepared statements
that can be stepped many times.

Read state is the shape everything else follows. It is per-user and lives in
its own table, so a listing is always a LEFT JOIN and "unread" means *no row*
— hence `COALESCE(r.is_read, 0)` throughout rather than a column on the
article.

Two behaviours carried over deliberately, both about not resurfacing things a
reader has dealt with:

- Articles already older than the retention window are **not ingested**.
  Writing them only to purge them churns the database.
- After ingest, read state is **restored from history**, because a purged
  article that reappears gets a new surrogate id and would otherwise come
  back unread.

The probe targets the ways these can be quietly wrong: a mark-all-read whose
scope clears the account instead of one feed, a subscriber count that drifts,
a listing that leaks another user's feeds, a NULL date sorting to the top.

### Four bugs the probe caught

All four were wrong assumptions about the schema or about SQLite, and none
would have raised at compile time:

- `likes` keys on **`author_did`**, not `user_did` — a like is an ATProto
  record with an author, not a user-scoped row.
- FTS5 requires the **bare table name** on the left of `MATCH`. Neither an
  alias nor a schema-qualified name is accepted, so the match moved into a
  subquery, which also carries `rank` out for ordering.
- The shared feed column list used bare names, which work until a query joins
  `subscriptions` — which also has `feed_url` — and then fail as ambiguous.
  Now qualified with the table alias.
- The probe's own first draft assumed `users` had a `handle` column. It does
  not; handles are resolved from atproto rather than stored.

### Scope

Roughly the core of the two files by usage: feeds, subscriptions, listing,
filtering, read state, counts, search, retention, dead feeds. Not ported:
`BatchReconcileSubscriptions`, OPML-adjacent helpers,
`GetNextArticleID`'s navigation logic, and the several list variants the
recommendation engine uses. They are mechanical against these patterns.

## 10. Likes, annotations, follows and trending

`atproto/socialstore.nim` ports `internal/db/social.go`, completing the
database layer.

Likes and annotations are ATProto records first and rows second. They are
keyed by their `at://` URI because that is the identity the PDS owns, and
each can arrive twice -- once from the firehose and once from a PDS sync --
so every write is idempotent by construction rather than by checking first.
An edit keeps the URI and changes the content, which is why annotations
upsert rather than insert.

Two details worth keeping:

- The language filter always keeps `language = ''`. That means *not yet
  classified*, not *no language*; excluding it would make new articles vanish
  for anyone with a filter set and reappear minutes later once classified.
- Future-dated articles sort last in trending. A feed publishing with a
  scheduled timestamp would otherwise pin itself to the top indefinitely.

The probe weights toward the destructive path. `deleteOrphanedLikes` and its
annotation twin delete local rows the PDS no longer has, so the checks assert
both halves: the dropped rows go, and *other* users' rows do not. The empty
active set is called out in the code, because "you unliked everything" and
"the fetch failed" are indistinguishable from inside the store — the caller
has to not get there.

## 11. Feed parsing

`atproto/feedparser.nim` ports `internal/feed/parser.go`: RSS 2.0, RDF
(RSS 1.0), Atom and JSON Feed.

The interesting part of a feed parser is not the happy path, it is the
fallbacks, because feeds in the wild disagree with the spec constantly. A
missing `guid` falls back to the link — it is what dedupes an article across
fetches, so an item without one would be re-inserted forever. `rel="self"`
loses to `rel="alternate"`, or a feed whose first link is itself would point
every article back at the feed. An unparseable date leaves the article
undated rather than dropping it. Atom's `updated` stands in for a missing
`published`, since Atom requires only the former.

Namespaces are matched by local name rather than resolved. Go's decoder maps
`content:encoded` to its namespace URI; Nim's keeps the literal prefix, and
feeds are inconsistent enough about prefixes that suffix matching is both
simpler and more forgiving.

### Two bugs, one of them quiet and severe

**`xmltree.innerText` silently skips CDATA.** RSS ships article bodies inside
CDATA *precisely because* they contain HTML, so relying on `innerText` loses
the content of most real feeds while every surrounding field parses
perfectly — a parser that looks like it works and returns empty articles.
The parser now walks the tree itself, including `xnCData`.

**Nim's `zzz` requires `-05:00`; `ZZZ` accepts `-0500`.** Feeds use both, so
both formats are in the list. Without it, every RSS feed using a numeric
offset — which is most of them — parsed as undated.

Neither would have failed a compile, and neither is visible without a
fixture that contains the case.

### Beyond fixtures

Fixtures only contain what their author thought of, so the probe also fetches
four real feeds across two formats (Rust Blog, nim-lang, LWN, Hacker News)
and asserts every item has a guid and a title. All four parse, 65 items
between them, all dated and all carrying text.

## 12. Fetching feeds

`atproto/feedfetcher.nim` ports `internal/feed/fetcher.go`. Retries exist for
servers that are briefly unwell -- 429 and 5xx -- and for nothing else. A 404
will still be a 404 in a second, and bytes that do not parse will not parse
differently on a second read.

The retry policy is the substance, so the probe drives it with a scripted
transport: 503 twice then success, a 404 that must not be retried, a 429
naming its own delay. None of those are reproducible against a real host on
demand.

### A bug in the Go fetcher

The Go retry policy does not do what it reads as doing. `executeRequest`
returns its `*http.Response` **only on success**, so in `Fetch` the guard

```go
if resp != nil && !httpclient.IsRetryable(resp.StatusCode) {
```

can never be true when `err != nil` -- the response is always nil on the
error path. Three things follow:

- Every failure retries the full four attempts regardless of status, so a
  permanently-404 feed costs four requests and ~7s of backoff on every
  refresh cycle.
- `retryBackoff`'s `Retry-After` handling is unreachable for the same reason:
  `lastResp` is only non-nil once the function has already returned, so a
  rate-limited server's own delay is never honoured.
- The HTTP status is never checked at all, so an error page is handed to the
  parser and fails there instead of failing as an HTTP error.

This port checks the status, honours `Retry-After` (capped, so a server
asking for an hour cannot hold a worker for one), and retries only what is
worth retrying. That is a behaviour change rather than a translation, which
is why it is written down both here and at the top of the module.

Worth fixing on the Go side too; it is a handful of lines.

## 13. The scraper

`atproto/scraper.nim` ports `internal/scraper/scraper.go`. Feeds routinely
ship a first paragraph and a "read more" link, so the reader view needs the
page itself: find the node most likely to be the article, strip the furniture
around it, and re-render a whitelisted subset of HTML. Output stays HTML
rather than text, because paragraphs, headings, lists and code blocks are
most of what makes a long article readable.

### Sanitisation is the real test

This renders HTML fetched from an arbitrary site into a reader's client, so
the probe weights toward what must *not* survive: `script` and `style`
content, `iframe`, event handlers, unknown data attributes, `javascript:`
URLs. Attributes are whitelisted rather than blacklisted, so an attribute
nobody anticipated is dropped by default instead of needing to be added to a
list. A page that renders correctly proves nothing about what was stripped,
so each of those is asserted separately.

Links carry `rel="noopener noreferrer"`, and a link whose target is not
http(s) is flattened to its text -- the words survive without the anchor.

Two smaller details worth keeping:

- A lazy-loaded image resolves through `src` → `data-src` → `data-lazy-src`
  to the first *reachable* URL. Rendering the placeholder gives a broken
  image where the picture should be.
- Removal walks children back to front. Deleting by index shifts every
  sibling after it, so a forward walk skips one after each removal -- the
  probe has four consecutive `<nav>` elements specifically to catch that.

The extractor refuses rather than returning something thin. A caller handed a
nav menu cannot tell it from a short article; an error lets it fall back to
the feed's own summary.

The live check takes its URL from the Rust blog's feed rather than
hard-coding one, after a hard-coded guess 404'd -- a probe that rots when
someone reorganises their permalinks is worse than no probe.

## 14. Similarity and scoring

`atproto/cluster.nim` ports the similarity computations from
`internal/cluster/jaccard.go` and the pure helpers from `scoring.go`.

Both similarities are precomputed on a schedule and staged into a temp table
before being swapped in, so the expensive pass holds no lock on the live
table and readers keep seeing the previous generation rather than an empty
one.

Feed similarity is time-decayed, `exp(-0.023 * days)` -- a half-life of about
a month. Without it a long-lived account's earliest subscriptions dominate
its recommendations forever. User similarity over *subscriptions* is
deliberately undecayed, because a subscription is a standing choice rather
than an event; shared *likes* are events, so those decay on both sides.

Recommendations have no obviously correct answer, so the probe checks
properties rather than values: that decay decays, that each pair is stored
once, that recomputing replaces rather than accumulates, that normalisation
of an all-equal set does not produce NaN, and that `samplePeople` reserves
half its slots for people the reader does *not* follow -- ranking purely by
similarity returns the network they already have, which is not discovery.

### A bug inherited from the Go implementation

`common_likes` is computed as `CAST(SUM(decay * decay) AS INTEGER)`. Each
decay factor is at most 1, so their product is always just under it: a pair
sharing one recent like sums to 0.9999 and **truncates to 0**. Two shared
likes store 1. The number is shown to readers as the reason for a
recommendation -- "N shared likes" -- so a real overlap reads as none.

This port uses `ROUND` instead. Worth fixing on the Go side too.

The probe caught it only because the assertion looked *vacuous*: both the
recent and the stale pair returned 0, so "recent >= old" passed while
proving nothing. A test that passes for the wrong reason is worse than one
that fails.

## What this does and does not prove

Proven: the cryptography, the storage layer, and the OAuth flow up to user
consent all work from Nim, in the formats ATProto and real servers accept.

Not proven, and still ahead:

- **Token exchange and refresh** — written, but needs a browser consent to
  exercise. This is the one place a real end-to-end test still has to happen,
  and it also covers the `invalid_token` refresh-and-retry branch of
  `xrpc.request`, which is currently unexercised.

### Not needed: DAG-CBOR, CAR and the MST

Earlier notes listed these as remaining work. That was wrong, and worth
correcting because they are by far the largest format risk left in ATProto.

Glean never parses them. `internal/atproto/sync.go` opens by saying so: it
reconciles with `com.atproto.repo.listRecords` rather than
`com.atproto.sync.getRepo`, because it only syncs known users, Jetstream
covers real-time events, and every reconcile is idempotent. Reads go through
`listRecords`/`getRecord` and writes through
`createRecord`/`putRecord`/`deleteRecord` — all JSON.

The imports confirm it. Glean uses four indigo packages — `atclient`,
`auth/oauth`, `identity`, `syntax` — and none of `repo`, `mst`, `cbor` or
`car`.

A Nim port inherits that choice for free. Implementing DAG-CBOR, CIDs, CAR
framing and Merkle Search Tree traversal would be real work in service of a
code path this application does not have.

The honest read: none of the three things that could have sunk the port did.
What is left is protocol plumbing against well-specified formats — large, but
ordinary.

## Caveats in this code

- `identity.nim` shells out to `dig` for the DNS leg of handle resolution,
  because Nim's stdlib has no resolver. A missing `dig` degrades to the
  HTTP `.well-known` fallback rather than failing, but a real port should bind
  a resolver instead of depending on a binary.
- Only public and localhost clients are supported. Confidential clients
  (`private_key_jwt` client assertions) are not implemented; Glean uses a
  public client today.
