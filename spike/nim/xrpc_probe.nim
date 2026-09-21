## Exercises the XRPC layer as far as it goes without a consented session.
##
##   nim c -r -d:ssl xrpc_probe.nim
##
## A valid access token needs a human in a browser, so the authenticated path
## is probed with a deliberately invalid one. That still proves the parts most
## likely to be wrong: that a real PDS accepts the *shape* of our request and
## rejects it for the right reason, and that the 401/WWW-Authenticate nonce
## handshake behaves the way the retry logic assumes.

import std/[httpclient, json, strformat, strutils]
import bearssl/rand
import atproto/[dpop, identity, xrpc]

var failures = 0

proc report(name: string, ok: bool, detail = "") =
  if not ok: inc failures
  let label = if ok: "PASS" else: "FAIL"
  if detail.len > 0: echo &"  {label}  {name}  -- {detail}"
  else: echo &"  {label}  {name}"

when isMainModule:
  echo "XRPC signing probe"
  echo ""

  let rngRef = HmacDrbgContext.new()
  doAssert rngRef != nil
  var rng = rngRef[]

  # --- claim construction -------------------------------------------------
  block:
    report("htu strips the query string",
           dpopUrl("https://pds.example/xrpc/a.b.c?limit=5#frag") ==
             "https://pds.example/xrpc/a.b.c",
           dpopUrl("https://pds.example/xrpc/a.b.c?limit=5#frag"))

  block:
    # Pinned against an independently computed value:
    #   printf 'test-access-token' | openssl dgst -sha256 -binary \
    #     | basenc --base64url | tr -d '='
    # A self-consistent ath would pass any internal check and still be
    # rejected by every PDS.
    const
      token = "test-access-token"
      want = "WXSA1LYsphIZPxnnP-TMOtF_C_nPwWp8v0tQZBMcSAU"
    let ath = accessTokenHash(token)
    report("ath matches an independently computed SHA-256", ath == want, ath)
    report("ath is unpadded base64url", ath.len == 43 and '=' notin ath)

  block:
    let key = generateKey(rng)
    let proof = dpopProof(key, rng, "GET", "https://pds.example/xrpc/a.b.c",
                          accessToken = "tok", issuer = "https://auth.example")
    let payload = parseJson(b64uDecode(proof.split('.')[1]))
    report("resource proof carries ath", payload.hasKey("ath"))
    report("resource proof carries iss", payload{"iss"}.getStr == "https://auth.example")
    report("resource proof carries exp", payload.hasKey("exp"))

    let authProof = dpopProof(key, rng, "POST", "https://auth.example/oauth/par")
    let authPayload = parseJson(b64uDecode(authProof.split('.')[1]))
    # An auth-server proof must NOT claim to be bound to a token.
    report("auth-server proof omits ath", not authPayload.hasKey("ath"))

  # --- unauthenticated XRPC -----------------------------------------------
  # Separates "can we speak XRPC" from "is our signing right".
  block:
    try:
      let j = publicQuery("public.api.bsky.app", "app.bsky.actor.getProfile",
                          {"actor": "bsky.app"})
      report("unauthenticated XRPC query works", j{"did"}.getStr.isDid,
             j{"handle"}.getStr)
    except CatchableError as e:
      report("unauthenticated XRPC query works", false, e.msg)

  # --- the authenticated path, against a real PDS -------------------------
  var ident: Identity
  try:
    ident = lookup("bsky.app")
  except CatchableError as e:
    report("resolve a PDS to talk to", false, e.msg)
    quit 1
  report("resolved a PDS", true, ident.pdsEndpoint)

  block:
    # A well-formed DPoP request carrying a token the PDS has never issued.
    # The interesting question is *which* error comes back: a shape problem
    # would read as invalid_dpop_proof, whereas invalid_token means the PDS
    # parsed and accepted everything except the credential itself.
    var sess = Session(
      did: ident.did,
      pdsUrl: ident.pdsEndpoint,
      authServerUrl: "https://bsky.social",
      accessToken: "not-a-real-token",
      dpopKey: generateKey(rng),
      clientId: "http://localhost",
    )
    # getSession, not listRecords: repo reads are public, so a PDS serves them
    # happily while ignoring the Authorization header entirely -- which looks
    # like a pass and tests nothing. This endpoint exists to answer "who am
    # I", so it has to evaluate the credential.
    let url = &"{sess.pdsUrl}/xrpc/com.atproto.server.getSession"
    let res = request(sess, rng, HttpGet, url)

    report("PDS rejects an unknown token with 401", res.code.int == 401,
           &"HTTP {res.code.int}")

    let wwwAuth = res.headers.getOrDefault("WWW-Authenticate").string
    report("401 carries WWW-Authenticate", wwwAuth.len > 0, wwwAuth)
    report("challenge is DPoP, not Bearer",
           wwwAuth.startsWith("DPoP"), wwwAuth.split(' ')[0])
    report("the proof itself was accepted (error is about the token, "&
           "not the proof)",
           wwwAuth.len > 0 and "invalid_dpop_proof" notin wwwAuth, wwwAuth)
    report("PDS issued a host nonce for the retry", sess.hostNonce.len > 0,
           sess.hostNonce[0 ..< min(12, sess.hostNonce.len)] & "...")

  echo ""
  if failures == 0:
    echo "RESULT: XRPC signing is correct as far as an unconsented session can show."
  else:
    echo &"RESULT: {failures} check(s) failed."
    quit 1
