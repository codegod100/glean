## Likes, annotations and the social reads built on them, ported from
## internal/db/social.go.
##
## Likes and annotations are ATProto records first and rows second. They are
## keyed by `uri` (the at:// record URI) because that is the identity the PDS
## owns, and every one of them can arrive twice -- once from the Jetstream
## firehose and once from a PDS sync -- so every write here is idempotent by
## construction rather than by checking first.

import std/[options, strutils, times]
import ./articlestore
import ./feedstore
import ./gleandb
import ./sqlite

type
  Annotation* = object
    id*: int64
    uri*: string
    authorDid*: string
    feedUrl*: string
    articleUrl*: string
    quote*: string
    note*: string
    tags*: seq[string]
    rating*: Option[int]
    createdAt*: Option[DateTime]
    cid*: string

  Like* = object
    id*: int64
    uri*: string
    authorDid*: string
    feedUrl*: string
    articleUrl*: string
    createdAt*: Option[DateTime]
    cid*: string

  TrendingItem* = object
    articleId*: int64
    title*: string
    url*: string
    author*: string
    summary*: string
    feedUrl*: string
    feedTitle*: string
    faviconUrl*: string
    likeCount*: int
    annotationCount*: int
    hasLiked*: bool

# --- helpers ---------------------------------------------------------------

proc splitTags(s: string): seq[string] =
  for t in s.split(','):
    let v = t.strip()
    if v.len > 0: result.add v

proc joinTags(tags: seq[string]): string = tags.join(",")

proc langFilter*(languages: seq[string], prefix: string): (string, seq[Param]) =
  ## Restrict to the reader's languages, always keeping unknown ones.
  ##
  ## `language = ''` means detection has not run yet, not "no language". It is
  ## included deliberately: excluding it would make new articles vanish for
  ## anyone with a filter set, and reappear minutes later once classified.
  if languages.len == 0:
    return ("", @[])
  var holes: seq[string]
  var params: seq[Param]
  for l in languages:
    holes.add "?"
    params.add p(l)
  (" AND (" & prefix & "language IN (" & holes.join(",") & ") OR " &
     prefix & "language = '')", params)

# --- annotations -----------------------------------------------------------

proc readAnnotation(r: Row): Annotation =
  Annotation(
    id: r.i64(0),
    uri: r.str(1),
    authorDid: r.str(2),
    feedUrl: r.str(3),
    articleUrl: r.str(4),
    quote: r.str(5),
    note: r.str(6),
    tags: splitTags(r.str(7)),
    rating: (if r.isNull(8): none(int) else: some(r.i(8))),
    createdAt: parseSqliteTime(r.str(9)),
    cid: r.str(10),
  )

const annotationColumns = """an.id, an.uri, an.author_did, an.feed_url,
  an.article_url, an.quote, an.note, an.tags, an.rating, an.created_at, an.cid"""

proc createAnnotation*(g: GleanDb, a: Annotation) =
  ## Upsert on `uri`: the same record can arrive from Jetstream and from a
  ## sync, and an edit keeps the URI while changing the content.
  g.db.run("""INSERT INTO articles.annotations
              (uri, author_did, feed_url, article_url, quote, note, tags,
               rating, created_at, cid)
              VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
              ON CONFLICT(uri) DO UPDATE SET
                quote = excluded.quote,
                note = excluded.note,
                tags = excluded.tags,
                rating = excluded.rating,
                cid = excluded.cid""",
           p(a.uri), p(a.authorDid), p(a.feedUrl), p(a.articleUrl),
           pOrNull(a.quote), pOrNull(a.note), pOrNull(joinTags(a.tags)),
           (if a.rating.isSome: p(a.rating.get.int64) else: nullParam()),
           p(formatSqliteTime(a.createdAt.get(now().utc))),
           pOrNull(a.cid))

proc getAnnotation*(g: GleanDb, id: int64): Option[Annotation] =
  g.db.queryFirst("SELECT " & annotationColumns &
                  " FROM articles.annotations an WHERE an.id = ?",
                  [p(id)], readAnnotation)

proc annotationExists*(g: GleanDb, uri: string): bool =
  g.db.queryInt("SELECT 1 FROM articles.annotations WHERE uri = ?",
                p(uri)).isSome

