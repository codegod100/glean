## A small SQLite wrapper, with sqlite-vec compiled in.
##
## Deliberately thin: enough to prepare, bind and step, not an ORM. The
## sqlite-vec extension is linked rather than loaded at runtime -- see
## nim.cfg, where the SQLITE_CORE define lives, and note that getting that
## wrong segfaults rather than erroring.

{.compile: "../vendor/sqlite-vec.c".}

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
{.pop.}

proc sqlite3_vec_init(db: pointer, errMsg: ptr cstring,
                      api: pointer): cint {.importc, cdecl.}

const SQLITE_TRANSIENT = cast[pointer](-1)

proc libVersion*(): string = $sqlite3_libversion()

proc check(db: pointer, rc: cint, what: string) =
  if rc notin [SQLITE_OK.cint, SQLITE_ROW.cint, SQLITE_DONE.cint]:
    raise newException(SqliteError, what & ": " & $sqlite3_errmsg(db))

proc open*(path: string, withVec = true): Db =
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

  if withVec:
    let rc = sqlite3_vec_init(handle, nil, nil)
    if rc != SQLITE_OK:
      raise newException(SqliteError, "registering sqlite-vec failed")

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
