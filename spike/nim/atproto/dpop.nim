## ES256 keys and DPoP proofs (RFC 9449) for ATProto OAuth.
##
## What has to be exactly right:
##   * ES256 signatures in JOSE form -- raw R||S, 64 bytes, NOT the DER that
##     most crypto libraries hand you.
##   * An RFC 7638 JWK thumbprint, which is a SHA-256 over canonical JSON:
##     exactly the members crv/kty/x/y, lexicographically ordered, no spaces.
##   * base64url without padding, everywhere.
##
## bearssl is used because br_ecdsa_sign_raw already emits R||S, so there is no
## DER unwrapping step to get wrong. dpop_probe.nim cross-checks the output
## against a Go verifier.

import std/[base64, json, strutils, times, strformat]
import bearssl/[ec, rand]
import bearssl/abi/[bearssl_ec, bearssl_rand, bearssl_hash]
import nimcrypto/[sha2, hash]

type P256Key* = object
  priv*: array[32, byte]
  pubX*: array[32, byte]
  pubY*: array[32, byte]

proc toSeq(a: openArray[byte]): seq[byte] =
  result = newSeq[byte](a.len)
  for i, b in a: result[i] = b

proc b64u*(data: openArray[byte]): string =
  ## base64url, unpadded (RFC 7515 §2).
  base64.encode(data, safe = true).strip(chars = {'='})

proc b64u*(s: string): string =
  b64u(s.toOpenArrayByte(0, s.high).toSeq)

proc generateKey*(rng: var HmacDrbgContext): P256Key =
  ## P-256 keypair. bearssl returns the public point uncompressed:
  ## 0x04 || X(32) || Y(32).
  var
    skBuf: array[EC_KBUF_PRIV_MAX_SIZE, byte]
    pkBuf: array[EC_KBUF_PUB_MAX_SIZE, byte]
    sk: EcPrivateKey
    pk: EcPublicKey

  let n = ecKeygen(PrngClassPointerConst(addr rng.vtable), addr ecPrimeI31, addr sk,
                   addr skBuf[0], EC_secp256r1.cint)
  doAssert n > 0, "ecKeygen failed"
  let m = ecComputePub(addr ecPrimeI31, addr pk, addr pkBuf[0], addr sk)
  doAssert m > 0, "ecComputePub failed"

  doAssert sk.xlen == 32, &"unexpected private key length {sk.xlen}"
  doAssert pk.qlen == 65 and cast[ptr UncheckedArray[byte]](pk.q)[0] == 0x04'u8,
           "expected an uncompressed public point"

  let
    skBytes = cast[ptr UncheckedArray[byte]](sk.x)
    pkBytes = cast[ptr UncheckedArray[byte]](pk.q)
  for i in 0 ..< 32:
    result.priv[i] = skBytes[i]
    result.pubX[i] = pkBytes[1 + i]
    result.pubY[i] = pkBytes[33 + i]

proc publicJwk*(k: P256Key): JsonNode =
  %*{"crv": "P-256", "kty": "EC", "x": b64u(k.pubX), "y": b64u(k.pubY)}

proc thumbprint*(k: P256Key): string =
  ## RFC 7638: SHA-256 over the canonical JWK. The member set and ordering are
  ## fixed by the spec, so this is built by hand rather than serialised from a
  ## JsonNode whose key order is incidental.
  let canonical = &"""{{"crv":"P-256","kty":"EC","x":"{b64u(k.pubX)}","y":"{b64u(k.pubY)}"}}"""
  b64u(sha256.digest(canonical).data)

proc signEs256*(k: P256Key, rng: var HmacDrbgContext, signingInput: string): seq[byte] =
  var
    sk: EcPrivateKey
    priv = k.priv
  sk.curve = EC_secp256r1.cint
  sk.x = cast[ptr byte](addr priv[0])
  sk.xlen = 32

  let digest = sha256.digest(signingInput).data
  var sig: array[64, byte]
  let n = ecdsaSignRawGetDefault()(
    addr ecPrimeI31, addr sha256Vtable, unsafeAddr digest[0], addr sk, addr sig[0])
  doAssert n == 64, &"expected a 64-byte raw signature, got {n}"
  result = sig.toSeq

proc dpopProof*(k: P256Key, rng: var HmacDrbgContext, htm, htu: string,
                nonce = ""): string =
  ## A DPoP proof JWT (RFC 9449) as an ATProto PDS expects it.
  let header = %*{"typ": "dpop+jwt", "alg": "ES256", "jwk": k.publicJwk}
  var payload = %*{
    "jti": b64u(sha256.digest(&"{htm}{htu}{epochTime()}").data)[0 ..< 16],
    "htm": htm,
    "htu": htu,
    "iat": now().utc.toTime.toUnix,
  }
  if nonce.len > 0:
    payload["nonce"] = %nonce

  let signingInput = b64u($header) & "." & b64u($payload)
  signingInput & "." & b64u(k.signEs256(rng, signingInput))

