## Jetstream consumer: the ATProto firehose as JSON over a websocket.
##
## Jetstream is a public, unauthenticated feed of repository commits, so this
## is the one part of the port that can be verified end to end without a
## consented session.
##
## Filtering happens server-side via query parameters (wantedCollections,
## wantedDids), which matters: the unfiltered firehose is the entire network.
## A subscription that silently loses its filters still delivers events, so a
## consumer that looks healthy can be reading the wrong stream entirely --
## see the note on websock's Uri overload in `connect`.

import std/[json, strutils, uri]
import pkg/chronicles
import pkg/chronos
import pkg/websock/websock

type
  JetstreamError* = object of CatchableError

  EventKind* = enum
    ekCommit = "commit"
    ekIdentity = "identity"
    ekAccount = "account"
    ekUnknown = "unknown"

  CommitOperation* = enum
    coCreate = "create"
    coUpdate = "update"
    coDelete = "delete"

  Commit* = object
    rev*: string
    operation*: CommitOperation
    collection*: string
    rkey*: string
    cid*: string
    record*: JsonNode   ## nil for a delete

  Event* = object
    did*: string
    timeUs*: int64      ## microseconds; also the replay cursor
    kind*: EventKind
    commit*: Commit     ## meaningful only when kind is ekCommit
    raw*: JsonNode

  JetstreamConfig* = object
    url*: string
    wantedCollections*: seq[string]
    wantedDids*: seq[string]
    cursor*: int64      ## 0 to start live
    userAgent*: string

  EventHandler* = proc(ev: Event) {.gcsafe, raises: [].}

const
  DefaultJetstream* = "wss://jetstream1.us-east.bsky.network"
  DefaultUserAgent* = "glean-nim/0.1"

proc bytesToString(bs: openArray[byte]): string =
  ## websock hands back seq[byte]; Jetstream frames are UTF-8 JSON text.
  result = newString(bs.len)
  if bs.len > 0:
    copyMem(result[0].addr, bs[0].unsafeAddr, bs.len)

proc parseEventKind(s: string): EventKind =
  case s
  of "commit": ekCommit
  of "identity": ekIdentity
  of "account": ekAccount
  else: ekUnknown

proc parseOperation(s: string): CommitOperation =
  case s
  of "create": coCreate
  of "update": coUpdate
  of "delete": coDelete
  else: coCreate

proc parseEvent*(raw: string): Event =
  ## Decode one Jetstream frame.
  let j =
    try: parseJson(raw)
    except CatchableError:
      raise newException(JetstreamError, "frame is not valid JSON")

  result = Event(
    did: j{"did"}.getStr,
    timeUs: j{"time_us"}.getBiggestInt.int64,
    kind: parseEventKind(j{"kind"}.getStr),
    raw: j,
  )

  if result.kind == ekCommit and j.hasKey("commit"):
    let c = j["commit"]
    result.commit = Commit(
      rev: c{"rev"}.getStr,
      operation: parseOperation(c{"operation"}.getStr),
      collection: c{"collection"}.getStr,
      rkey: c{"rkey"}.getStr,
      cid: c{"cid"}.getStr,
      record: c{"record"},
    )

proc subscribeUrl*(cfg: JetstreamConfig): string =
  ## Build the /subscribe URL, including filters.
  var base = cfg.url.strip(chars = {'/'})
  if not base.endsWith("/subscribe"):
    base &= "/subscribe"

  var params: seq[(string, string)]
  # Repeated keys, not a comma-joined list: Jetstream reads these as a
  # multi-valued parameter.
  for c in cfg.wantedCollections:
    params.add ("wantedCollections", c)
  for d in cfg.wantedDids:
    params.add ("wantedDids", d)
  if cfg.cursor > 0:
    params.add ("cursor", $cfg.cursor)

  if params.len > 0: base & "?" & params.encodeQuery else: base

proc connect*(cfg: JetstreamConfig): Future[WSSession] {.async.} =
  ## Open the websocket.
  ##
  ## Deliberately not websock's `Uri` overload: it forwards only `uri.path`
  ## and drops `uri.query`, which for Jetstream means losing every filter and
  ## silently subscribing to the entire network firehose instead of the
  ## handful of collections asked for.
  let full = subscribeUrl(cfg)
  let u = parseUri(full)
  if u.scheme notin ["ws", "wss"]:
    raise newException(JetstreamError, "not a websocket URL: " & full)

  let
    secure = u.scheme == "wss"
    port = if u.port.len > 0: u.port else: (if secure: "443" else: "80")
    pathAndQuery = (if u.path.len > 0: u.path else: "/") &
                   (if u.query.len > 0: "?" & u.query else: "")

  await WebSocket.connect(
    host = u.hostname & ":" & port,
    path = pathAndQuery,
    hostName = u.hostname,
    secure = secure,
  )

proc readEvents*(ws: WSSession, handler: EventHandler,
                 maxEvents = 0) {.async.} =
  ## Pump frames until the socket closes, or until `maxEvents` have been
  ## handled when that is non-zero.
  var seen = 0
  while ws.readyState != ReadyState.Closed:
    let frame = await ws.recvMsg()
    if frame.len == 0:
      # A zero-length read means the peer closed; recvMsg does not raise.
      break
    var ev: Event
    try:
      ev = parseEvent(bytesToString(frame))
    except JetstreamError:
      # One malformed frame should not end a long-lived subscription.
      continue
    handler(ev)
    inc seen
    if maxEvents > 0 and seen >= maxEvents:
      break

proc collectEvents*(cfg: JetstreamConfig, count: int,
                    timeout = 30.seconds): Future[seq[Event]] {.async.} =
  ## Connect, gather `count` events, and close. For probes and tests.
  var got: seq[Event]
  let ws = await connect(cfg)
  defer:
    try: await ws.close()
    except CatchableError: discard

  proc onEvent(ev: Event) {.gcsafe, raises: [].} =
    got.add ev

  let pump = readEvents(ws, onEvent, maxEvents = count)
  if not await pump.withTimeout(timeout):
    await pump.cancelAndWait()
  got
