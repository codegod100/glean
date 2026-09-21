## Handle and DID resolution: the chain that turns "alice.bsky.social" into
## the PDS host an OAuth flow has to talk to.
##
## The steps, matching what indigo's identity directory does:
##   handle -> DID     via DNS TXT _atproto.<handle>, else
##                     https://<handle>/.well-known/atproto-did
##   DID    -> doc     via the PLC directory for did:plc, or
##                     https://<host>/.well-known/did.json for did:web
##   doc    -> PDS     the service entry with id "#atproto_pds"

import std/[httpclient, json, options, os, osproc, strutils, uri]

const
  DefaultPlcUrl* = "https://plc.directory"
  UserAgent* = "glean-nim/0.1"
  HttpTimeoutMs = 10_000

type
  IdentityError* = object of CatchableError

  Identity* = object
    did*: string
    handle*: string
    pdsEndpoint*: string

proc newClient(): HttpClient =
  newHttpClient(userAgent = UserAgent, timeout = HttpTimeoutMs)

proc isValidHandle*(handle: string): bool =
  ## Deliberately loose: a dotted, non-empty ASCII domain. The authoritative
  ## check is whether it resolves.
  if handle.len == 0 or handle.len > 253 or '.' notin handle:
    return false
  for label in handle.split('.'):
    if label.len == 0: return false
    for ch in label:
      if ch notin {'a'..'z', 'A'..'Z', '0'..'9', '-'}: return false
  true

proc isDid*(s: string): bool =
  s.startsWith("did:plc:") or s.startsWith("did:web:")

proc resolveHandleDns(handle: string): Option[string] =
  ## _atproto.<handle> TXT record containing "did=did:plc:...".
  ##
  ## Nim has no resolver in the stdlib, so this shells out to `dig`. A real
  ## port should bind a resolver rather than depend on a binary being present;
  ## a missing `dig` is treated as "no DNS answer" so the HTTP fallback runs.
  let dig = findExe("dig")
  if dig.len == 0:
    return none(string)
  let (output, code) = execCmdEx(dig & " +short +time=3 +tries=1 TXT _atproto." &
                                 quoteShell(handle))
  if code != 0:
    return none(string)
  for rawLine in output.splitLines:
    let line = rawLine.strip(chars = {'"', ' ', '\t'})
    if line.startsWith("did="):
      let did = line[4 .. ^1].strip(chars = {'"'})
      if did.isDid:
        return some(did)
  none(string)

proc resolveHandleHttp(handle: string): Option[string] =
  let client = newClient()
  defer: client.close()
  try:
    let res = client.get("https://" & handle & "/.well-known/atproto-did")
    if res.code.int != 200:
      return none(string)
    let did = res.body.strip()
    if did.isDid: some(did) else: none(string)
  except CatchableError:
    none(string)

proc resolveHandle*(handle: string): string =
  ## DNS first, then the well-known endpoint, as the spec prefers.
  if not handle.isValidHandle:
    raise newException(IdentityError, "not a valid handle: " & handle)
  let viaDns = resolveHandleDns(handle)
  if viaDns.isSome:
    return viaDns.get
  let viaHttp = resolveHandleHttp(handle)
  if viaHttp.isSome:
    return viaHttp.get
  raise newException(IdentityError, "could not resolve handle: " & handle)

proc resolveDidDoc*(did: string, plcUrl = DefaultPlcUrl): JsonNode =
  if not did.isDid:
    raise newException(IdentityError, "unsupported DID method: " & did)

  let url =
    if did.startsWith("did:plc:"):
      plcUrl & "/" & did
    else:
      # did:web:example.com -> https://example.com/.well-known/did.json
      let host = did["did:web:".len .. ^1].replace("%3A", ":")
      "https://" & host & "/.well-known/did.json"

  let client = newClient()
  defer: client.close()
  let res = client.get(url)
  if res.code.int != 200:
    raise newException(IdentityError,
      "DID document lookup failed (HTTP " & $res.code.int & "): " & did)
  try:
    result = parseJson(res.body)
  except JsonParsingError:
    raise newException(IdentityError, "DID document is not valid JSON: " & did)

proc pdsEndpoint*(doc: JsonNode): string =
  ## The service entry whose id ends in "#atproto_pds".
  if doc.kind != JObject or "service" notin doc:
    return ""
  for svc in doc["service"]:
    let id = svc{"id"}.getStr
    if id.endsWith("#atproto_pds"):
      return svc{"serviceEndpoint"}.getStr.strip(chars = {'/'})
  ""

proc handleFromDoc(doc: JsonNode): string =
  for aka in doc{"alsoKnownAs"}:
    let v = aka.getStr
    if v.startsWith("at://"):
      return v["at://".len .. ^1]
  ""

proc lookup*(identifier: string, plcUrl = DefaultPlcUrl): Identity =
  ## Resolve a handle or DID all the way to a PDS endpoint.
  let did = if identifier.isDid: identifier else: resolveHandle(identifier)
  let doc = resolveDidDoc(did, plcUrl)
  result = Identity(
    did: did,
    handle: if identifier.isDid: handleFromDoc(doc) else: identifier,
    pdsEndpoint: pdsEndpoint(doc),
  )
  if result.pdsEndpoint.len == 0:
    raise newException(IdentityError, "identity has no atproto PDS: " & did)

proc hostOf*(url: string): string =
  let u = parseUri(url)
  if u.port.len > 0: u.hostname & ":" & u.port else: u.hostname
