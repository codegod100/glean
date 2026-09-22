## Feeds and subscriptions, ported from internal/db/feed.go.
##
## `feeds` is global -- one row per URL however many people subscribe -- and
## `subscriptions` is the per-user join onto it. That split is why
## subscriber_count exists as a stored column rather than a COUNT(*): it is
## read on every listing and written only when someone subscribes.

import std/[options, times]
import ./pulseboarddb
import ./sqlite

type
  Feed* = object
    feedUrl*: string
    title*: string
    siteUrl*: string
    description*: string
    feedType*: string
    faviconUrl*: string
    subscriberCount*: int
    errorCount*: int
    lastError*: string
    lastFetchedAt*: Option[DateTime]

  Subscription* = object
    id*: int64
    userDid*: string
    feedUrl*: string
    feedTitle*: string
    category*: string
    uri*: string
    cid*: string
    addedAt*: Option[DateTime]
    unreadCount*: int
    faviconUrl*: string

const SqliteTimeFormat = "yyyy-MM-dd HH:mm:ss"

proc parseSqliteTime*(s: string): Option[DateTime] =
  ## SQLite has no date type; CURRENT_TIMESTAMP writes "YYYY-MM-DD HH:MM:SS"
  ## in UTC. Values written by Go's driver can carry a timezone and
  ## fractional seconds, so both shapes are accepted.
  if s.len == 0: return none(DateTime)
  for fmt in [SqliteTimeFormat, "yyyy-MM-dd'T'HH:mm:ss'Z'",
              "yyyy-MM-dd HH:mm:ss'.'fff", "yyyy-MM-dd'T'HH:mm:sszzz"]:
    try:
      return some(parse(s, fmt, utc()))
    except TimeParseError:
      continue
  none(DateTime)

proc formatSqliteTime*(t: DateTime): string =
  t.utc.format(SqliteTimeFormat)

proc readFeed(r: Row): Feed =
  Feed(
    feedUrl: r.str(0),
    title: r.str(1),
    siteUrl: r.str(2),
    description: r.str(3),
    feedType: r.str(4),
    faviconUrl: r.str(5),
    subscriberCount: r.i(6),
    errorCount: r.i(7),
    lastError: r.str(8),
    lastFetchedAt: parseSqliteTime(r.str(9)),
  )

## Qualified with the `f` alias, which every query below uses. Bare names
## work until a query joins subscriptions -- which also has feed_url -- and
## then fail as ambiguous rather than picking the wrong one.
const feedColumns = """f.feed_url, f.title, f.site_url, f.description,
  f.feed_type, f.favicon_url, f.subscriber_count, f.error_count, f.last_error,
  f.last_fetched_at"""

proc upsertFeed*(g: PulseboardDb, f: Feed) =
  ## Insert, or refresh the metadata of an existing feed.
  ##
  ## subscriber_count and the error columns are deliberately not touched: they
  ## are owned by subscribe/unsubscribe and by the fetcher, and a metadata
  ## refresh that reset them would silently revive dead feeds.
  g.db.run("""INSERT INTO articles.feeds
              (feed_url, title, site_url, description, feed_type, favicon_url)
              VALUES (?, ?, ?, ?, ?, ?)
              ON CONFLICT(feed_url) DO UPDATE SET
                title = excluded.title,
                site_url = excluded.site_url,
                description = excluded.description,
                feed_type = COALESCE(excluded.feed_type, articles.feeds.feed_type),
                favicon_url = COALESCE(excluded.favicon_url, articles.feeds.favicon_url)""",
           p(f.feedUrl), pOrNull(f.title), pOrNull(f.siteUrl),
           pOrNull(f.description), pOrNull(f.feedType), pOrNull(f.faviconUrl))

proc getFeed*(g: PulseboardDb, feedUrl: string): Option[Feed] =
  g.db.queryFirst("SELECT " & feedColumns &
                  " FROM articles.feeds f WHERE f.feed_url = ?",
                  [p(feedUrl)], readFeed)