proc deleteAnnotation*(g: GleanDb, uri: string) =
  g.db.run("DELETE FROM articles.annotations WHERE uri = ?", p(uri))

proc listAnnotations*(g: GleanDb, feedUrl = "", articleUrl = "",
                      authorDid = "", limit = 50, offset = 0): seq[Annotation] =
  var sql = "SELECT " & annotationColumns &
            " FROM articles.annotations an WHERE 1=1"
  var params: seq[Param]
  if feedUrl.len > 0:
    sql &= " AND an.feed_url = ?"
    params.add p(feedUrl)
  if articleUrl.len > 0:
    sql &= " AND an.article_url = ?"
    params.add p(articleUrl)
  if authorDid.len > 0:
    sql &= " AND an.author_did = ?"
    params.add p(authorDid)
  sql &= " ORDER BY an.created_at DESC LIMIT ? OFFSET ?"
  params.add p(limit.int64)
  params.add p(offset.int64)
  g.db.queryAll(sql, params, readAnnotation)

proc batchCreateAnnotations*(g: GleanDb, annotations: seq[Annotation]) =
  if annotations.len == 0: return
  g.db.transaction:
    var ps = g.db.prepared("""
      INSERT INTO articles.annotations
      (uri, author_did, feed_url, article_url, quote, note, tags, rating,
       created_at, cid)
      VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
      ON CONFLICT(uri) DO NOTHING""")
    defer: ps.finalize()
    for a in annotations:
      ps.exec(p(a.uri), p(a.authorDid), p(a.feedUrl), p(a.articleUrl),
              pOrNull(a.quote), pOrNull(a.note), pOrNull(joinTags(a.tags)),
              (if a.rating.isSome: p(a.rating.get.int64) else: nullParam()),
              p(formatSqliteTime(a.createdAt.get(now().utc))), pOrNull(a.cid))

# --- likes -----------------------------------------------------------------

proc readLike(r: Row): Like =
  Like(
    id: r.i64(0),
    uri: r.str(1),
    authorDid: r.str(2),
    feedUrl: r.str(3),
    articleUrl: r.str(4),
    createdAt: parseSqliteTime(r.str(5)),
    cid: r.str(6),
  )

const likeColumns = """l.id, l.uri, l.author_did, l.feed_url, l.article_url,
  l.created_at, l.cid"""

proc createLike*(g: GleanDb, l: Like) =
  g.db.run("""INSERT INTO articles.likes
              (uri, author_did, feed_url, article_url, created_at, cid)
              VALUES (?, ?, ?, ?, ?, ?)
              ON CONFLICT(uri) DO NOTHING""",
           p(l.uri), p(l.authorDid), p(l.feedUrl), p(l.articleUrl),
           p(formatSqliteTime(l.createdAt.get(now().utc))), pOrNull(l.cid))

proc batchCreateLikes*(g: GleanDb, likes: seq[Like]) =
  if likes.len == 0: return
  g.db.transaction:
    var ps = g.db.prepared("""
      INSERT INTO articles.likes
      (uri, author_did, feed_url, article_url, created_at, cid)
      VALUES (?, ?, ?, ?, ?, ?)
      ON CONFLICT(uri) DO NOTHING""")
    defer: ps.finalize()
    for l in likes:
      ps.exec(p(l.uri), p(l.authorDid), p(l.feedUrl), p(l.articleUrl),
              p(formatSqliteTime(l.createdAt.get(now().utc))), pOrNull(l.cid))

proc deleteLike*(g: GleanDb, uri: string) =
  g.db.run("DELETE FROM articles.likes WHERE uri = ?", p(uri))

proc deleteLikeByUserArticle*(g: GleanDb, authorDid, feedUrl, articleUrl: string) =
  g.db.run("""DELETE FROM articles.likes
              WHERE author_did = ? AND feed_url = ? AND article_url = ?""",
           p(authorDid), p(feedUrl), p(articleUrl))

proc getLike*(g: GleanDb, authorDid, feedUrl, articleUrl: string): Option[Like] =
  g.db.queryFirst("SELECT " & likeColumns & """
    FROM articles.likes l
    WHERE l.author_did = ? AND l.feed_url = ? AND l.article_url = ?""",
    [p(authorDid), p(feedUrl), p(articleUrl)], readLike)

