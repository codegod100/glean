## Cross-checks atproto/dpop.nim against an independent Go verifier.
##
##   nim c -r dpop_probe.nim && ./dpop_probe | (cd verify && go run .)

import std/json
import bearssl/rand
import atproto/dpop

when isMainModule:
  let rngRef = HmacDrbgContext.new()
  doAssert rngRef != nil, "no system randomness"
  var rng = rngRef[]

  let key = generateKey(rng)
  let jwt = key.dpopProof(rng, "POST", "https://bsky.social/oauth/token",
                          nonce = "spike-nonce")

  # Emitted for verify/verify.go to check independently.
  echo $(%*{
    "jwt": jwt,
    "jwk": key.publicJwk,
    "thumbprint": key.thumbprint,
  })
