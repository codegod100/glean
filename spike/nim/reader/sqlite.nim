## A small SQLite wrapper.
##
## Deliberately thin: enough to prepare, bind and step, not an ORM. FTS5 is
## used for article search and ships with the system library; nothing here
## needs an extension.

import std/options

type
  SqliteError* = object of CatchableError

  Db* = object
    handle*: pointer

  Stmt* = object
    handle*: pointer
    db*: pointer

const
  SQLITE_OK* = 0
  SQLITE_ROW* = 100
  SQLITE_DONE* = 101

{.push importc, cdecl.}
proc sqlite3_open(filename: cstring, db: var pointer): cint
proc sqlite3_close(db: pointer): cint
proc sqlite3_errmsg(db: pointer): cstring
proc sqlite3_exec(db: pointer, sql: cstring, cb, arg: pointer,
                  errmsg: ptr cstring): cint
proc sqlite3_prepare_v2(db: pointer, sql: cstring, nByte: cint,
                        stmt: var pointer, tail: ptr cstring): cint
proc sqlite3_step(s: pointer): cint
proc sqlite3_finalize(s: pointer): cint
proc sqlite3_reset(s: pointer): cint
proc sqlite3_column_text(s: pointer, col: cint): cstring
proc sqlite3_column_int64(s: pointer, col: cint): int64
proc sqlite3_column_double(s: pointer, col: cint): cdouble
proc sqlite3_column_count(s: pointer): cint
proc sqlite3_bind_text(s: pointer, col: cint, data: cstring, n: cint,
                       destructor: pointer): cint
proc sqlite3_bind_int64(s: pointer, col: cint, v: int64): cint
proc sqlite3_bind_blob(s: pointer, col: cint, data: pointer, n: cint,
                       destructor: pointer): cint
proc sqlite3_libversion(): cstring
proc sqlite3_column_type(s: pointer, col: cint): cint
proc sqlite3_last_insert_rowid(db: pointer): int64
proc sqlite3_changes(db: pointer): cint
proc sqlite3_clear_bindings(s: pointer): cint
{.pop.}


const SQLITE_TRANSIENT = cast[pointer](-1)

proc libVersion*(): string = $sqlite3_libversion()

proc check(db: pointer, rc: cint, what: string) =
  if rc notin [SQLITE_OK.cint, SQLITE_ROW.cint, SQLITE_DONE.cint]:
    raise newException(SqliteError, what & ": " & $sqlite3_errmsg(db))

proc open*(path: string): Db =
  ## Open (or create) a database.
  ##
  ## WAL is set here rather than left to callers: it is what makes a reader
  ## and the writer coexist, and a store that forgets it fails only under
  ## concurrency, which is the worst time to find out.
  var handle: pointer
  if sqlite3_open(path.cstring, handle) != SQLITE_OK:
    let msg = $sqlite3_errmsg(handle)
    discard sqlite3_close(handle)
    raise newException(SqliteError, "opening " & path & ": " & msg)
  result = Db(handle: handle)


  discard sqlite3_exec(handle, "PRAGMA journal_mode=WAL", nil, nil, nil)
  discard sqlite3_exec(handle, "PRAGMA synchronous=NORMAL", nil, nil, nil)
  discard sqlite3_exec(handle, "PRAGMA foreign_keys=ON", nil, nil, nil)
  discard sqlite3_exec(handle, "PRAGMA busy_timeout=5000", nil, nil, nil)

proc close*(db: var Db) =
  if db.handle != nil:
    discard sqlite3_close(db.handle)
    db.handle = nil

proc exec*(db: Db, sql: string) =
  var err: cstring
  let rc = sqlite3_exec(db.handle, sql.cstring, nil, nil, addr err)
  if rc != SQLITE_OK:
    raise newException(SqliteError, "exec failed: " & $sqlite3_errmsg(db.handle))

type Param* = object
  case isNull*: bool
  of true: discard
  of false:
    case kind*: range[0 .. 2]
    of 0: s*: string
    of 1: i*: int64
    of 2: b*: seq[byte]

