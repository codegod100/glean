## Keeping a Jetstream subscription alive.
##
## `jetstream.nim` reads a stream; this survives it ending. Three concerns,
## all of which are about not losing events rather than about uptime:
##
## 1. **Resume from a cursor, rewound.** Cursors are persisted in batches, so
##    the last one written is always slightly behind the last one *seen*. On
##    reconnect the consumer rewinds past that gap and accepts re-delivery --
##    duplicate events are cheap when handling is idempotent, and a hole in
##    the record is not recoverable.
##
## 2. **Rotate connections on a timer.** Server-side filters are fixed for the
##    life of a connection, so a subscription that never reconnects never
##    learns about newly-known DIDs. Glean's Go consumer rotates every 15
##    minutes for exactly this reason.
##
## 3. **Back off, with jitter.** The Go consumer waits a flat 5s between
##    attempts, which is fine against one healthy server and unkind to one
##    that is down. This grows the delay to a cap, and jitters it so a fleet
##    of consumers does not reconnect in lockstep.

import std/[math, random]
import pkg/chronicles
import pkg/chronos
import pkg/websock/websock
import ./jetstream

type
  BackoffConfig* = object
    initial*: Duration
    maxDelay*: Duration
    factor*: float
    jitter*: float      ## 0.0-1.0 fraction of the delay to randomise

  SupervisorConfig* = object
    jetstream*: JetstreamConfig
    backoff*: BackoffConfig
    rewind*: Duration
      ## How far behind the persisted cursor to resume. Covers the window
      ## between the last cursor flush and the disconnection.
    rotateEvery*: Duration
      ## Force a reconnect this often so filter changes take effect. Zero
      ## disables rotation.

  CursorStore* = ref object of RootObj ## Override to persist across restarts.
    cursor*: int64

  Supervisor* = ref object
    cfg*: SupervisorConfig
    store*: CursorStore
    handler*: EventHandler
    running*: bool
    reconnects*: int    ## observable for tests and metrics

# Both are `raises: []` on purpose. Saving a cursor happens on the event path,
# and a persistent store that throws -- a locked database, a full disk -- must
# not tear down the subscription. An implementation that can fail is expected
# to swallow and log, and to lose at most the rewind window.
method load*(s: CursorStore): int64 {.base, gcsafe, raises: [].} =
  s.cursor
method save*(s: CursorStore, cursor: int64) {.base, gcsafe, raises: [].} =
  s.cursor = cursor

const
  DefaultBackoff* = BackoffConfig(
    initial: 1.seconds, maxDelay: 60.seconds, factor: 2.0, jitter: 0.2)
  DefaultRewind* = 5.seconds
  DefaultRotateEvery* = 15.minutes

proc newSupervisorConfig*(js: JetstreamConfig): SupervisorConfig =
  SupervisorConfig(
    jetstream: js,
    backoff: DefaultBackoff,
    rewind: DefaultRewind,
    rotateEvery: DefaultRotateEvery,
  )

proc delayFor*(b: BackoffConfig, attempt: int, rand01: float = -1.0): Duration =
  ## Delay before attempt `attempt` (0 = first retry).
  ##
  ## `rand01` is injectable so the jitter can be tested deterministically;
  ## left at -1 it draws from the global RNG.
  ##
  ## The first retry is jittered like every other. Short-circuiting it to the
  ## bare initial delay would leave the one moment a herd is most likely --
  ## every consumer dropped at once by the same server restart -- as the one
  ## moment they all wake together.
  let
    exp = max(attempt, 0).float
    grown = b.initial.milliseconds.float * pow(b.factor, exp)
    capped = min(grown, b.maxDelay.milliseconds.float)
  if b.jitter <= 0:
    return milliseconds(capped.int64)
  # Jitter downward only, so the cap stays a real ceiling.
  let r = if rand01 >= 0: rand01 else: rand(1.0)
  let jittered = capped * (1.0 - b.jitter * r)
  milliseconds(max(jittered.int64, 1'i64))

proc rewoundCursor*(store: CursorStore, rewind: Duration): int64 =
  ## The persisted cursor, moved back to cover the unflushed window. Zero
  ## (start live) stays zero -- there is nothing to rewind to.
  let c = store.load()
  if c <= 0: return 0
  max(c - rewind.microseconds, 0'i64)

proc newSupervisor*(cfg: SupervisorConfig, handler: EventHandler,
                    store: CursorStore = nil): Supervisor =
  Supervisor(
    cfg: cfg,
    store: if store != nil: store else: CursorStore(),
    handler: handler,
  )

proc runOnce(sup: Supervisor) {.async.} =
  ## One connection, read until it ends or the rotation timer fires.
  var cfg = sup.cfg.jetstream
  cfg.cursor = rewoundCursor(sup.store, sup.cfg.rewind)

  let ws = await connect(cfg)
  defer:
    try: await ws.close()
    except CatchableError: discard

  # Record every event's cursor, so a drop resumes near where it stopped.
  let store = sup.store
  let inner = sup.handler
  proc onEvent(ev: Event) {.gcsafe, raises: [].} =
    if ev.timeUs > 0:
      store.save(ev.timeUs)
    inner(ev)

  let pump = readEvents(ws, onEvent)
  if sup.cfg.rotateEvery > ZeroDuration:
    if not await pump.withTimeout(sup.cfg.rotateEvery):
      # Rotation is a normal end to a connection, not a failure.
      await pump.cancelAndWait()
  else:
    await pump

proc run*(sup: Supervisor) {.async.} =
  ## Reconnect forever. Stop by setting `running` to false.
  sup.running = true
  var attempt = 0
  while sup.running:
    try:
      await sup.runOnce()
      # A clean end (rotation, or the server closing) is not a failure, so
      # the next attempt starts from the bottom of the backoff curve.
      attempt = 0
    except CancelledError as e:
      raise e
    except CatchableError as e:
      inc sup.reconnects
      warn "jetstream connection lost", err = e.msg, attempt = attempt
      let delay = delayFor(sup.cfg.backoff, attempt)
      inc attempt
      await sleepAsync(delay)
      continue

    if not sup.running:
      break
    # Even a clean rotation waits a moment, so a server that closes
    # immediately cannot spin this loop.
    await sleepAsync(sup.cfg.backoff.initial)

proc stop*(sup: Supervisor) =
  sup.running = false
