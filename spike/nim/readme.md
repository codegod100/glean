# Nim port spike

Two questions had to be answered before committing to a Nim rewrite of the Go
backend. Neither is about volume -- 14k lines is a lot of typing, not a lot of
risk. The risk is that Go has libraries Nim does not, and two of them sit under
everything.

Run both:

```
cd spike/nim
nim c -r sqlite_probe.nim          # storage
nim c -r dpop_probe.nim && ./dpop_probe | (cd verify && go run .)   # auth
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

## What this does and does not prove

Proven: the cryptography and the storage layer are available to Nim, in the
exact formats ATProto and Glean need.

Not proven, and still ahead:

- The **OAuth protocol flow** — PAR, the authorization request, the DPoP nonce
  retry dance (a PDS rejects the first request and returns a nonce to use).
  Mechanical, but a lot of it.
- **DID/handle resolution** and the PLC directory.
- **XRPC** and the Jetstream websocket consumer (`indigo` covers both today).
- **CBOR/CAR** parsing for repository records.

The honest read: nothing here is blocking, and the two things that could have
been blocking are not. The remaining work is large but ordinary.
