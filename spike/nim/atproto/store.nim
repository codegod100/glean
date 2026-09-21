## Persisting what an OAuth session and a firehose subscription need to
## survive a restart.
##
## Three things are stored, matching the tables Glean's Go backend uses:
##
##   oauth_auth_requests  a flow in progress, keyed by `state`
##   oauth_sessions       a live session, keyed by (account_did, session_id)
##   jetstream_cursor     a single row: where the firehose got to
##
## The rows hold JSON, as the Go side does, so the schema does not have to
## change every time a field is added to a session.
##
## **This stores key material.** A session row contains the DPoP private key
## and the refresh token; together they are the account. The file wants the
## same protection as a password store: not world-readable, and not in a
## backup that travels somewhere less careful.

import std/[json, options, sequtils, times]
import ./dpop
import ./oauth
import ./sqlite

type
  StoreError* = object of CatchableError

  SessionRecord* = object ## A usable OAuth session.
    accountDid*: string
    sessionId*: string
    pdsUrl*: string
    authServerUrl*: string
    tokenEndpoint*: string
    accessToken*: string
    refreshToken*: string
    dpopKey*: P256Key
    hostNonce*: string
    authNonce*: string
    scopes*: seq[string]
    createdAt*: int64

const Schema = [
  """CREATE TABLE IF NOT EXISTS oauth_auth_requests (
       state TEXT PRIMARY KEY,
       data TEXT NOT NULL,
       created_at INTEGER NOT NULL
     )""",
  """CREATE TABLE IF NOT EXISTS oauth_sessions (
       account_did TEXT NOT NULL,
       session_id TEXT NOT NULL,
       data TEXT NOT NULL,
       PRIMARY KEY (account_did, session_id)
     )""",
  """CREATE TABLE IF NOT EXISTS jetstream_cursor (
       id INTEGER PRIMARY KEY CHECK(id = 1),
       cursor_us INTEGER NOT NULL
     )""",
]

proc migrate*(db: Db) =
  for stmt in Schema:
    db.exec stmt

# --- key serialisation -----------------------------------------------------

proc toJson*(k: P256Key): JsonNode =
  ## The private scalar plus the public point.
  ##
  ## The public half is stored rather than re-derived on load: deriving it
  ## needs a curve operation, and a stored key that cannot be read back
  ## without one is a restart that silently drops every session.
  %*{"d": b64u(k.priv), "x": b64u(k.pubX), "y": b64u(k.pubY)}

proc keyFromJson*(j: JsonNode): P256Key =
  proc part(name: string): seq[byte] =
    let raw = b64uDecode(j{name}.getStr)
    if raw.len != 32:
      raise newException(StoreError, "stored key field " & name & " is not 32 bytes")
    result = newSeq[byte](32)
    copyMem(result[0].addr, raw[0].unsafeAddr, 32)

  let d = part("d")
  let x = part("x")
  let y = part("y")
  for i in 0 ..< 32:
    result.priv[i] = d[i]
    result.pubX[i] = x[i]
    result.pubY[i] = y[i]

# --- auth requests ---------------------------------------------------------

proc toJson(info: AuthRequestData): JsonNode =
  %*{
    "state": info.state,
    "auth_server_url": info.authServerUrl,
    "scopes": info.scopes,
    "pkce_verifier": info.pkceVerifier,
    "request_uri": info.requestUri,
    "token_endpoint": info.tokenEndpoint,
    "revocation_endpoint": info.revocationEndpoint,
    "dpop_nonce": info.dpopNonce,
    "dpop_key": info.dpopKey.toJson,
    "account_did": info.accountDid,
  }

proc authRequestFromJson(j: JsonNode): AuthRequestData =
  AuthRequestData(
    state: j{"state"}.getStr,
    authServerUrl: j{"auth_server_url"}.getStr,
    scopes: j{"scopes"}.getElems.mapIt(it.getStr),
    pkceVerifier: j{"pkce_verifier"}.getStr,
    requestUri: j{"request_uri"}.getStr,
    tokenEndpoint: j{"token_endpoint"}.getStr,
    revocationEndpoint: j{"revocation_endpoint"}.getStr,
    dpopNonce: j{"dpop_nonce"}.getStr,
    dpopKey: keyFromJson(j{"dpop_key"}),
    accountDid: j{"account_did"}.getStr,
  )

proc saveAuthRequest*(db: Db, info: AuthRequestData) =
  db.run("""INSERT INTO oauth_auth_requests (state, data, created_at)
            VALUES (?, ?, ?)
            ON CONFLICT(state) DO UPDATE SET data = excluded.data""",
         p(info.state), p($info.toJson), p(getTime().toUnix))

