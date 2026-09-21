## Authenticated XRPC against a PDS, using a DPoP-bound OAuth access token.
##
## The important difference from the OAuth endpoints: a resource server
## signals a stale nonce with **401** and a `WWW-Authenticate: DPoP
## error="use_dpop_nonce"` header, where the auth server used 400 and a JSON
## body. Same protocol, different shape, so the retry logic cannot be shared
## by pattern-matching the status code alone.
##
## A session therefore tracks two nonces: one for the auth server (token
## refresh) and one for the PDS. They are issued independently and are not
## interchangeable.

import std/[httpclient, json, strformat, strutils, uri]
import bearssl/rand
import ./dpop
import ./identity
import ./oauth

type
  XrpcError* = object of CatchableError
    status*: int
    xrpcError*: string   ## the `error` field of an XRPC error body

  Session* = object ## Everything needed to make an authenticated call.
    did*: string
    pdsUrl*: string
    authServerUrl*: string
    tokenEndpoint*: string
    accessToken*: string
    refreshToken*: string
    dpopKey*: P256Key
    hostNonce*: string   ## nonce issued by the PDS
    authNonce*: string   ## nonce issued by the auth server
    clientId*: string

const HttpTimeoutMs = 30_000

proc sessionFromTokens*(info: AuthRequestData, tokens: TokenResponse,
                        pdsUrl, clientId: string): Session =
  Session(
    did: tokens.sub,
    pdsUrl: pdsUrl.strip(chars = {'/'}),
    authServerUrl: info.authServerUrl,
    tokenEndpoint: info.tokenEndpoint,
    accessToken: tokens.accessToken,
    refreshToken: tokens.refreshToken,
    dpopKey: info.dpopKey,
    authNonce: info.dpopNonce,
    clientId: clientId,
  )

proc dpopUrl*(url: string): string =
  ## The `htu` claim is the request URL without query or fragment (RFC 9449
  ## §4.2). Leaving the query on is a silent mismatch: the proof verifies
  ## against a URL the server never compares it to, and every call 401s.
  let u = parseUri(url)
  var bare = u
  bare.query = ""
  bare.anchor = ""
  $bare

proc isNonceRetry(wwwAuth: string): bool =
  ## WWW-Authenticate: DPoP error="use_dpop_nonce", error_description="..."
  "use_dpop_nonce" in wwwAuth

proc isExpiredToken(wwwAuth: string): bool =
  "invalid_token" in wwwAuth

proc parseXrpcError(body: string): (string, string) =
  try:
    let j = parseJson(body)
    (j{"error"}.getStr, j{"message"}.getStr)
  except CatchableError:
    ("", "")

proc raiseXrpc(status: int, body: string, what: string) =
  let (err, msg) = parseXrpcError(body)
  var e = newException(XrpcError,
    &"{what} failed (HTTP {status})" &
    (if err.len > 0: &": {err}" else: "") &
    (if msg.len > 0: &" -- {msg}" else: ""))
  e.status = status
  e.xrpcError = err
  raise e

proc refreshSession*(sess: var Session, rng: var HmacDrbgContext) =
  ## Exchange the refresh token for a new access token, reusing the session's
  ## DPoP key -- the refresh token is bound to it just as the access token is.
  if sess.refreshToken.len == 0:
    raise newException(XrpcError, "session has no refresh token")
  var info = AuthRequestData(
    authServerUrl: sess.authServerUrl,
    tokenEndpoint: sess.tokenEndpoint,
    dpopKey: sess.dpopKey,
    dpopNonce: sess.authNonce,
  )
  let cfg = ClientConfig(clientId: sess.clientId)
  let tokens = refresh(cfg, info, sess.refreshToken, rng)
  sess.accessToken = tokens.accessToken
  if tokens.refreshToken.len > 0:
    sess.refreshToken = tokens.refreshToken
  sess.authNonce = info.dpopNonce

