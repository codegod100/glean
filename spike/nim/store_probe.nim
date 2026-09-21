## Checks that a session survives a restart intact.
##
##   nim c -r -d:ssl store_probe.nim
##
## The interesting failure is not "the row is missing" -- that is loud. It is
## a DPoP key that round-trips *almost* correctly: every request after a
## restart then fails authentication with no clue that storage is at fault.
## So the key is checked by signing with the reloaded copy and verifying
## against the original public point, not by comparing fields.

import std/[json, options, os, strformat, strutils]
import pkg/chronos
import bearssl/rand
import atproto/[cursor_store, dpop, jetstream_run, oauth, sqlite, store]

var failures = 0

proc report(name: string, ok: bool, detail = "") =
  if not ok: inc failures
  let label = if ok: "PASS" else: "FAIL"
  if detail.len > 0: echo &"  {label}  {name}  -- {detail}"
  else: echo &"  {label}  {name}"

proc main() =
  echo "Session persistence probe"
  echo ""

  let rngRef = HmacDrbgContext.new()
  doAssert rngRef != nil
  var rng = rngRef[]

  let path = getTempDir() / "glean_store_probe.db"
  removeFile(path)
  removeFile(path & "-wal")
  removeFile(path & "-shm")
  defer:
    removeFile(path)
    removeFile(path & "-wal")
    removeFile(path & "-shm")

  let key = generateKey(rng)
  let originalThumb = key.thumbprint

  # --- write, then close the database entirely ---------------------------
  block:
    var db = sqlite.open(path)
    migrate(db)
    report("schema applies", true, "sqlite " & libVersion())

    saveSession(db, SessionRecord(
      accountDid: "did:plc:probe",
      sessionId: "sess-1",
      pdsUrl: "https://pds.example",
      authServerUrl: "https://bsky.social",
      tokenEndpoint: "https://bsky.social/oauth/token",
      accessToken: "access-abc",
      refreshToken: "refresh-xyz",
      dpopKey: key,
      hostNonce: "host-n",
      authNonce: "auth-n",
      scopes: @["atproto", "transition:generic"],
      createdAt: 1_700_000_000,
    ))
    saveCursor(db, 1_700_000_000_000_000'i64)
    saveAuthRequest(db, AuthRequestData(
      state: "state-1",
      authServerUrl: "https://bsky.social",
      scopes: @["atproto"],
      pkceVerifier: "verifier-1",
      requestUri: "urn:ietf:params:oauth:request_uri:req-1",
      tokenEndpoint: "https://bsky.social/oauth/token",
      dpopNonce: "n-1",
      dpopKey: key,
      accountDid: "did:plc:probe",
    ))
    db.close()

  # --- reopen, as a restart would ----------------------------------------
  var db = sqlite.open(path)
  defer: db.close()
  migrate(db)   # idempotent, as it is on every boot

  block:
    let got = getSession(db, "did:plc:probe", "sess-1")
    report("session survives a restart", got.isSome)
    if got.isSome:
      let s = got.get
      report("tokens round-trip",
             s.accessToken == "access-abc" and s.refreshToken == "refresh-xyz")
      report("nonces round-trip",
             s.hostNonce == "host-n" and s.authNonce == "auth-n")
      report("scopes round-trip", s.scopes == @["atproto", "transition:generic"])

      # The check that matters.
      report("reloaded key has the same JWK thumbprint",
             s.dpopKey.thumbprint == originalThumb, s.dpopKey.thumbprint)

      const msg = "eyJhbGciOiJFUzI1NiJ9.eyJodG0iOiJHRVQifQ"
      let sig = s.dpopKey.signEs256(rng, msg)
      report("reloaded key still signs verifiably",
             key.verifyEs256(msg, sig),
             "signed with the reloaded key, verified against the original")

      # Negative control: a verifier that accepts anything proves nothing.
      let other = generateKey(rng)
      report("a different key does not verify",
             not other.verifyEs256(msg, sig))

  block:
    report("unknown session reads as none",
           getSession(db, "did:plc:probe", "nope").isNone)
    report("sessions are listed per account",
           listSessions(db, "did:plc:probe") == @["sess-1"])
    report("accounts are listed", listAccounts(db) == @["did:plc:probe"])

  block:
    report("cursor survives a restart",
           loadCursor(db) == 1_700_000_000_000_000'i64, $loadCursor(db))

  # --- auth requests are single-use --------------------------------------
  block:
    let first = takeAuthRequest(db, "state-1")
    report("pending auth request survives a restart", first.isSome)
    if first.isSome:
      report("PKCE verifier round-trips", first.get.pkceVerifier == "verifier-1")
      report("auth request key round-trips",
             first.get.dpopKey.thumbprint == originalThumb)
    # An authorization code is single-use; leaving the row behind invites a
    # replayed callback.
    report("taking an auth request consumes it",
           takeAuthRequest(db, "state-1").isNone)

  # --- batched cursor store ----------------------------------------------
  block:
    let cs = newDbCursorStore(db, flushEvery = 3600)  # never auto-flush here
    report("cursor store loads the persisted value",
           cs.load() == 1_700_000_000_000_000'i64)

    cs.save(1_700_000_000_500_000'i64)
    # Batching means the write is deferred, not lost.
    report("a fresh cursor is not written immediately",
           loadCursor(db) == 1_700_000_000_000_000'i64)
    cs.close()
    report("closing flushes the pending cursor",
           loadCursor(db) == 1_700_000_000_500_000'i64, $loadCursor(db))

    # Cursors must never move backwards, or a late event would rewind the
    # subscription on the next reconnect.
    let cs2 = newDbCursorStore(db, flushEvery = 0)
    cs2.save(1'i64)
    cs2.close()
    report("an older cursor does not overwrite a newer one",
           loadCursor(db) == 1_700_000_000_500_000'i64, $loadCursor(db))

  # --- the seam with the supervisor --------------------------------------
  # jetstream_run takes a CursorStore base reference, so this only works if
  # the overrides dispatch dynamically. A silent fall-through to the base
  # method would read the in-memory field instead of the database and look
  # entirely healthy while resuming from the wrong place.
  block:
    let cs = newDbCursorStore(db, flushEvery = 3600)
    let asBase: CursorStore = cs
    report("DbCursorStore dispatches through the CursorStore base",
           asBase.load() == loadCursor(db), $asBase.load())

    let rewound = rewoundCursor(asBase, 5.seconds)
    report("the supervisor rewinds the persisted cursor",
           rewound == loadCursor(db) - 5_000_000, $rewound)

  # --- deletion ----------------------------------------------------------
  block:
    deleteSession(db, "did:plc:probe", "sess-1")
    report("sessions can be deleted",
           getSession(db, "did:plc:probe", "sess-1").isNone and
           listAccounts(db).len == 0)

  echo ""
  if failures == 0:
    echo "RESULT: sessions, keys and cursors survive a restart."
  else:
    echo &"RESULT: {failures} check(s) failed."
    quit 1

when isMainModule:
  main()
