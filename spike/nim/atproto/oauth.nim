## ATProto OAuth client: metadata discovery, PAR, authorization URL, and the
## token exchange, ported from the shape of bluesky-social/indigo's
## atproto/auth/oauth package.
##
## Two details drive most of the design:
##
## 1. Every request to the auth server carries a DPoP proof bound to a
##    per-session P-256 key, and the server will reject the first one with
##    `use_dpop_nonce` plus a DPoP-Nonce header. That is normal, not an error:
##    the request must be retried once with the supplied nonce. Note the
##    status is 400 on the auth server (not the 401 a resource server uses).
##
## 2. PKCE is mandatory. The verifier is kept alongside the request so the
##    callback can present it.

import std/[httpclient, json, strutils, strformat, sysrand, uri]
import nimcrypto/[sha2, hash]
import bearssl/rand
import ./dpop
import ./identity

type
  OAuthError* = object of CatchableError

  ClientConfig* = object
    clientId*: string
    callbackUrl*: string
    scopes*: seq[string]

  AuthServerMetadata* = object
    issuer*: string
    authorizationEndpoint*: string
    tokenEndpoint*: string
    parEndpoint*: string
    revocationEndpoint*: string

  AuthRequestData* = object ## Persisted between the redirect and the callback.
    state*: string
    authServerUrl*: string
    scopes*: seq[string]
    pkceVerifier*: string
    requestUri*: string
    tokenEndpoint*: string
    revocationEndpoint*: string
    dpopNonce*: string
    dpopKey*: P256Key
    accountDid*: string

  TokenResponse* = object
    accessToken*: string
    refreshToken*: string
    tokenType*: string
    expiresIn*: int
    scope*: string
    sub*: string

const HttpTimeoutMs = 15_000

proc newClient(): HttpClient =
  newHttpClient(userAgent = UserAgent, timeout = HttpTimeoutMs)

proc scopeStr*(scopes: seq[string]): string = scopes.join(" ")

proc newPublicConfig*(clientId, callbackUrl: string,
                      scopes = @["atproto", "transition:generic"]): ClientConfig =
  ClientConfig(clientId: clientId, callbackUrl: callbackUrl, scopes: scopes)

proc newLocalhostConfig*(callbackUrl: string,
                         scopes = @["atproto", "transition:generic"]): ClientConfig =
  ## Localhost development clients have no hosted metadata document; the
  ## client_id encodes the redirect and scopes instead.
  var params = @[("redirect_uri", callbackUrl), ("scope", scopeStr(scopes))]
  ClientConfig(
    clientId: "http://localhost" & "?" & params.encodeQuery,
    callbackUrl: callbackUrl,
    scopes: scopes,
  )

proc clientMetadata*(cfg: ClientConfig): JsonNode =
  ## Served at the client_id URL for a public (non-confidential) client.
  %*{
    "client_id": cfg.clientId,
    "application_type": "web",
    "client_name": "glean",
    "grant_types": ["authorization_code", "refresh_token"],
    "scope": scopeStr(cfg.scopes),
    "response_types": ["code"],
    "redirect_uris": [cfg.callbackUrl],
    "token_endpoint_auth_method": "none",
    "dpop_bound_access_tokens": true,
  }

proc secureRandomB64*(n: int): string =
  var buf = newSeq[byte](n)
  doAssert urandom(buf), "system randomness unavailable"
  b64u(buf)

proc s256Challenge*(verifier: string): string =
  b64u(sha256.digest(verifier).data)

# --- discovery -------------------------------------------------------------

proc resolveAuthServerUrl*(pdsUrl: string): string =
  ## The PDS advertises which authorization server governs it.
  let url = &"https://{hostOf(pdsUrl)}/.well-known/oauth-protected-resource"
  let client = newClient()
  defer: client.close()
  let res = client.get(url)
  if res.code.int != 200:
    raise newException(OAuthError,
      &"protected resource metadata failed (HTTP {res.code.int}) at {url}")
  let body = parseJson(res.body)
  let servers = body{"authorization_servers"}
  if servers == nil or servers.kind != JArray or servers.len == 0:
    raise newException(OAuthError, "PDS lists no authorization servers: " & pdsUrl)
  servers[0].getStr.strip(chars = {'/'})