proc p*(s: string): Param = Param(isNull: false, kind: 0, s: s)
proc p*(i: int64): Param = Param(isNull: false, kind: 1, i: i)
proc p*(b: seq[byte]): Param = Param(isNull: false, kind: 2, b: b)

proc prepare(db: Db, sql: string, params: openArray[Param]): pointer =
  var stmt: pointer
  check(db.handle, sqlite3_prepare_v2(db.handle, sql.cstring, -1, stmt, nil),
        "prepare")
  for i, prm in params:
    let col = (i + 1).cint
    let rc =
      if prm.isNull: SQLITE_OK.cint
      else:
        case prm.kind
        of 0: sqlite3_bind_text(stmt, col, prm.s.cstring, prm.s.len.cint,
                                SQLITE_TRANSIENT)
        of 1: sqlite3_bind_int64(stmt, col, prm.i)
        of 2:
          if prm.b.len == 0:
            sqlite3_bind_blob(stmt, col, nil, 0, SQLITE_TRANSIENT)
          else:
            sqlite3_bind_blob(stmt, col, prm.b[0].unsafeAddr, prm.b.len.cint,
                              SQLITE_TRANSIENT)
    if rc != SQLITE_OK:
      discard sqlite3_finalize(stmt)
      raise newException(SqliteError, "binding parameter " & $col)
  stmt

proc run*(db: Db, sql: string, params: varargs[Param]) =
  ## Execute a statement that returns no rows.
  let stmt = prepare(db, sql, params)
  defer: discard sqlite3_finalize(stmt)
  check(db.handle, sqlite3_step(stmt), "step")

proc queryText*(db: Db, sql: string, params: varargs[Param]): Option[string] =
  ## First column of the first row, as text.
  let stmt = prepare(db, sql, params)
  defer: discard sqlite3_finalize(stmt)
  if sqlite3_step(stmt) != SQLITE_ROW:
    return none(string)
  let raw = sqlite3_column_text(stmt, 0)
  if raw == nil: none(string) else: some($raw)

proc queryInt*(db: Db, sql: string, params: varargs[Param]): Option[int64] =
  let stmt = prepare(db, sql, params)
  defer: discard sqlite3_finalize(stmt)
  if sqlite3_step(stmt) != SQLITE_ROW:
    return none(int64)
  some(sqlite3_column_int64(stmt, 0))

iterator rows*(db: Db, sql: string, params: varargs[Param]): seq[string] =
  ## Every row, all columns as text. Fine for the small result sets a session
  ## store deals in; not for scanning a table of articles.
  let stmt = prepare(db, sql, @params)
  defer: discard sqlite3_finalize(stmt)
  let n = sqlite3_column_count(stmt)
  while sqlite3_step(stmt) == SQLITE_ROW:
    var row = newSeq[string](n)
    for i in 0 ..< n:
      let raw = sqlite3_column_text(stmt, i.cint)
      row[i] = if raw == nil: "" else: $raw
    yield row


# --- typed row access ------------------------------------------------------

const SQLITE_NULL* = 5

type Row* = object
  ## A cursor positioned on one result row. Valid only until the next step.
  stmt: pointer

proc isNull*(r: Row, col: int): bool =
  sqlite3_column_type(r.stmt, col.cint) == SQLITE_NULL

proc str*(r: Row, col: int): string =
  ## NULL reads as "". Most of Glean's text columns are nullable but mean the
  ## empty string, and the Go side collapses them the same way at the wire
  ## boundary (see nullStr in internal/server/api.go).
  if r.isNull(col): return ""
  let raw = sqlite3_column_text(r.stmt, col.cint)
  if raw == nil: "" else: $raw

proc strOpt*(r: Row, col: int): Option[string] =
  ## For the few places where NULL and "" genuinely differ.
  if r.isNull(col): none(string) else: some(r.str(col))

proc i64*(r: Row, col: int): int64 =
  if r.isNull(col): 0'i64 else: sqlite3_column_int64(r.stmt, col.cint)