proc hasLiked*(g: GleanDb, authorDid, feedUrl, articleUrl: string): bool =
  getLike(g, authorDid, feedUrl, articleUrl).isSome

proc getLikeCount*(g: GleanDb, feedUrl, articleUrl: string): int =
  g.db.queryInt("""SELECT count(*) FROM articles.likes
                   WHERE feed_url = ? AND article_url = ?""",
                p(feedUrl), p(articleUrl)).get(0).int

proc listLikes*(g: GleanDb, authorDid: string; feedUrl = "",
                limit = 50, offset = 0): seq[Like] =
  var sql = "SELECT " & likeColumns &
            " FROM articles.likes l WHERE l.author_did = ?"
  var params = @[p(authorDid)]
  if feedUrl.len > 0:
    sql &= " AND l.feed_url = ?"
    params.add p(feedUrl)
  sql &= " ORDER BY l.created_at DESC LIMIT ? OFFSET ?"
  params.add p(limit.int64)
  params.add p(offset.int64)
  g.db.queryAll(sql, params, readLike)

# --- reconciliation --------------------------------------------------------

proc deleteOrphanedLikes*(g: GleanDb, userDid: string, activeUris: seq[string]) =
  ## Remove local likes the user's PDS no longer has.
  ##
  ## This is the destructive half of a sync, so an empty active set is treated
  ## as "delete everything this user liked" only because that is what an empty
  ## PDS collection means. A caller that failed to *fetch* the collection must
  ## not reach here -- the difference between "you unliked everything" and
  ## "the request failed" is not visible from this side.
  if activeUris.len == 0:
    g.db.run("DELETE FROM articles.likes WHERE author_did = ?", p(userDid))
    return
  g.db.transaction:
    g.db.run("CREATE TEMP TABLE IF NOT EXISTS active_uris (uri TEXT PRIMARY KEY)")
    g.db.run("DELETE FROM active_uris")
    var ps = g.db.prepared(
      "INSERT OR IGNORE INTO active_uris (uri) VALUES (?)")
    defer: ps.finalize()
    for uri in activeUris:
      ps.exec(p(uri))
    g.db.run("""DELETE FROM articles.likes
                WHERE author_did = ?
                  AND uri NOT IN (SELECT uri FROM active_uris)""", p(userDid))

proc deleteOrphanedAnnotations*(g: GleanDb, userDid: string,
                                activeUris: seq[string]) =
  if activeUris.len == 0:
    g.db.run("DELETE FROM articles.annotations WHERE author_did = ?", p(userDid))
    return
  g.db.transaction:
    g.db.run("CREATE TEMP TABLE IF NOT EXISTS active_uris (uri TEXT PRIMARY KEY)")
    g.db.run("DELETE FROM active_uris")
    var ps = g.db.prepared(
      "INSERT OR IGNORE INTO active_uris (uri) VALUES (?)")
    defer: ps.finalize()
    for uri in activeUris:
      ps.exec(p(uri))
    g.db.run("""DELETE FROM articles.annotations
                WHERE author_did = ?
                  AND uri NOT IN (SELECT uri FROM active_uris)""", p(userDid))

# --- follows ---------------------------------------------------------------

proc upsertFollow*(g: GleanDb, userDid, targetDid, uri, cid: string) =
  g.db.run("""INSERT INTO follows (user_did, target_did, uri, cid, followed_at)
              VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)
              ON CONFLICT(user_did, target_did) DO UPDATE SET
                uri = COALESCE(excluded.uri, follows.uri),
                cid = COALESCE(excluded.cid, follows.cid)""",
           p(userDid), p(targetDid), pOrNull(uri), pOrNull(cid))

proc deleteFollow*(g: GleanDb, userDid, targetDid: string) =
  g.db.run("DELETE FROM follows WHERE user_did = ? AND target_did = ?",
           p(userDid), p(targetDid))

proc listFollows*(g: GleanDb, userDid: string): seq[string] =
  for r in g.db.query(
      "SELECT target_did FROM follows WHERE user_did = ? ORDER BY target_did",
      p(userDid)):
    result.add r.str(0)

# --- trending --------------------------------------------------------------