proc resolveAuthServerMetadata*(authServerUrl: string): AuthServerMetadata =
  let url = &"https://{hostOf(authServerUrl)}/.well-known/oauth-authorization-server"
  let client = newClient()
  defer: client.close()
  let res = client.get(url)
  if res.code.int != 200:
    raise newException(OAuthError,
      &"auth server metadata failed (HTTP {res.code.int}) at {url}")
  let b = parseJson(res.body)

  result = AuthServerMetadata(
    issuer: b{"issuer"}.getStr,
    authorizationEndpoint: b{"authorization_endpoint"}.getStr,
    tokenEndpoint: b{"token_endpoint"}.getStr,
    parEndpoint: b{"pushed_authorization_request_endpoint"}.getStr,
    revocationEndpoint: b{"revocation_endpoint"}.getStr,
  )

  # An auth server missing any of these cannot complete an ATProto flow, and
  # failing here beats a confusing error three requests later.
  for (name, value) in {
    "issuer": result.issuer,
    "authorization_endpoint": result.authorizationEndpoint,
    "token_endpoint": result.tokenEndpoint,
    "pushed_authorization_request_endpoint": result.parEndpoint,
  }:
    if value.len == 0:
      raise newException(OAuthError, "auth server metadata is missing " & name)

  if result.issuer.strip(chars = {'/'}) != authServerUrl.strip(chars = {'/'}):
    raise newException(OAuthError,
      &"auth server issuer mismatch: metadata says {result.issuer}, fetched from {authServerUrl}")

# --- requests --------------------------------------------------------------

proc errorReason(body: string): string =
  try:
    let j = parseJson(body)
    result = j{"error"}.getStr
    let desc = j{"error_description"}.getStr
    if desc.len > 0:
      result &= ": " & desc
  except CatchableError:
    result = "unknown"
  if result.len == 0:
    result = "unknown"

proc errorCode(body: string): string =
  try: parseJson(body){"error"}.getStr
  except CatchableError: ""

proc postWithDpop(url, body: string, key: P256Key, rng: var HmacDrbgContext,
                  nonce: var string): Response =
  ## POST with a DPoP proof, retrying once when the server demands a nonce.
  ##
  ## The first request of a session always lacks a nonce, so the retry is the
  ## expected path rather than an error case. The auth server signals this
  ## with 400 + error=use_dpop_nonce and supplies DPoP-Nonce.
  let client = newClient()
  defer: client.close()

  for attempt in 0 .. 1:
    let proof = dpopProof(key, rng, "POST", url, nonce = nonce)
    client.headers = newHttpHeaders({
      "Content-Type": "application/x-www-form-urlencoded",
      "DPoP": proof,
    })
    result = client.post(url, body = body)

    let supplied = result.headers.getOrDefault("DPoP-Nonce").string
    if supplied.len > 0:
      nonce = supplied

    if attempt == 0 and result.code.int == 400 and supplied.len > 0:
      if errorCode(result.body) == "use_dpop_nonce":
        continue
    return result

proc sendAuthRequest*(cfg: ClientConfig, meta: AuthServerMetadata,
                      rng: var HmacDrbgContext, loginHint = ""): AuthRequestData =
  ## Pushed Authorization Request: hand the parameters to the auth server up
  ## front and get back a short-lived request_uri to redirect the user with.
  let
    state = secureRandomB64(16)
    verifier = secureRandomB64(48)
    key = generateKey(rng)

  var params = @[
    ("client_id", cfg.clientId),
    ("state", state),
    ("redirect_uri", cfg.callbackUrl),
    ("scope", scopeStr(cfg.scopes)),
    ("response_type", "code"),
    ("code_challenge", s256Challenge(verifier)),
    ("code_challenge_method", "S256"),
  ]
  if loginHint.len > 0:
    params.add ("login_hint", loginHint)

  var nonce = ""
  let res = postWithDpop(meta.parEndpoint, params.encodeQuery, key, rng, nonce)
  if res.code.int notin [200, 201]:
    raise newException(OAuthError,
      &"PAR failed (HTTP {res.code.int}): {errorReason(res.body)}")

  let requestUri = parseJson(res.body){"request_uri"}.getStr
  if requestUri.len == 0:
    raise newException(OAuthError, "PAR response has no request_uri")

  AuthRequestData(
    state: state,
    authServerUrl: meta.issuer,
    scopes: cfg.scopes,
    pkceVerifier: verifier,
    requestUri: requestUri,
    tokenEndpoint: meta.tokenEndpoint,
    revocationEndpoint: meta.revocationEndpoint,
    dpopNonce: nonce,
    dpopKey: key,
  )