proc i*(r: Row, col: int): int = r.i64(col).int

proc f*(r: Row, col: int): float =
  if r.isNull(col): 0.0 else: sqlite3_column_double(r.stmt, col.cint).float

proc b*(r: Row, col: int): bool = r.i64(col) != 0

iterator query*(db: Db, sql: string, params: varargs[Param]): Row =
  ## Typed row iteration. The Row borrows the statement, so copy anything you
  ## need out of it before the next iteration.
  let stmt = prepare(db, sql, @params)
  defer: discard sqlite3_finalize(stmt)
  while sqlite3_step(stmt) == SQLITE_ROW:
    yield Row(stmt: stmt)

proc queryFirst*[T](db: Db, sql: string, params: openArray[Param],
                    read: proc(r: Row): T): Option[T] =
  let stmt = prepare(db, sql, params)
  defer: discard sqlite3_finalize(stmt)
  if sqlite3_step(stmt) != SQLITE_ROW:
    return none(T)
  some(read(Row(stmt: stmt)))

proc queryAll*[T](db: Db, sql: string, params: openArray[Param],
                  read: proc(r: Row): T): seq[T] =
  let stmt = prepare(db, sql, params)
  defer: discard sqlite3_finalize(stmt)
  while sqlite3_step(stmt) == SQLITE_ROW:
    result.add read(Row(stmt: stmt))

proc lastInsertId*(db: Db): int64 = sqlite3_last_insert_rowid(db.handle)
proc changes*(db: Db): int = sqlite3_changes(db.handle).int

# --- transactions ----------------------------------------------------------

template transaction*(db: Db, body: untyped) =
  ## Run `body` in a transaction, rolling back if it raises.
  ##
  ## Batch ingest is the reason this exists: a few thousand inserts outside a
  ## transaction is a few thousand fsyncs, which turns a feed refresh into a
  ## minutes-long operation.
  db.exec "BEGIN IMMEDIATE"
  try:
    body
    db.exec "COMMIT"
  except CatchableError:
    try: db.exec "ROLLBACK"
    except CatchableError: discard
    raise

# --- reusable prepared statements ------------------------------------------

type Prepared* = object
  ## A statement prepared once and stepped many times. Worth it inside a
  ## batch; pointless outside one.
  stmt: pointer
  db: pointer

proc prepared*(db: Db, sql: string): Prepared =
  var stmt: pointer
  check(db.handle, sqlite3_prepare_v2(db.handle, sql.cstring, -1, stmt, nil),
        "prepare")
  Prepared(stmt: stmt, db: db.handle)

proc exec*(ps: Prepared, params: varargs[Param]) =
  discard sqlite3_reset(ps.stmt)
  discard sqlite3_clear_bindings(ps.stmt)
  for i, prm in params:
    let col = (i + 1).cint
    let rc =
      if prm.isNull: SQLITE_OK.cint
      else:
        case prm.kind
        of 0: sqlite3_bind_text(ps.stmt, col, prm.s.cstring, prm.s.len.cint,
                                SQLITE_TRANSIENT)
        of 1: sqlite3_bind_int64(ps.stmt, col, prm.i)
        of 2:
          if prm.b.len == 0:
            sqlite3_bind_blob(ps.stmt, col, nil, 0, SQLITE_TRANSIENT)
          else:
            sqlite3_bind_blob(ps.stmt, col, prm.b[0].unsafeAddr, prm.b.len.cint,
                              SQLITE_TRANSIENT)
    if rc != SQLITE_OK:
      raise newException(SqliteError, "binding parameter " & $col)
  check(ps.db, sqlite3_step(ps.stmt), "step")

proc finalize*(ps: var Prepared) =
  if ps.stmt != nil:
    discard sqlite3_finalize(ps.stmt)
    ps.stmt = nil

proc nullParam*(): Param = Param(isNull: true)

proc pOrNull*(s: string): Param =
  ## Empty text becomes NULL, matching the Go side's nilIfEmpty.
  if s.len == 0: nullParam() else: p(s)