proc readTrending(r: Row): TrendingItem =
  TrendingItem(
    articleId: r.i64(0),
    title: r.str(1),
    url: r.str(2),
    author: r.str(3),
    summary: r.str(4),
    feedUrl: r.str(5),
    feedTitle: r.str(6),
    faviconUrl: r.str(7),
    likeCount: r.i(8),
    annotationCount: r.i(9),
    hasLiked: r.b(10),
  )

const trendingSelect = """
  SELECT ar.id, ar.title, COALESCE(ar.url, ''), COALESCE(ar.author, ''),
         COALESCE(ar.summary, ''), l.feed_url, COALESCE(f.title, ''),
         COALESCE(f.favicon_url, ''),
         COUNT(DISTINCT l.id) AS like_count,
         COUNT(DISTINCT an.id) AS annotation_count,
         COALESCE(MAX(CASE WHEN ul.id IS NOT NULL THEN 1 ELSE 0 END), 0)
  FROM articles.likes l
  JOIN articles.articles ar
    ON ar.url = l.article_url AND ar.feed_url = l.feed_url
  LEFT JOIN articles.feeds f ON f.feed_url = l.feed_url
  LEFT JOIN articles.annotations an
    ON an.feed_url = l.feed_url AND an.article_url = l.article_url
   AND an.created_at >= ?
  LEFT JOIN articles.likes ul
    ON ul.feed_url = l.feed_url AND ul.article_url = l.article_url
   AND ul.author_did = ?
  WHERE l.created_at >= ?"""

## Future-dated articles sort last: a feed that publishes with a scheduled
## timestamp would otherwise pin itself to the top of trending indefinitely.
const trendingOrder = """
  GROUP BY ar.id
  ORDER BY like_count DESC, annotation_count DESC,
           (CASE WHEN ar.published > datetime('now') THEN 1 ELSE 0 END),
           ar.published DESC
  LIMIT ? OFFSET ?"""

proc listTrendingArticles*(g: GleanDb, userDid, since: string,
                           limit = 25, offset = 0): seq[TrendingItem] =
  ## Global trending: everyone's likes.
  g.db.queryAll(trendingSelect & trendingOrder,
                [p(since), p(userDid), p(since), p(limit.int64), p(offset.int64)],
                readTrending)

proc listTrendingArticlesForUser*(g: GleanDb, userDid, since: string,
                                  languages: seq[string] = @[],
                                  limit = 25, offset = 0): seq[TrendingItem] =
  ## Trending within the reader's own graph: people they follow, people the
  ## similarity table pairs them with, and themselves.
  let (filter, langParams) = langFilter(languages, "ar.")
  var params = @[p(since), p(userDid), p(since),
                 p(userDid), p(userDid), p(userDid), p(userDid), p(userDid)]
  params.add langParams
  params.add p(limit.int64)
  params.add p(offset.int64)
  g.db.queryAll(trendingSelect & """
    AND l.author_did IN (
      SELECT CASE WHEN us.user_a = ? THEN us.user_b ELSE us.user_a END
      FROM recs.user_similarity us
      WHERE us.user_a = ? OR us.user_b = ?
      UNION SELECT ?
      UNION SELECT fo.target_did FROM follows fo WHERE fo.user_did = ?
    )""" & filter & trendingOrder, params, readTrending)

proc listLikedArticles*(g: GleanDb, userDid: string,
                        limit = 25, offset = 0): seq[Article] =
  ## The reader's own likes, as articles.
  g.db.queryAll("""
    SELECT a.id, a.feed_url, COALESCE(f.title, a.feed_url), f.favicon_url,
           a.guid, a.title, a.url, a.author, a.summary, a.content,
           a.full_content, a.published, a.updated,
           COALESCE(r.is_read, 0),
           (SELECT count(*) FROM articles.likes l2 WHERE l2.article_url = a.url),
           1
    FROM articles.likes l
    JOIN articles.articles a
      ON a.url = l.article_url AND a.feed_url = l.feed_url
    LEFT JOIN articles.feeds f ON f.feed_url = a.feed_url
    LEFT JOIN articles.read_state r ON r.article_id = a.id AND r.user_did = ?
    WHERE l.author_did = ?
    ORDER BY l.created_at DESC
    LIMIT ? OFFSET ?""",
    [p(userDid), p(userDid), p(limit.int64), p(offset.int64)],
    proc(r: Row): Article = readArticleRow(r))
