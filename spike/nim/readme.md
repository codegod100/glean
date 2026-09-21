# Nim port spike

Two questions had to be answered before committing to a Nim rewrite of the Go
backend. Neither is about volume -- 14k lines is a lot of typing, not a lot of
risk. The risk is that Go has libraries Nim does not, and two of them sit under
everything.

Run both:

```
cd spike/nim
nim c -r sqlite_probe.nim                                          # storage
nim c -r dpop_probe.nim && ./dpop_probe | (cd verify && go run .)  # crypto
nim c -r -d:ssl oauth_probe.nim [handle]                           # the flow
nim c -r -d:ssl xrpc_probe.nim                                     # signing
nim c -r -d:ssl jetstream_probe.nim                                # firehose
```

Reusable code lives in `atproto/`; the `*_probe.nim` files are the checks.

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

## What this does and does not prove

Proven: the cryptography, the storage layer, and the OAuth flow up to user
consent all work from Nim, in the formats ATProto and real servers accept.

Not proven, and still ahead:

- **Token exchange and refresh** — written, but needs a browser consent to
  exercise. This is the one place a real end-to-end test still has to happen,
  and it also covers the `invalid_token` refresh-and-retry branch of
  `xrpc.request`, which is currently unexercised.
- **Session persistence** across restarts.
- **CBOR/CAR** parsing for repository records — the last unexplored format,
  needed for full-repo sync rather than the live stream.
- **Reconnect and backoff** for a long-lived Jetstream subscription; the
  consumer reads a stream but does not yet survive the connection dropping.

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
