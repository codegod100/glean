## Articles and read state, ported from internal/db/article.go.
##
## Read state is per-user and lives in its own table, so "the article list"
## is always a LEFT JOIN: an article with no read_state row is unread. That
## shape is why COALESCE(r.is_read, 0) appears everywhere rather than a
## boolean column on the article itself.

import std/[options, strutils, times]
import ./feedstore
import ./gleandb
import ./sqlite

type
  Article* = object
    id*: int64
    feedUrl*: string
    feedTitle*: string
    feedFaviconUrl*: string
    guid*: string
    title*: string
    url*: string
    author*: string
    summary*: string
    content*: string
    fullContent*: string
    published*: Option[DateTime]
    updated*: Option[DateTime]
    isRead*: bool

  ArticleFilter* = enum
    afAll = "all"
    afUnread = "unread"
    afRead = "read"

  NewArticle* = object ## What a feed parser produces, before it has an id.
    feedUrl*: string
    guid*: string
    title*: string
    url*: string
    author*: string
    summary*: string
    content*: string
    published*: Option[DateTime]
    updated*: Option[DateTime]

proc readArticleRow*(r: Row): Article =
  ## Exported so other stores returning the same column list can share it --
  ## socialstore's liked-articles listing, in particular. The positional
  ## reader is only safe while every caller selects `articleColumns`.
  Article(
    id: r.i64(0),
    feedUrl: r.str(1),
    feedTitle: r.str(2),
    feedFaviconUrl: r.str(3),
    guid: r.str(4),
    title: r.str(5),
    url: r.str(6),
    author: r.str(7),
    summary: r.str(8),
    content: r.str(9),
    fullContent: r.str(10),
    published: parseSqliteTime(r.str(11)),
    updated: parseSqliteTime(r.str(12)),
    isRead: r.b(13),
  )

## The column list is shared so every listing returns the same shape and
## readArticle can stay positional. A mismatch here reads as silently shifted
## fields rather than an error.
const articleColumns = """
  a.id, a.feed_url, COALESCE(f.title, a.feed_url), f.favicon_url,
  a.guid, a.title, a.url, a.author, a.summary, a.content, a.full_content,
  a.published, a.updated,
  COALESCE(r.is_read, 0)"""

const articleFrom = """
  FROM articles.articles a
  LEFT JOIN articles.feeds f ON f.feed_url = a.feed_url
  LEFT JOIN articles.read_state r ON r.article_id = a.id AND r.user_did = ?"""

proc getArticle*(g: GleanDb, userDid: string, id: int64): Option[Article] =
  g.db.queryFirst("SELECT " & articleColumns & articleFrom & " WHERE a.id = ?",
                  [p(userDid), p(id)], readArticleRow)

proc getArticleByUrl*(g: GleanDb, userDid, url: string): Option[Article] =
  g.db.queryFirst("SELECT " & articleColumns & articleFrom & " WHERE a.url = ?",
                  [p(userDid), p(url)], readArticleRow)

proc listArticles*(g: GleanDb, userDid: string, filter = afAll,
                   feedUrl = "", category = "", sortOldest = false,
                   limit = 25, offset = 0): seq[Article] =
  ## The main listing, scoped to what the user subscribes to.
  var sql = "SELECT " & articleColumns & articleFrom & """
    JOIN articles.subscriptions s
      ON s.feed_url = a.feed_url AND s.user_did = ?
    WHERE 1=1"""
  var params = @[p(userDid), p(userDid)]

  case filter
  of afUnread: sql &= " AND COALESCE(r.is_read, 0) = 0"
  of afRead: sql &= " AND COALESCE(r.is_read, 0) = 1"
  of afAll: discard

  if feedUrl.len > 0:
    sql &= " AND a.feed_url = ?"
    params.add p(feedUrl)
  if category.len > 0:
    sql &= " AND s.category = ?"
    params.add p(category)

  # NULL published sorts last either way: an article with no date is almost
  # always a parse failure, and burying it beats putting it at the top.
  sql &= (if sortOldest:
            " ORDER BY a.published IS NULL, a.published ASC, a.id ASC"
          else:
            " ORDER BY a.published IS NULL, a.published DESC, a.id DESC")
  sql &= " LIMIT ? OFFSET ?"
  params.add p(limit.int64)
  params.add p(offset.int64)

  g.db.queryAll(sql, params, readArticleRow)

proc searchArticles*(g: GleanDb, userDid, query: string,
                     limit = 25, offset = 0): seq[Article] =
  ## Full-text search over the user's subscriptions.
  ##
  ## The query goes to FTS5 as-is, so its operators (AND, OR, NEAR, "phrase")
  ## work. That also means a stray quote is a syntax error rather than a
  ## literal, which is why callers should treat a raised error as "no results"
  ## rather than a failure.
  ##
  ## MATCH is in a subquery rather than joined directly: FTS5 requires the
  ## bare table name on the left of MATCH, so an aliased or schema-qualified
  ## reference is rejected. The subquery also carries `rank` out for ordering.
  if query.strip.len == 0: return @[]
  g.db.queryAll("SELECT " & articleColumns & articleFrom & """
    JOIN articles.subscriptions s
      ON s.feed_url = a.feed_url AND s.user_did = ?
    JOIN (SELECT rowid, rank FROM articles.articles_fts
          WHERE articles_fts MATCH ?) m ON m.rowid = a.id
    ORDER BY m.rank
    LIMIT ? OFFSET ?""",
    [p(userDid), p(userDid), p(query),
     p(limit.int64), p(offset.int64)], readArticleRow)

