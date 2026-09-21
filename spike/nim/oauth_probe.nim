## Drives the OAuth flow against a real PDS, as far as it can go without a
## human at a browser.
##
##   nim c -r -d:ssl oauth_probe.nim [handle]
##
## Everything here is unauthenticated and read-only except the PAR, which
## creates a short-lived pending authorization the server discards on its own.
## Getting a request_uri back is the meaningful result: it means the DPoP
## proof was accepted, the nonce retry worked, and PKCE was well-formed.

import std/[httpclient, json, os, strformat, strutils, uri]
import bearssl/rand
import atproto/[dpop, identity, oauth]

var failures = 0

proc report(name: string, ok: bool, detail = "") =
  if not ok: inc failures
  let label = if ok: "PASS" else: "FAIL"
  if detail.len > 0:
    echo &"  {label}  {name}  -- {detail}"
  else:
    echo &"  {label}  {name}"

when isMainModule:
  let handle = if paramCount() >= 1: paramStr(1) else: "bsky.app"
  echo "ATProto OAuth flow probe"
  echo &"  identifier: {handle}"
  echo ""

  let rngRef = HmacDrbgContext.new()
  doAssert rngRef != nil, "no system randomness"
  var rng = rngRef[]

  # --- resolution ---------------------------------------------------------
  var ident: Identity
  try:
    ident = lookup(handle)
    report("handle resolves to a DID", ident.did.isDid, ident.did)
    report("identity links to a PDS", ident.pdsEndpoint.len > 0, ident.pdsEndpoint)
  except CatchableError as e:
    report("handle resolves", false, e.msg)
    quit 1

  # --- discovery ----------------------------------------------------------
  var authServerUrl: string
  try:
    authServerUrl = resolveAuthServerUrl(ident.pdsEndpoint)
    report("PDS advertises an auth server", authServerUrl.len > 0, authServerUrl)
  except CatchableError as e:
    report("PDS advertises an auth server", false, e.msg)
    quit 1

  var meta: AuthServerMetadata
  try:
    meta = resolveAuthServerMetadata(authServerUrl)
    report("auth server metadata fetched", true, meta.issuer)
    report("has a PAR endpoint", meta.parEndpoint.len > 0, meta.parEndpoint)
    report("has a token endpoint", meta.tokenEndpoint.len > 0, meta.tokenEndpoint)
  except CatchableError as e:
    report("auth server metadata fetched", false, e.msg)
    quit 1

  # --- PKCE ---------------------------------------------------------------
  block:
    # RFC 7636 test vector: this verifier must produce this challenge.
    const
      verifier = "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
      want = "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"
    report("PKCE S256 matches RFC 7636 vector",
           s256Challenge(verifier) == want, s256Challenge(verifier))

  # --- the nonce dance ----------------------------------------------------
  # sendAuthRequest retries transparently, which would hide a server that
  # never demanded a nonce. Prove the first unnonced attempt really is
  # rejected, so the retry path is load-bearing rather than decorative.
  let cfg0 = newLocalhostConfig("http://127.0.0.1:8080/callback")
  block:
    let
      key = generateKey(rng)
      proof = dpopProof(key, rng, "POST", meta.parEndpoint)
      client = newHttpClient(userAgent = UserAgent, timeout = 15_000)
    defer: client.close()
    client.headers = newHttpHeaders({
      "Content-Type": "application/x-www-form-urlencoded",
      "DPoP": proof,
    })
    # The body must be fully valid: bsky.social validates parameters before it
    # checks the nonce, so a stub body returns invalid_request and tells us
    # nothing about the nonce requirement.
    let verifier = secureRandomB64(48)
    let body = @{
      "client_id": cfg0.clientId,
      "state": secureRandomB64(16),
      "redirect_uri": cfg0.callbackUrl,
      "scope": scopeStr(cfg0.scopes),
      "response_type": "code",
      "code_challenge": s256Challenge(verifier),
      "code_challenge_method": "S256",
    }.encodeQuery
    let res = client.post(meta.parEndpoint, body = body)
    let code = try: parseJson(res.body){"error"}.getStr except CatchableError: ""
    let nonce = res.headers.getOrDefault("DPoP-Nonce").string
    report("first PAR without a nonce is rejected",
           res.code.int == 400 and code == "use_dpop_nonce", &"HTTP {res.code.int} {code}")
    report("rejection supplies a DPoP-Nonce to retry with", nonce.len > 0)

  # --- PAR ----------------------------------------------------------------
  # A localhost client needs no hosted metadata document, so the flow can be
  # exercised without deploying anything.
  let cfg = cfg0
  report("localhost client_id is well-formed",
         cfg.clientId.startsWith("http://localhost?"), cfg.clientId)

  try:
    let info = sendAuthRequest(cfg, meta, rng, loginHint = handle)
    report("PAR accepted (DPoP proof + nonce retry + PKCE)",
           info.requestUri.startsWith("urn:ietf:params:oauth:request_uri:"),
           info.requestUri)
    report("auth server issued a DPoP nonce", info.dpopNonce.len > 0,
           info.dpopNonce[0 ..< min(12, info.dpopNonce.len)] & "...")
    report("PKCE verifier retained for callback", info.pkceVerifier.len >= 43)
    report("state is unguessable", info.state.len >= 21, &"{info.state.len} chars")

    let url = authorizeUrl(cfg, meta, info)
    let q = parseUri(url).query
    report("authorize URL carries request_uri", "request_uri=" in q)
    report("authorize URL carries client_id", "client_id=" in q)
    echo ""
    echo "  authorize URL:"
    echo "  ", url
  except CatchableError as e:
    report("PAR accepted", false, e.msg)

  echo ""
  if failures == 0:
    echo "RESULT: the OAuth flow works against a live PDS up to user consent."
  else:
    echo &"RESULT: {failures} check(s) failed."
    quit 1
