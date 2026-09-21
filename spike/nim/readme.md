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

## What this does and does not prove

Proven: the cryptography, the storage layer, and the OAuth flow up to user
consent all work from Nim, in the formats ATProto and real servers accept.

Not proven, and still ahead:

- **Token exchange and refresh** — written, but needs a browser consent to
  exercise. This is the one place a real end-to-end test still has to happen.
- **Session persistence** and the token-refresh lifecycle.
- **XRPC** request signing with the DPoP-bound access token.
- The **Jetstream** websocket consumer.
- **CBOR/CAR** parsing for repository records.

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
