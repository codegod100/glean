## Checks the reconnect/backoff supervisor.
##
##   nim c -r -d:ssl reconnect_probe.nim
##
## The backoff curve and cursor arithmetic are pure and tested directly. The
## reconnect behaviour itself is tested against a local websocket server that
## is deliberately hostile -- it drops every connection after a few frames --
## because a consumer that "works" against a healthy server proves nothing
## about the case this module exists for.

import std/[json, sequtils, strformat, strutils]
import pkg/chronicles
import pkg/chronos
import pkg/websock/websock
import atproto/[jetstream, jetstream_run]

var failures = 0

proc report(name: string, ok: bool, detail = "") =
  if not ok: inc failures
  let label = if ok: "PASS" else: "FAIL"
  if detail.len > 0: echo &"  {label}  {name}  -- {detail}"
  else: echo &"  {label}  {name}"

# --- a server that hangs up ------------------------------------------------

type FlakyServer = ref object
  http: HttpServer
  connections*: int
  framesPerConn*: int
  cursorsSeen*: seq[string]   ## the cursor query param of each connection

proc start(framesPerConn: int): FlakyServer =
  let srv = FlakyServer(framesPerConn: framesPerConn)

  proc handle(request: HttpRequest) {.async, gcsafe.} =
    srv.connections.inc
    srv.cursorsSeen.add request.uri.query
    let server = WSServer.new(protos = ["", "jetstream"])
    let ws = await server.handleRequest(request)
    # Send a few well-formed events, then drop the connection mid-stream.
    for i in 0 ..< srv.framesPerConn:
      let ev = %*{
        "did": "did:plc:probe",
        "time_us": 1_700_000_000_000_000'i64 + srv.connections * 1000 + i,
        "kind": "commit",
        "commit": {"rev": "r", "operation": "create",
                   "collection": "app.test.item", "rkey": $i,
                   "record": {"n": i}},
      }
      await ws.send($ev)
    await ws.stream.closeWait()

  srv.http = HttpServer.create(initTAddress("127.0.0.1:0"), handle)
  srv.http.start()
  srv

proc port(s: FlakyServer): int = s.http.localAddress.port.int

# --- checks ----------------------------------------------------------------

proc main() {.async.} =
  echo "Jetstream reconnect probe"
  echo ""

  # --- backoff curve ------------------------------------------------------
  block:
    let b = BackoffConfig(initial: 1.seconds, maxDelay: 30.seconds,
                          factor: 2.0, jitter: 0.0)
    report("first retry uses the initial delay",
           delayFor(b, 0) == 1.seconds, $delayFor(b, 0).milliseconds & "ms")
    report("the first retry is jittered too (herd control)",
           delayFor(BackoffConfig(initial: 1.seconds, maxDelay: 10.seconds,
                                  factor: 2.0, jitter: 0.5),
                    0, rand01 = 1.0) == 500.milliseconds)
    report("delay grows geometrically",
           delayFor(b, 1) == 2.seconds and delayFor(b, 2) == 4.seconds,
           &"{delayFor(b, 1).milliseconds}ms, {delayFor(b, 2).milliseconds}ms")
    report("delay is capped",
           delayFor(b, 20) == 30.seconds, $delayFor(b, 20).milliseconds & "ms")

  block:
    # Jitter must reduce, never exceed, so the cap stays a real ceiling.
    let b = BackoffConfig(initial: 1.seconds, maxDelay: 10.seconds,
                          factor: 2.0, jitter: 0.5)
    let atMax = delayFor(b, 0, rand01 = 0.0)
    let atMin = delayFor(b, 0, rand01 = 1.0)
    report("jitter only shortens the delay",
           atMax == 1.seconds and atMin == 500.milliseconds,
           &"{atMin.milliseconds}..{atMax.milliseconds}ms")
    report("a jittered capped delay never exceeds the cap",
           delayFor(b, 20, rand01 = 0.0) <= 10.seconds)

  # --- cursor rewind ------------------------------------------------------
  block:
    let store = CursorStore(cursor: 1_700_000_000_000_000'i64)
    let c = rewoundCursor(store, 5.seconds)
    report("resume rewinds past the unflushed window",
           c == 1_700_000_000_000_000'i64 - 5_000_000,
           &"{store.cursor} -> {c}")

    # Starting live must stay live; rewinding zero would ask for 1970.
    report("no cursor stays live", rewoundCursor(CursorStore(), 5.seconds) == 0)

    let young = CursorStore(cursor: 1000)
    report("rewind clamps at zero", rewoundCursor(young, 5.seconds) == 0)

  # --- reconnect against a server that hangs up ---------------------------
  let server = start(framesPerConn = 3)
  var received: seq[Event]

  proc onEvent(ev: Event) {.gcsafe, raises: [].} =
    received.add ev

  var cfg = newSupervisorConfig(JetstreamConfig(
    url: &"ws://127.0.0.1:{server.port}",
    wantedCollections: @["app.test.item"],
  ))
  # Tight timings so the probe finishes; rotation off, so every reconnect
  # here is a genuine recovery from the server hanging up.
  cfg.backoff = BackoffConfig(initial: 50.milliseconds,
                              maxDelay: 200.milliseconds,
                              factor: 2.0, jitter: 0.1)
  cfg.rotateEvery = ZeroDuration
  cfg.rewind = 0.seconds

  let sup = newSupervisor(cfg, onEvent)
  let runner = sup.run()
  await sleepAsync(2.seconds)
  sup.stop()
  await runner.cancelAndWait()

  report("survived the server hanging up", server.connections > 1,
         &"{server.connections} connections")
  report("kept receiving across reconnects", received.len > 3,
         &"{received.len} events over {server.connections} connections")
  report("events arrived from more than one connection",
         received.len >= server.connections,
         &"{received.len} events")

  # The cursor must advance across connections, or a restart would replay
  # from the beginning forever.
  report("cursor advanced to the newest event seen",
         sup.store.load() == received[^1].timeUs,
         &"{sup.store.load()}")

  # And it must actually reach the server. Tracking a cursor that is never
  # sent looks identical from the client side and loses every event in the
  # gap on each reconnect.
  block:
    let withCursor = server.cursorsSeen.filterIt("cursor=" in it)
    report("reconnects send a cursor",
           withCursor.len > 0,
           &"{withCursor.len} of {server.cursorsSeen.len} connections")
    report("the first connection has no cursor (starts live)",
           server.cursorsSeen.len > 0 and "cursor=" notin server.cursorsSeen[0])
    if withCursor.len >= 2:
      proc cursorOf(q: string): int64 =
        for part in q.split('&'):
          if part.startsWith("cursor="):
            return try: parseBiggestInt(part[7 .. ^1]).int64
                   except ValueError: 0
        0
      report("the cursor advances between reconnects",
             cursorOf(withCursor[^1]) > cursorOf(withCursor[0]),
             &"{cursorOf(withCursor[0])} -> {cursorOf(withCursor[^1])}")

  # Stop accepting before closing: a connection still being torn down by the
  # supervisor would otherwise keep closeWait pending indefinitely.
  server.http.stop()
  if not await server.http.closeWait().withTimeout(5.seconds):
    report("server shut down cleanly", false, "closeWait timed out")

  echo ""
  if failures == 0:
    echo "RESULT: reconnect, backoff and cursor resume all work."
  else:
    echo &"RESULT: {failures} check(s) failed."
    quit 1

when isMainModule:
  waitFor main()