proc takeAuthRequest*(db: Db, state: string): Option[AuthRequestData] =
  ## Read **and delete**: an authorization code is single-use, so leaving the
  ## request behind invites a replay of the callback.
  let row = db.queryText(
    "SELECT data FROM oauth_auth_requests WHERE state = ?", p(state))
  if row.isNone:
    return none(AuthRequestData)
  db.run("DELETE FROM oauth_auth_requests WHERE state = ?", p(state))
  try:
    some(authRequestFromJson(parseJson(row.get)))
  except JsonParsingError:
    raise newException(StoreError, "stored auth request is not valid JSON")

proc purgeAuthRequests*(db: Db, olderThan: int64) =
  ## Flows that were started and never finished. Without this the table grows
  ## forever, one row per abandoned sign-in.
  db.run("DELETE FROM oauth_auth_requests WHERE created_at < ?",
         p(getTime().toUnix - olderThan))

# --- sessions --------------------------------------------------------------

proc toJson(s: SessionRecord): JsonNode =
  %*{
    "account_did": s.accountDid,
    "session_id": s.sessionId,
    "pds_url": s.pdsUrl,
    "auth_server_url": s.authServerUrl,
    "token_endpoint": s.tokenEndpoint,
    "access_token": s.accessToken,
    "refresh_token": s.refreshToken,
    "dpop_key": s.dpopKey.toJson,
    "host_nonce": s.hostNonce,
    "auth_nonce": s.authNonce,
    "scopes": s.scopes,
    "created_at": s.createdAt,
  }

proc sessionFromJson(j: JsonNode): SessionRecord =
  SessionRecord(
    accountDid: j{"account_did"}.getStr,
    sessionId: j{"session_id"}.getStr,
    pdsUrl: j{"pds_url"}.getStr,
    authServerUrl: j{"auth_server_url"}.getStr,
    tokenEndpoint: j{"token_endpoint"}.getStr,
    accessToken: j{"access_token"}.getStr,
    refreshToken: j{"refresh_token"}.getStr,
    dpopKey: keyFromJson(j{"dpop_key"}),
    hostNonce: j{"host_nonce"}.getStr,
    authNonce: j{"auth_nonce"}.getStr,
    scopes: j{"scopes"}.getElems.mapIt(it.getStr),
    createdAt: j{"created_at"}.getBiggestInt.int64,
  )

proc saveSession*(db: Db, s: SessionRecord) =
  if s.accountDid.len == 0 or s.sessionId.len == 0:
    raise newException(StoreError, "a session needs both a DID and a session id")
  db.run("""INSERT INTO oauth_sessions (account_did, session_id, data)
            VALUES (?, ?, ?)
            ON CONFLICT(account_did, session_id) DO UPDATE SET data = excluded.data""",
         p(s.accountDid), p(s.sessionId), p($s.toJson))

proc getSession*(db: Db, did, sessionId: string): Option[SessionRecord] =
  let row = db.queryText(
    "SELECT data FROM oauth_sessions WHERE account_did = ? AND session_id = ?",
    p(did), p(sessionId))
  if row.isNone:
    return none(SessionRecord)
  try:
    some(sessionFromJson(parseJson(row.get)))
  except JsonParsingError:
    raise newException(StoreError, "stored session is not valid JSON")

proc deleteSession*(db: Db, did, sessionId: string) =
  db.run("DELETE FROM oauth_sessions WHERE account_did = ? AND session_id = ?",
         p(did), p(sessionId))

proc listSessions*(db: Db, did: string): seq[string] =
  for row in db.rows(
      "SELECT session_id FROM oauth_sessions WHERE account_did = ?", p(did)):
    result.add row[0]

proc listAccounts*(db: Db): seq[string] =
  for row in db.rows("SELECT DISTINCT account_did FROM oauth_sessions"):
    result.add row[0]

# --- jetstream cursor ------------------------------------------------------

proc saveCursor*(db: Db, cursorUs: int64) =
  db.run("""INSERT INTO jetstream_cursor (id, cursor_us) VALUES (1, ?)
            ON CONFLICT(id) DO UPDATE SET cursor_us = excluded.cursor_us""",
         p(cursorUs))

proc loadCursor*(db: Db): int64 =
  db.queryInt("SELECT cursor_us FROM jetstream_cursor WHERE id = 1").get(0'i64)
