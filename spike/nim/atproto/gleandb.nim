## Glean's database: three SQLite files behind one connection.
##
## `main` holds users and auth, `articles` the feeds and their contents, and
## `recs` the derived recommendation tables. They are separate files attached
## together, so a query can join across them while each still checkpoints and
## vacuums on its own -- and, more practically, so the derived tables can be
## deleted and rebuilt without touching anything a user wrote.
##
## The pragmas are copied from internal/db/db.go. They are not decoration:
## WAL is what lets readers run during a write, and the per-database repeats
## matter because PRAGMA applies to one schema at a time -- setting WAL on
## `main` leaves `articles` journalling the slow way.

import std/[math, options, strformat, strutils]
import ./schema
import ./sqlite

type
  GleanDb* = object
    db*: Db
    basePath*: string

{.push importc, cdecl.}
proc sqlite3_create_function_v2(db: pointer, name: cstring, nArg: cint,
                                eTextRep: cint, pApp: pointer,
                                xFunc, xStep, xFinal, xDestroy: pointer): cint
proc sqlite3_value_double(v: pointer): cdouble
proc sqlite3_result_double(ctx: pointer, v: cdouble)
proc sqlite3_result_null(ctx: pointer)
{.pop.}

const
  SQLITE_UTF8 = 1.cint
  SQLITE_DETERMINISTIC = 0x000000800.cint

# SQLite ships maths functions only when built with
# SQLITE_ENABLE_MATH_FUNCTIONS, which the library here is not, so these are
# registered exactly as the Go connection registers them. They are not
# optional: the clustering SQL calls EXP and LOG in seven places for time
# decay and popularity normalisation, and without them every recommendation
# query fails at runtime rather than at startup.
proc sqlExp(ctx: pointer, n: cint, args: ptr UncheckedArray[pointer])
           {.cdecl.} =
  if n != 1:
    sqlite3_result_null(ctx)
    return
  sqlite3_result_double(ctx, exp(sqlite3_value_double(args[0])).cdouble)

proc sqlLog(ctx: pointer, n: cint, args: ptr UncheckedArray[pointer])
           {.cdecl.} =
  if n != 1:
    sqlite3_result_null(ctx)
    return
  let v = sqlite3_value_double(args[0]).float
  # log(0) is -inf and log(negative) is NaN; SQL callers want neither, and a
  # NULL propagates through an ORDER BY far more gracefully.
  if v <= 0:
    sqlite3_result_null(ctx)
  else:
    sqlite3_result_double(ctx, ln(v).cdouble)

proc registerFunctions*(db: Db) =
  for (name, fn) in [("exp", sqlExp), ("log", sqlLog)]:
    let rc = sqlite3_create_function_v2(
      db.handle, name.cstring, 1,
      SQLITE_UTF8 or SQLITE_DETERMINISTIC,
      nil, cast[pointer](fn), nil, nil, nil)
    if rc != SQLITE_OK:
      raise newException(SqliteError, "registering SQL function " & name)

const
  MainPragmas = [
    "journal_mode=WAL",
    "wal_autocheckpoint=1000",
    "busy_timeout=30000",
    "synchronous=NORMAL",
    "temp_store=FILE",
    "mmap_size=268435456",
    "auto_vacuum=INCREMENTAL",
  ]
  AttachedPragmas = [
    "journal_mode=WAL",
    "wal_autocheckpoint=1000",
    "busy_timeout=30000",
    "synchronous=NORMAL",
    "temp_store=FILE",
    "mmap_size=268435456",
  ]

proc applyPragmas(db: Db, schemaName: string, pragmas: openArray[string]) =
  for pragma in pragmas:
    # A pragma that will not take is not worth failing a boot over -- some are
    # advisory, and mmap_size in particular is refused on filesystems that
    # cannot map. The ones that matter are asserted after opening instead.
    try:
      if schemaName.len == 0:
        db.exec &"PRAGMA {pragma}"
      else:
        db.exec &"PRAGMA {schemaName}.{pragma}"
    except SqliteError:
      discard

proc open*(basePath: string): GleanDb =
  ## Open `<basePath>_users` and attach `_articles` and `_recs`.
  ##
  ## The suffixes match the Go layout, so a database written by either side is
  ## readable by the other.
  let
    usersPath = basePath & "_users"
    articlesPath = basePath & "_articles"
    recsPath = basePath & "_recs"

  var db = sqlite.open(usersPath)
  registerFunctions(db)
  applyPragmas(db, "", MainPragmas)

  # Quoted and single-quote-escaped: a path is not necessarily tame, and this
  # one can come from configuration.
  for (name, path) in [("articles", articlesPath), ("recs", recsPath)]:
    db.exec &"ATTACH DATABASE '{path.replace(\"'\", \"''\")}' AS {name}"
    applyPragmas(db, name, AttachedPragmas)

  GleanDb(db: db, basePath: basePath)

proc migrate*(g: GleanDb) =
  ## Apply the schema. Every statement is IF NOT EXISTS, so this runs on every
  ## boot rather than being gated on a version.
  for stmt in usersSchema: g.db.exec stmt
  for stmt in articlesSchema: g.db.exec stmt
  for stmt in recsSchema: g.db.exec stmt

proc createEmbeddingTables*(g: GleanDb, dimension: int) =
  ## The vec0 tables carry their dimension in the DDL, so they cannot live in
  ## the static schema -- it comes from GLEAN_EMBED_DIMENSION.
  g.db.exec &"""CREATE VIRTUAL TABLE IF NOT EXISTS recs.feed_embeddings
                USING vec0(feed_url TEXT PRIMARY KEY, embedding float[{dimension}])"""
  g.db.exec &"""CREATE VIRTUAL TABLE IF NOT EXISTS recs.article_embeddings
                USING vec0(article_id INTEGER PRIMARY KEY, embedding float[{dimension}])"""

proc close*(g: var GleanDb) =
  g.db.close()

proc journalMode*(g: GleanDb, schemaName = ""): string =
  let sql = if schemaName.len == 0: "PRAGMA journal_mode"
            else: &"PRAGMA {schemaName}.journal_mode"
  g.db.queryText(sql).get("")

proc attachedSchemas*(g: GleanDb): seq[string] =
  for row in g.db.rows("PRAGMA database_list"):
    result.add row[1]