proc getFeedsToFetch*(g: PulseboardDb, olderThan: Duration, limit: int): seq[Feed] =
  ## Feeds due for a refresh: never fetched, or last fetched before the
  ## cutoff. Dead feeds are excluded by error_count so a permanently broken
  ## URL does not consume a fetch slot every cycle.
  let cutoff = formatSqliteTime(now().utc - olderThan)
  g.db.queryAll("""SELECT """ & feedColumns & """
                   FROM articles.feeds f
                   WHERE f.subscriber_count > 0
                     AND f.error_count < 10
                     AND (f.last_fetched_at IS NULL OR f.last_fetched_at < ?)
                   ORDER BY f.last_fetched_at IS NOT NULL, f.last_fetched_at
                   LIMIT ?""",
                [p(cutoff), p(limit.int64)], readFeed)

proc markFeedFetched*(g: PulseboardDb, feedUrl: string) =
  ## A successful fetch clears the error state, so a feed that recovers
  ## re-enters the rotation instead of staying dead.
  g.db.run("""UPDATE articles.feeds
              SET last_fetched_at = CURRENT_TIMESTAMP,
                  last_error = NULL,
                  error_count = 0
              WHERE feed_url = ?""", p(feedUrl))

proc markFeedFetchError*(g: PulseboardDb, feedUrl, lastError: string) =
  g.db.run("""UPDATE articles.feeds
              SET last_fetched_at = CURRENT_TIMESTAMP,
                  last_error = ?,
                  error_count = error_count + 1
              WHERE feed_url = ?""", p(lastError), p(feedUrl))

proc updateFeedFavicon*(g: PulseboardDb, feedUrl, faviconUrl: string) =
  g.db.run("UPDATE articles.feeds SET favicon_url = ? WHERE feed_url = ?",
           pOrNull(faviconUrl), p(feedUrl))

# --- subscriptions ---------------------------------------------------------

proc readSubscription(r: Row): Subscription =
  Subscription(
    id: r.i64(0),
    userDid: r.str(1),
    feedUrl: r.str(2),
    feedTitle: r.str(3),
    category: r.str(4),
    uri: r.str(5),
    cid: r.str(6),
    addedAt: parseSqliteTime(r.str(7)),
    unreadCount: r.i(8),
    faviconUrl: r.str(9),
  )

proc createSubscription*(g: PulseboardDb, userDid, feedUrl, title, category,
                         uri, cid: string) =
  ## Subscribe, and keep subscriber_count in step.
  ##
  ## Both statements run in one transaction because the count is derived: a
  ## crash between them leaves a number that no longer matches reality, and
  ## nothing recomputes it outside the nightly recount.
  g.db.transaction:
    g.db.run("""INSERT INTO articles.subscriptions
                (user_did, feed_url, title, category, uri, cid)
                VALUES (?, ?, ?, ?, ?, ?)
                ON CONFLICT(user_did, feed_url) DO UPDATE SET
                  title = excluded.title,
                  category = excluded.category,
                  uri = COALESCE(excluded.uri, articles.subscriptions.uri),
                  cid = COALESCE(excluded.cid, articles.subscriptions.cid)""",
             p(userDid), p(feedUrl), pOrNull(title), pOrNull(category),
             pOrNull(uri), pOrNull(cid))
    # Only on a genuine insert: re-subscribing must not inflate the count.
    if g.db.changes() > 0:
      g.db.run("""UPDATE articles.feeds
                  SET subscriber_count = (
                    SELECT count(*) FROM articles.subscriptions
                    WHERE feed_url = ?)
                  WHERE feed_url = ?""", p(feedUrl), p(feedUrl))

proc deleteSubscription*(g: PulseboardDb, userDid, feedUrl: string) =
  g.db.transaction:
    g.db.run("""DELETE FROM articles.subscriptions
                WHERE user_did = ? AND feed_url = ?""", p(userDid), p(feedUrl))
    g.db.run("""UPDATE articles.feeds
                SET subscriber_count = (
                  SELECT count(*) FROM articles.subscriptions
                  WHERE feed_url = ?)
                WHERE feed_url = ?""", p(feedUrl), p(feedUrl))

proc deleteAllSubscriptions*(g: PulseboardDb, userDid: string) =
  g.db.transaction:
    g.db.run("DELETE FROM articles.subscriptions WHERE user_did = ?", p(userDid))
    g.db.run("""UPDATE articles.feeds SET subscriber_count = (
                  SELECT count(*) FROM articles.subscriptions s
                  WHERE s.feed_url = articles.feeds.feed_url)""")

