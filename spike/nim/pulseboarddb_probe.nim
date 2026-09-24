## Checks the three-database layout, the schema, and the behaviour that
## depends on both.
##
##   nim c -r -d:ssl pulseboarddb_probe.nim
##
## Most of this is about things that fail quietly. An ATTACH that did not
## happen, a pragma that did not take, an FTS index whose triggers never fire
## -- each leaves a database that works for the first query anyone tries and
## degrades or diverges later.

import std/[options, os, strformat, strutils]
import reader/[pulseboarddb, schema, sqlite]

var failures = 0

proc report(name: string, ok: bool, detail = "") =
  if not ok: inc failures
  let label = if ok: "PASS" else: "FAIL"
  if detail.len > 0: echo &"  {label}  {name}  -- {detail}"
  else: echo &"  {label}  {name}"

proc cleanup(base: string) =
  for suffix in ["_users", "_articles"]:
    for ext in ["", "-wal", "-shm"]:
      removeFile(base & suffix & ext)

proc main() =
  echo "Pulseboard database layer probe"
  echo ""

  let base = getTempDir() / "pulseboard_db_probe"
  cleanup(base)
  defer: cleanup(base)

  var g = pulseboarddb.open(base)
  defer: g.close()

  report("the schema covers the reader's tables", readerSchema.len == 17,
         &"{readerSchema.len} statements")

  # --- the three databases ------------------------------------------------
  block:
    let schemas = g.attachedSchemas()
    report("both databases are attached",
           "main" in schemas and "articles" in schemas, schemas.join(", "))
    report("each is a separate file on disk",
           fileExists(base & "_users") and fileExists(base & "_articles"))

  # --- pragmas ------------------------------------------------------------
  # WAL has to be set per schema: PRAGMA applies to one at a time, so setting
  # it on main leaves the others journalling the slow way.
  block:
    for name in ["", "articles"]:
      let mode = g.journalMode(name).toLowerAscii
      report(&"WAL is on for {(if name.len == 0: \"main\" else: name)}",
             mode == "wal", mode)

  # --- schema -------------------------------------------------------------
  g.migrate()
  report("migrate() is idempotent", (g.migrate(); true))

  block:
    proc tableCount(schemaName: string): int =
      let n = g.db.queryInt(
        &"SELECT count(*) FROM {schemaName}.sqlite_master WHERE type='table'")
      n.get(0).int
    # main holds identities and short-lived web sessions; everything that
    # grows with feed contents lives in the articles database.
    report("the users database holds users and web sessions",
           tableCount("main") == 2, $tableCount("main"))
    report("articles holds feeds, subscriptions, articles and read state",
           tableCount("articles") >= 5, $tableCount("articles"))

  # --- a real cross-database query ---------------------------------------
  # The whole point of attaching rather than opening three connections.
  block:
    # `users` is keyed on the DID alone -- handles are resolved from atproto
    # rather than stored, so there is no handle column to join on.
    g.db.run("INSERT INTO users (did) VALUES (?)", p("did:plc:probe"))
    g.db.run("""INSERT INTO articles.feeds (feed_url, title)
                VALUES (?, ?)""", p("https://ex.test/feed"), p("Example"))
    g.db.run("""INSERT INTO articles.subscriptions (user_did, feed_url)
                VALUES (?, ?)""", p("did:plc:probe"), p("https://ex.test/feed"))

    let joined = g.db.queryText("""
      SELECT f.title FROM articles.subscriptions s
      JOIN users u ON u.did = s.user_did
      JOIN articles.feeds f ON f.feed_url = s.feed_url
      WHERE u.did = ?""", p("did:plc:probe"))
    report("a query joins across attached databases",
           joined == some("Example"), joined.get("nothing"))

  # --- FTS5 ---------------------------------------------------------------
  # The index is kept current by triggers, so inserting into `articles` alone
  # must make the row findable. If the triggers were missed, search silently
  # returns nothing while everything else looks fine.
  block:
    g.db.run("""INSERT INTO articles.articles
                (feed_url, guid, title, summary, content, author)
                VALUES (?, ?, ?, ?, ?, ?)""",
             p("https://ex.test/feed"), p("g1"),
             p("Nim systems programming"), p("a summary"),
             p("the body text"), p("An Author"))

    let hit = g.db.queryText(
      "SELECT title FROM articles.articles_fts WHERE articles_fts MATCH ?",
      p("systems"))
    report("FTS5 triggers index new rows", hit.isSome, hit.get("no match"))

    g.db.run("UPDATE articles.articles SET title = ? WHERE guid = ?",
             p("Rust systems programming"), p("g1"))
    let updated = g.db.queryText(
      "SELECT title FROM articles.articles_fts WHERE articles_fts MATCH ?",
      p("Rust"))
    report("FTS5 triggers follow updates", updated.isSome, updated.get("no match"))

    g.db.run("DELETE FROM articles.articles WHERE guid = ?", p("g1"))
    let afterDelete = g.db.queryText(
      "SELECT title FROM articles.articles_fts WHERE articles_fts MATCH ?",
      p("systems"))
    # The detail is only interesting when this fails, and printing the
    # fallback on success reads like the opposite of what happened.
    report("FTS5 triggers follow deletes", afterDelete.isNone,
           if afterDelete.isNone: "row gone from the index"
           else: "STILL INDEXED: " & afterDelete.get)

  echo ""
  if failures == 0:
    echo "RESULT: the database layer matches Pulseboard's layout and behaviour."
  else:
    echo &"RESULT: {failures} check(s) failed."
    quit 1

when isMainModule:
  main()
