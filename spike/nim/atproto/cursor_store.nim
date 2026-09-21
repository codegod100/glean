## A Jetstream cursor store backed by SQLite, with batched writes.
##
## Bridges `store` and `jetstream_run`. Separate from both so the supervisor
## does not drag a database dependency behind it, and so an application can
## substitute its own persistence.
##
## Writes are batched because the cursor advances on *every* event -- often
## hundreds a second -- and a synchronous write per event would make the
## database the bottleneck for a firehose. The cost is that the stored cursor
## trails the live one, which is exactly the gap the supervisor's rewind
## exists to cover.

import std/times
import ./jetstream_run
import ./sqlite
import ./store

type DbCursorStore* = ref object of CursorStore
  db*: Db
  flushEvery*: int64    ## seconds
  lastFlush: int64
  pending: int64
  dirty: bool

proc newDbCursorStore*(db: Db, flushEvery = 5'i64): DbCursorStore =
  result = DbCursorStore(db: db, flushEvery: flushEvery,
                         lastFlush: getTime().toUnix)
  result.cursor = loadCursor(db)
  result.pending = result.cursor

method load*(s: DbCursorStore): int64 {.gcsafe, raises: [].} =
  s.cursor

proc flushNow*(s: DbCursorStore) =
  ## Write the newest cursor out. Safe to call at any time; a failure is
  ## swallowed, because losing a cursor update costs a replay and raising
  ## here would cost the subscription.
  if not s.dirty: return
  try:
    saveCursor(s.db, s.pending)
    s.cursor = s.pending
    s.dirty = false
    s.lastFlush = getTime().toUnix
  except CatchableError:
    discard

method save*(s: DbCursorStore, cursor: int64) {.gcsafe, raises: [].} =
  ## Called per event. Records in memory and writes at most once per
  ## flushEvery seconds.
  if cursor <= s.pending: return
  s.pending = cursor
  s.dirty = true
  let now = try: getTime().toUnix except CatchableError: return
  if now - s.lastFlush >= s.flushEvery:
    s.flushNow()

proc close*(s: DbCursorStore) =
  ## Final flush, so a clean shutdown does not throw away the last batch.
  s.flushNow()