proc getSubscription*(g: PulseboardDb, userDid, feedUrl: string): Option[Subscription] =
  g.db.queryFirst("""
    SELECT s.id, s.user_did, s.feed_url,
           COALESCE(s.title, f.title, s.feed_url), s.category, s.uri, s.cid,
           s.added_at, 0, f.favicon_url
    FROM articles.subscriptions s
    LEFT JOIN articles.feeds f ON f.feed_url = s.feed_url
    WHERE s.user_did = ? AND s.feed_url = ?""",
    [p(userDid), p(feedUrl)], readSubscription)

proc listSubscriptions*(g: PulseboardDb, userDid, category: string,
                        limit, offset: int): seq[Subscription] =
  ## The unread count is computed per row here rather than stored: it is
  ## per-user and changes constantly, so a cached column would be wrong more
  ## often than right.
  var sql = """
    SELECT s.id, s.user_did, s.feed_url,
           COALESCE(s.title, f.title, s.feed_url), s.category, s.uri, s.cid,
           s.added_at,
           (SELECT count(*) FROM articles.articles a
            LEFT JOIN articles.read_state r
              ON r.article_id = a.id AND r.user_did = s.user_did
            WHERE a.feed_url = s.feed_url AND COALESCE(r.is_read, 0) = 0),
           f.favicon_url
    FROM articles.subscriptions s
    LEFT JOIN articles.feeds f ON f.feed_url = s.feed_url
    WHERE s.user_did = ?"""
  var params = @[p(userDid)]
  if category.len > 0:
    sql &= " AND s.category = ?"
    params.add p(category)
  sql &= " ORDER BY COALESCE(s.title, f.title, s.feed_url) COLLATE NOCASE LIMIT ? OFFSET ?"
  params.add p(limit.int64)
  params.add p(offset.int64)
  g.db.queryAll(sql, params, readSubscription)

proc getSubscriptionCount*(g: PulseboardDb, userDid: string): int =
  g.db.queryInt("SELECT count(*) FROM articles.subscriptions WHERE user_did = ?",
                p(userDid)).get(0).int

proc getCategories*(g: PulseboardDb, userDid: string): seq[string] =
  for r in g.db.query("""SELECT DISTINCT category FROM articles.subscriptions
                         WHERE user_did = ? AND category IS NOT NULL
                           AND category != ''
                         ORDER BY category COLLATE NOCASE""", p(userDid)):
    result.add r.str(0)

proc listDeadFeeds*(g: PulseboardDb, userDid: string, threshold: int): seq[Feed] =
  ## Subscribed feeds that keep failing, so the UI can offer a retry rather
  ## than silently showing nothing.
  g.db.queryAll("""SELECT """ & feedColumns & """
                   FROM articles.feeds f
                   JOIN articles.subscriptions s ON s.feed_url = f.feed_url
                   WHERE s.user_did = ? AND f.error_count >= ?
                   ORDER BY f.error_count DESC""",
                [p(userDid), p(threshold.int64)], readFeed)

proc recountSubscriberCounts*(g: PulseboardDb) =
  ## Repair pass. The counts are maintained incrementally, so anything that
  ## interrupts a subscribe leaves them drifted.
  g.db.run("""UPDATE articles.feeds SET subscriber_count = (
                SELECT count(*) FROM articles.subscriptions s
                WHERE s.feed_url = articles.feeds.feed_url)""")

proc batchUpsertFeeds*(g: PulseboardDb, feeds: seq[Feed]) =
  if feeds.len == 0: return
  g.db.transaction:
    var ps = g.db.prepared("""
      INSERT INTO articles.feeds
      (feed_url, title, site_url, description, feed_type, favicon_url)
      VALUES (?, ?, ?, ?, ?, ?)
      ON CONFLICT(feed_url) DO UPDATE SET
        title = COALESCE(excluded.title, articles.feeds.title),
        site_url = COALESCE(excluded.site_url, articles.feeds.site_url)""")
    defer: ps.finalize()
    for f in feeds:
      ps.exec(p(f.feedUrl), pOrNull(f.title), pOrNull(f.siteUrl),
              pOrNull(f.description), pOrNull(f.feedType), pOrNull(f.faviconUrl))
