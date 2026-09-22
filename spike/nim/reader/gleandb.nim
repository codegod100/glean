## The database: two SQLite files behind one connection.
##
## `main` holds the local user, `articles` the feeds and their contents. They
## are attached together so a query can join across them, while each still
## checkpoints and vacuums on its own -- which matters because the article
## store is the only one that grows without bound.
##
## The pragmas are copied from internal/db/db.go. They are not decoration:
## WAL is what lets readers run during a write, and the per-database repeats
## matter because PRAGMA applies to one schema at a time -- setting WAL on
## `main` leaves `articles` journalling the slow way.

import std/[options, strformat, strutils]
import ./schema
import ./sqlite

type
  GleanDb* = object
    db*: Db
    basePath*: string

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
  ## Open `<basePath>_users` and attach `_articles`.
  let
    usersPath = basePath & "_users"
    articlesPath = basePath & "_articles"

  var db = sqlite.open(usersPath)
  applyPragmas(db, "", MainPragmas)

  # Quoted and single-quote-escaped: a path is not necessarily tame, and this
  # one can come from configuration.
  for (name, path) in [("articles", articlesPath)]:
    db.exec &"ATTACH DATABASE '{path.replace(\"'\", \"''\")}' AS {name}"
    applyPragmas(db, name, AttachedPragmas)

  GleanDb(db: db, basePath: basePath)

proc migrate*(g: GleanDb) =
  ## Apply the schema. Every statement is IF NOT EXISTS, so this runs on every
  ## boot rather than being gated on a version.
  for stmt in readerSchema: g.db.exec stmt

proc close*(g: var GleanDb) =
  g.db.close()

proc journalMode*(g: GleanDb, schemaName = ""): string =
  let sql = if schemaName.len == 0: "PRAGMA journal_mode"
            else: &"PRAGMA {schemaName}.journal_mode"
  g.db.queryText(sql).get("")

proc attachedSchemas*(g: GleanDb): seq[string] =
  for row in g.db.rows("PRAGMA database_list"):
    result.add row[1]