proc authorizeUrl*(cfg: ClientConfig, meta: AuthServerMetadata,
                   info: AuthRequestData): string =
  let params = @[("client_id", cfg.clientId), ("request_uri", info.requestUri)]
  meta.authorizationEndpoint & "?" & params.encodeQuery

proc startAuthFlow*(cfg: ClientConfig, identifier: string,
                    rng: var HmacDrbgContext,
                    plcUrl = DefaultPlcUrl): (string, AuthRequestData) =
  ## Resolve an account (or take an auth server URL directly), push the
  ## request, and return the URL to send the user to.
  var
    authServerUrl: string
    accountDid: string
    loginHint: string

  if identifier.startsWith("https://"):
    authServerUrl = identifier.strip(chars = {'/'})
  else:
    let ident = lookup(identifier, plcUrl)
    accountDid = ident.did
    loginHint = identifier
    authServerUrl = resolveAuthServerUrl(ident.pdsEndpoint)

  let meta = resolveAuthServerMetadata(authServerUrl)
  var info = sendAuthRequest(cfg, meta, rng, loginHint)
  info.accountDid = accountDid
  (authorizeUrl(cfg, meta, info), info)

proc exchangeCode*(cfg: ClientConfig, info: var AuthRequestData,
                   code: string, rng: var HmacDrbgContext): TokenResponse =
  ## Trade the authorization code for tokens, proving possession of both the
  ## PKCE verifier and the session's DPoP key.
  let params = @[
    ("client_id", cfg.clientId),
    ("redirect_uri", cfg.callbackUrl),
    ("grant_type", "authorization_code"),
    ("code", code),
    ("code_verifier", info.pkceVerifier),
  ]
  var nonce = info.dpopNonce
  let res = postWithDpop(info.tokenEndpoint, params.encodeQuery,
                         info.dpopKey, rng, nonce)
  info.dpopNonce = nonce
  if res.code.int != 200:
    raise newException(OAuthError,
      &"token request failed (HTTP {res.code.int}): {errorReason(res.body)}")

  let b = parseJson(res.body)
  result = TokenResponse(
    accessToken: b{"access_token"}.getStr,
    refreshToken: b{"refresh_token"}.getStr,
    tokenType: b{"token_type"}.getStr,
    expiresIn: b{"expires_in"}.getInt,
    scope: b{"scope"}.getStr,
    sub: b{"sub"}.getStr,
  )

  # DPoP-bound tokens are useless as bearer tokens; a server answering
  # token_type=Bearer means the binding silently did not happen.
  if not result.tokenType.toLowerAscii.startsWith("dpop"):
    raise newException(OAuthError,
      "expected a DPoP-bound token, got token_type=" & result.tokenType)
  if result.sub.len == 0:
    raise newException(OAuthError, "token response has no subject DID")
  if info.accountDid.len > 0 and result.sub != info.accountDid:
    raise newException(OAuthError,
      &"token subject {result.sub} does not match requested account {info.accountDid}")

proc refresh*(cfg: ClientConfig, info: var AuthRequestData,
              refreshToken: string, rng: var HmacDrbgContext): TokenResponse =
  let params = @[
    ("client_id", cfg.clientId),
    ("grant_type", "refresh_token"),
    ("refresh_token", refreshToken),
  ]
  var nonce = info.dpopNonce
  let res = postWithDpop(info.tokenEndpoint, params.encodeQuery,
                         info.dpopKey, rng, nonce)
  info.dpopNonce = nonce
  if res.code.int != 200:
    raise newException(OAuthError,
      &"refresh failed (HTTP {res.code.int}): {errorReason(res.body)}")
  let b = parseJson(res.body)
  TokenResponse(
    accessToken: b{"access_token"}.getStr,
    refreshToken: b{"refresh_token"}.getStr,
    tokenType: b{"token_type"}.getStr,
    expiresIn: b{"expires_in"}.getInt,
    scope: b{"scope"}.getStr,
    sub: b{"sub"}.getStr,
  )