proc request*(sess: var Session, rng: var HmacDrbgContext,
              httpMethod: HttpMethod, url: string,
              body = "", contentType = "application/json"): Response =
  ## One authenticated request, handling both retry conditions.
  ##
  ## Up to three attempts: the original, one for a nonce the PDS only reveals
  ## by rejecting us, and one after refreshing an expired access token.
  let client = newHttpClient(userAgent = UserAgent, timeout = HttpTimeoutMs)
  defer: client.close()

  let htu = dpopUrl(url)
  # HttpMethod stringifies straight to the uppercase token the `htm` claim
  # wants ("GET", "POST"), so no trimming.
  let htm = $httpMethod
  var refreshed = false
  var noncedOnce = false

  for attempt in 0 .. 2:
    let proof = dpopProof(sess.dpopKey, rng, htm, htu,
                          nonce = sess.hostNonce,
                          accessToken = sess.accessToken,
                          issuer = sess.authServerUrl)
    var headers = @{
      "Authorization": "DPoP " & sess.accessToken,  # not Bearer
      "DPoP": proof,
    }
    if body.len > 0:
      headers.add ("Content-Type", contentType)
    client.headers = newHttpHeaders(headers)

    result = client.request(url, httpMethod = httpMethod, body = body)

    # The PDS rotates nonces freely; adopt any it offers, success or not.
    let supplied = result.headers.getOrDefault("DPoP-Nonce").string
    if supplied.len > 0:
      sess.hostNonce = supplied

    if result.code.int != 401:
      return result

    let wwwAuth = result.headers.getOrDefault("WWW-Authenticate").string
    if wwwAuth.len == 0:
      return result

    if isNonceRetry(wwwAuth) and supplied.len > 0 and not noncedOnce:
      noncedOnce = true
      continue

    if isExpiredToken(wwwAuth) and not refreshed and sess.refreshToken.len > 0:
      refreshed = true
      refreshSession(sess, rng)
      continue

    return result

  raise newException(XrpcError, "ran out of retries for " & url)

proc query*(sess: var Session, rng: var HmacDrbgContext, nsid: string,
            params: openArray[(string, string)] = []): JsonNode =
  ## An XRPC query (GET), e.g. com.atproto.repo.listRecords.
  var url = &"{sess.pdsUrl}/xrpc/{nsid}"
  if params.len > 0:
    url &= "?" & params.encodeQuery
  let res = request(sess, rng, HttpGet, url)
  if res.code.int != 200:
    raiseXrpc(res.code.int, res.body, nsid)
  try:
    parseJson(res.body)
  except JsonParsingError:
    raise newException(XrpcError, nsid & " returned a non-JSON body")

proc procedure*(sess: var Session, rng: var HmacDrbgContext, nsid: string,
                body: JsonNode = nil): JsonNode =
  ## An XRPC procedure (POST), e.g. com.atproto.repo.createRecord.
  let url = &"{sess.pdsUrl}/xrpc/{nsid}"
  let payload = if body == nil: "" else: $body
  let res = request(sess, rng, HttpPost, url, body = payload)
  if res.code.int notin [200, 201]:
    raiseXrpc(res.code.int, res.body, nsid)
  if res.body.strip.len == 0:
    return newJObject()
  try:
    parseJson(res.body)
  except JsonParsingError:
    raise newException(XrpcError, nsid & " returned a non-JSON body")

# --- unauthenticated helpers ----------------------------------------------

proc publicQuery*(host, nsid: string,
                  params: openArray[(string, string)] = []): JsonNode =
  ## Many XRPC endpoints need no auth. Useful on its own, and it isolates
  ## "can we speak XRPC at all" from "is our DPoP signing right".
  var url = &"https://{hostOf(host)}/xrpc/{nsid}"
  if params.len > 0:
    url &= "?" & params.encodeQuery
  let client = newHttpClient(userAgent = UserAgent, timeout = HttpTimeoutMs)
  defer: client.close()
  let res = client.get(url)
  if res.code.int != 200:
    raiseXrpc(res.code.int, res.body, nsid)
  parseJson(res.body)
