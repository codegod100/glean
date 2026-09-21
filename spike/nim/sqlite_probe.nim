## Spike: can Nim drive the two SQLite features Glean depends on?
##
## Glean's Go backend needs FTS5 (full-text search over articles) and
## sqlite-vec (embedding similarity for recommendations). Both are C-level
## concerns, so the question for a Nim port is not "does Nim have a SQLite
## library" but whether the same extension can be linked and registered the
## same way. The Go bindings compile sqlite-vec.c with -DSQLITE_CORE and call
## sqlite3_vec_init directly; this does exactly that from Nim.

{.compile: "vendor/sqlite-vec.c".}  # flags live in nim.cfg

import std/[strformat, strutils, sequtils]

type
  Sqlite3 = distinct pointer
  Stmt = distinct pointer

const
  SQLITE_OK = 0
  SQLITE_ROW = 100
  SQLITE_DONE = 101

{.push importc, cdecl.}
proc sqlite3_open(filename: cstring, db: var Sqlite3): cint
proc sqlite3_close(db: Sqlite3): cint
proc sqlite3_errmsg(db: Sqlite3): cstring
proc sqlite3_exec(db: Sqlite3, sql: cstring, cb, arg: pointer,
                  errmsg: ptr cstring): cint
proc sqlite3_prepare_v2(db: Sqlite3, sql: cstring, nByte: cint,
                        stmt: var Stmt, tail: ptr cstring): cint
proc sqlite3_step(s: Stmt): cint
proc sqlite3_finalize(s: Stmt): cint
proc sqlite3_column_text(s: Stmt, col: cint): cstring
proc sqlite3_column_double(s: Stmt, col: cint): cdouble
proc sqlite3_column_int64(s: Stmt, col: cint): int64
proc sqlite3_bind_blob(s: Stmt, col: cint, data: pointer, n: cint,
                       destructor: pointer): cint
proc sqlite3_libversion(): cstring
{.pop.}

# Provided by the compiled sqlite-vec.c. With SQLITE_CORE it is an ordinary
# function rather than a loadable-extension entry point.
proc sqlite3_vec_init(db: Sqlite3, errMsg: ptr cstring,
                      api: pointer): cint {.importc, cdecl.}

const SQLITE_TRANSIENT = cast[pointer](-1)

var failures = 0

proc check(db: Sqlite3, rc: cint, what: string) =
  if rc notin [SQLITE_OK, SQLITE_ROW, SQLITE_DONE]:
    raise newException(IOError, &"{what}: {sqlite3_errmsg(db)} (rc={rc})")

proc exec(db: Sqlite3, sql: string) =
  check(db, sqlite3_exec(db, sql.cstring, nil, nil, nil), "exec " & sql[0..min(40, sql.high)])

proc report(name: string, ok: bool, detail = "") =
  if ok:
    echo &"  PASS  {name}" & (if detail.len > 0: "  -- " & detail else: "")
  else:
    inc failures
    echo &"  FAIL  {name}" & (if detail.len > 0: "  -- " & detail else: "")

## Embeddings are bound as raw little-endian float32 blobs, the same wire
## format sqlite-vec expects from every binding.
proc toBlob(v: seq[float32]): string =
  result = newString(v.len * 4)
  if v.len > 0:
    copyMem(result[0].addr, v[0].unsafeAddr, result.len)

proc main() =
  echo "sqlite-vec / FTS5 spike"
  echo "  sqlite ", sqlite3_libversion()

  var db: Sqlite3
  check(db, sqlite3_open(":memory:", db), "open")
  defer: discard sqlite3_close(db)

  # --- sqlite-vec registration ---
  block:
    let rc = sqlite3_vec_init(db, nil, nil)
    report("sqlite3_vec_init registers", rc == SQLITE_OK, &"rc={rc}")

  block:
    var s: Stmt
    check(db, sqlite3_prepare_v2(db, "SELECT vec_version()", -1, s, nil), "prepare vec_version")
    let ok = sqlite3_step(s) == SQLITE_ROW
    let ver = if ok: $sqlite3_column_text(s, 0) else: ""
    discard sqlite3_finalize(s)
    report("vec_version() callable", ok, ver)

  # --- FTS5, as used for article search ---
  block:
    db.exec "CREATE VIRTUAL TABLE docs USING fts5(title, body)"
    db.exec "INSERT INTO docs VALUES ('Nim at scale', 'systems programming with a python face')"
    db.exec "INSERT INTO docs VALUES ('Go concurrency', 'goroutines and channels')"
    var s: Stmt
    check(db, sqlite3_prepare_v2(db,
      "SELECT title FROM docs WHERE docs MATCH 'systems' ORDER BY rank", -1, s, nil), "prepare fts")
    let ok = sqlite3_step(s) == SQLITE_ROW
    let title = if ok: $sqlite3_column_text(s, 0) else: ""
    discard sqlite3_finalize(s)
    report("FTS5 match + rank", ok and title == "Nim at scale", title)

  # --- vec0 virtual table: the recommendation path ---
  block:
    db.exec "CREATE VIRTUAL TABLE embeddings USING vec0(id INTEGER PRIMARY KEY, v float[4])"
    report("vec0 table created", true)

    # Three vectors; the query is nearest to id 2.
    let rows = {
      1'i64: @[1.0'f32, 0.0, 0.0, 0.0],
      2'i64: @[0.0'f32, 1.0, 0.0, 0.0],
      3'i64: @[0.0'f32, 0.0, 1.0, 0.0],
    }
    for (id, vec) in rows.items:
      var s: Stmt
      let sql = &"INSERT INTO embeddings(id, v) VALUES ({id}, ?)"
      check(db, sqlite3_prepare_v2(db, sql.cstring, -1, s, nil), "prepare insert")
      let blob = vec.toBlob
      check(db, sqlite3_bind_blob(s, 1, blob[0].unsafeAddr, blob.len.cint,
                                  SQLITE_TRANSIENT), "bind blob")
      check(db, sqlite3_step(s), "insert vec")
      discard sqlite3_finalize(s)
    report("float32 blobs bound", true, &"{rows.len} rows")

  block:
    let query = @[0.05'f32, 0.95, 0.0, 0.0]
    var s: Stmt
    check(db, sqlite3_prepare_v2(db,
      "SELECT id, distance FROM embeddings WHERE v MATCH ? AND k = 2 ORDER BY distance",
      -1, s, nil), "prepare knn")
    let blob = query.toBlob
    check(db, sqlite3_bind_blob(s, 1, blob[0].unsafeAddr, blob.len.cint,
                                SQLITE_TRANSIENT), "bind query")
    var got: seq[(int64, float)]
    while sqlite3_step(s) == SQLITE_ROW:
      got.add (sqlite3_column_int64(s, 0), sqlite3_column_double(s, 1).float)
    discard sqlite3_finalize(s)
    let nearest = if got.len > 0: got[0][0] else: -1
    report("KNN returns nearest first", got.len == 2 and nearest == 2,
           &"k=2 -> {got.mapIt($it[0]).join(\",\")}")

  echo ""
  if failures == 0:
    echo "RESULT: sqlite-vec + FTS5 work from Nim."
  else:
    echo &"RESULT: {failures} check(s) failed."
    quit 1

main()