proc batchUpsertArticles*(g: GleanDb, articles: seq[NewArticle],
                          retentionDays = 0) =
  ## Ingest a fetch. One transaction, one prepared statement.
  ##
  ## Two behaviours carried over from the Go version, both about not
  ## resurfacing things the reader has dealt with:
  ##   * Articles already older than the retention window are skipped --
  ##     ingesting them only to purge them churns the database.
  ##   * Read state is restored from history afterwards, because a purged
  ##     article that comes back gets a new surrogate id and would otherwise
  ##     read as unread again.
  if articles.len == 0: return

  var cutoff = none(DateTime)
  if retentionDays > 0:
    cutoff = some(now().utc - initDuration(days = retentionDays))

  var feedUrls: seq[string]
  g.db.transaction:
    var ps = g.db.prepared("""
      INSERT INTO articles.articles
      (feed_url, guid, title, url, author, summary, content, published, updated, language)
      VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, '')
      ON CONFLICT(feed_url, guid) DO NOTHING""")
    defer: ps.finalize()

    for a in articles:
      if cutoff.isSome and a.published.isSome and a.published.get < cutoff.get:
        continue
      if a.feedUrl notin feedUrls:
        feedUrls.add a.feedUrl
      ps.exec(
        p(a.feedUrl), p(a.guid), p(a.title), pOrNull(a.url), pOrNull(a.author),
        pOrNull(a.summary), pOrNull(a.content),
        (if a.published.isSome: p(formatSqliteTime(a.published.get)) else: nullParam()),
        (if a.updated.isSome: p(formatSqliteTime(a.updated.get)) else: nullParam()))

    var restore = g.db.prepared("""
      INSERT INTO articles.read_state (user_did, article_id, is_read, read_at)
      SELECT h.user_did, a.id, h.is_read, h.read_at
      FROM articles.read_state_history h
      JOIN articles.articles a ON a.feed_url = h.feed_url AND a.guid = h.guid
      WHERE h.feed_url = ?
      ON CONFLICT(user_did, article_id) DO NOTHING""")
    defer: restore.finalize()
    for url in feedUrls:
      restore.exec(p(url))

proc updateArticleFullContent*(g: GleanDb, id: int64, fullContent: string) =
  g.db.run("UPDATE articles.articles SET full_content = ? WHERE id = ?",
           pOrNull(fullContent), p(id))

# --- read state ------------------------------------------------------------

proc setRead(g: GleanDb, userDid: string, articleId: int64, isRead: bool) =
  g.db.run("""INSERT INTO articles.read_state (user_did, article_id, is_read, read_at)
              VALUES (?, ?, ?, CURRENT_TIMESTAMP)
              ON CONFLICT(user_did, article_id) DO UPDATE SET
                is_read = excluded.is_read,
                read_at = excluded.read_at""",
           p(userDid), p(articleId), p(if isRead: 1'i64 else: 0'i64))

proc markArticleRead*(g: GleanDb, userDid: string, articleId: int64) =
  setRead(g, userDid, articleId, true)

proc markArticleUnread*(g: GleanDb, userDid: string, articleId: int64) =
  setRead(g, userDid, articleId, false)

proc markAllRead*(g: GleanDb, userDid, feedUrl: string) =
  ## Scoped to one feed when given, otherwise everything the user subscribes
  ## to. Getting this scope wrong is the difference between clearing a feed
  ## and clearing the whole account, so the two cases are separate statements
  ## rather than one with an optional predicate.
  if feedUrl.len > 0:
    g.db.run("""INSERT INTO articles.read_state (user_did, article_id, is_read, read_at)
                SELECT ?, a.id, 1, CURRENT_TIMESTAMP
                FROM articles.articles a
                WHERE a.feed_url = ?
                ON CONFLICT(user_did, article_id) DO UPDATE SET
                  is_read = 1, read_at = CURRENT_TIMESTAMP""",
             p(userDid), p(feedUrl))
  else:
    g.db.run("""INSERT INTO articles.read_state (user_did, article_id, is_read, read_at)
                SELECT ?, a.id, 1, CURRENT_TIMESTAMP
                FROM articles.articles a
                JOIN articles.subscriptions s
                  ON s.feed_url = a.feed_url AND s.user_did = ?
                ON CONFLICT(user_did, article_id) DO UPDATE SET
                  is_read = 1, read_at = CURRENT_TIMESTAMP""",
             p(userDid), p(userDid))

proc getUnreadCount*(g: GleanDb, userDid, feedUrl, category: string): int =
  var sql = """
    SELECT count(*)
    FROM articles.articles a
    JOIN articles.subscriptions s
      ON s.feed_url = a.feed_url AND s.user_did = ?
    LEFT JOIN articles.read_state r
      ON r.article_id = a.id AND r.user_did = ?
    WHERE COALESCE(r.is_read, 0) = 0"""
  var params = @[p(userDid), p(userDid)]
  if feedUrl.len > 0:
    sql &= " AND a.feed_url = ?"
    params.add p(feedUrl)
  if category.len > 0:
    sql &= " AND s.category = ?"
    params.add p(category)
  g.db.queryInt(sql, params).get(0).int

proc isRead*(g: GleanDb, userDid: string, articleId: int64): bool =
  g.db.queryInt("""SELECT COALESCE(is_read, 0) FROM articles.read_state
                   WHERE user_did = ? AND article_id = ?""",
                p(userDid), p(articleId)).get(0) != 0

proc countNewArticles*(g: GleanDb, userDid: string, since: DateTime): int =
  g.db.queryInt("""
    SELECT count(*) FROM articles.articles a
    JOIN articles.subscriptions s
      ON s.feed_url = a.feed_url AND s.user_did = ?
    WHERE a.fetched_at > ?""",
    p(userDid), p(formatSqliteTime(since))).get(0).int
