## Exercises the article and feed queries against a real database.
##
##   nim c -r -d:ssl stores_probe.nim
##
## The checks lean on the places these queries can be quietly wrong: a scope
## that clears more than it should, a count that drifts, a listing that
## returns other people's feeds, a NULL that sorts to the top.

import std/[options, os, strformat, strutils, times]
import reader/[articlestore, feedstore, gleandb, sqlite]

var failures = 0

proc report(name: string, ok: bool, detail = "") =
  if not ok: inc failures
  let label = if ok: "PASS" else: "FAIL"
  if detail.len > 0: echo &"  {label}  {name}  -- {detail}"
  else: echo &"  {label}  {name}"

proc cleanup(base: string) =
  for suffix in ["_users", "_articles", "_recs"]:
    for ext in ["", "-wal", "-shm"]:
      removeFile(base & suffix & ext)

const
  alice = "did:plc:alice"
  bob = "did:plc:bob"
  feedA = "https://a.test/feed"
  feedB = "https://b.test/feed"

proc main() =
  echo "Article / feed store probe"
  echo ""

  let base = getTempDir() / "glean_stores_probe"
  cleanup(base)
  defer: cleanup(base)

  var g = gleandb.open(base)
  defer: g.close()
  g.migrate()

  for did in [alice, bob]:
    g.db.run("INSERT INTO users (did) VALUES (?)", p(did))

  # --- feeds --------------------------------------------------------------
  block:
    g.upsertFeed(Feed(feedUrl: feedA, title: "Feed A", feedType: "rss",
                      siteUrl: "https://a.test"))
    g.upsertFeed(Feed(feedUrl: feedB, title: "Feed B", feedType: "atom"))
    let f = g.getFeed(feedA)
    report("a feed round-trips", f.isSome and f.get.title == "Feed A",
           f.get(Feed()).title)

    # A metadata refresh must not reset state the fetcher owns.
    g.markFeedFetchError(feedA, "boom")
    g.upsertFeed(Feed(feedUrl: feedA, title: "Feed A renamed", feedType: "rss"))
    let after = g.getFeed(feedA).get
    report("upsert refreshes metadata", after.title == "Feed A renamed")
    report("upsert preserves error state", after.errorCount == 1,
           &"error_count={after.errorCount}")

    g.markFeedFetched(feedA)
    let healed = g.getFeed(feedA).get
    report("a successful fetch clears the error state",
           healed.errorCount == 0 and healed.lastError.len == 0)
    report("last_fetched_at is recorded", healed.lastFetchedAt.isSome)

  # --- subscriptions ------------------------------------------------------
  block:
    g.createSubscription(alice, feedA, "", "news", "", "")
    g.createSubscription(alice, feedB, "", "", "", "")
    g.createSubscription(bob, feedA, "", "", "", "")

    report("subscriber_count tracks subscribers",
           g.getFeed(feedA).get.subscriberCount == 2,
           $g.getFeed(feedA).get.subscriberCount)

    # Re-subscribing is an update, not a second subscriber.
    g.createSubscription(alice, feedA, "My title", "tech", "", "")
    report("re-subscribing does not inflate the count",
           g.getFeed(feedA).get.subscriberCount == 2,
           $g.getFeed(feedA).get.subscriberCount)
    report("re-subscribing updates the row",
           g.getSubscription(alice, feedA).get.category == "tech")

    report("subscription count is per user",
           g.getSubscriptionCount(alice) == 2 and g.getSubscriptionCount(bob) == 1)
    report("categories exclude empties",
           g.getCategories(alice) == @["tech"], g.getCategories(alice).join(","))

    let listed = g.listSubscriptions(alice, "", 10, 0)
    report("listing returns only this user's subscriptions", listed.len == 2,
           $listed.len)
    report("listing falls back to the feed title when none is set",
           g.listSubscriptions(bob, "", 10, 0)[0].feedTitle == "Feed A renamed")

  # --- articles -----------------------------------------------------------
  block:
    let t0 = now().utc - initDuration(days = 2)
    g.batchUpsertArticles(@[
      NewArticle(feedUrl: feedA, guid: "a1", title: "Nim systems programming",
                 url: "https://a.test/1", summary: "about nim",
                 published: some(t0)),
      NewArticle(feedUrl: feedA, guid: "a2", title: "Second post",
                 url: "https://a.test/2", published: some(t0 + initDuration(hours = 1))),
      NewArticle(feedUrl: feedB, guid: "b1", title: "Elsewhere",
                 url: "https://b.test/1", published: some(t0)),
    ])
    let all = g.listArticles(alice, limit = 50)
    report("articles are ingested and listed", all.len == 3, $all.len)

    # Re-ingesting the same guids must not duplicate.
    g.batchUpsertArticles(@[
      NewArticle(feedUrl: feedA, guid: "a1", title: "Nim systems programming",
                 published: some(t0))])
    report("re-ingesting is idempotent",
           g.listArticles(alice, limit = 50).len == 3)

    report("bob only sees his own subscriptions",
           g.listArticles(bob, limit = 50).len == 2,
           $g.listArticles(bob, limit = 50).len)

    # Retention: an article older than the window is not worth ingesting.
    g.batchUpsertArticles(@[
      NewArticle(feedUrl: feedA, guid: "ancient", title: "Old news",
                 published: some(now().utc - initDuration(days = 90)))],
      retentionDays = 30)
    report("articles past the retention window are skipped",
           g.listArticles(alice, limit = 50).len == 3)

    let newest = g.listArticles(alice, feedUrl = feedA, limit = 10)
    report("newest first by default", newest[0].guid == "a2", newest[0].guid)
    let oldest = g.listArticles(alice, feedUrl = feedA, sortOldest = true, limit = 10)
    report("oldest first when asked", oldest[0].guid == "a1", oldest[0].guid)

    report("filtering by feed scopes the listing",
           g.listArticles(alice, feedUrl = feedB, limit = 10).len == 1)
    report("filtering by category scopes the listing",
           g.listArticles(alice, category = "tech", limit = 10).len == 2,
           $g.listArticles(alice, category = "tech", limit = 10).len)

  # --- read state ---------------------------------------------------------
  block:
    let arts = g.listArticles(alice, feedUrl = feedA, limit = 10)
    let first = arts[0]

    report("everything starts unread", g.getUnreadCount(alice, "", "") == 3,
           $g.getUnreadCount(alice, "", ""))

    g.markArticleRead(alice, first.id)
    report("marking read is reflected in the count",
           g.getUnreadCount(alice, "", "") == 2)
    report("read state is per user",
           g.getUnreadCount(bob, "", "") == 2, $g.getUnreadCount(bob, "", ""))
    report("the article reads back as read",
           g.getArticle(alice, first.id).get.isRead)

    report("unread filter excludes it",
           g.listArticles(alice, filter = afUnread, limit = 50).len == 2)
    report("read filter includes only it",
           g.listArticles(alice, filter = afRead, limit = 50).len == 1)

    g.markArticleUnread(alice, first.id)
    report("marking unread reverses it", g.getUnreadCount(alice, "", "") == 3)

    # The scope that matters: one feed, not the account.
    g.markAllRead(alice, feedA)
    report("mark-all-read on a feed clears only that feed",
           g.getUnreadCount(alice, feedA, "") == 0 and
           g.getUnreadCount(alice, feedB, "") == 1,
           &"A={g.getUnreadCount(alice, feedA, \"\")} B={g.getUnreadCount(alice, feedB, \"\")}")
    report("and leaves other users alone",
           g.getUnreadCount(bob, "", "") == 2)

    g.markAllRead(alice, "")
    report("unscoped mark-all-read clears everything subscribed",
           g.getUnreadCount(alice, "", "") == 0)

  # --- search -------------------------------------------------------------
  block:
    let hits = g.searchArticles(alice, "systems")
    report("full-text search finds an article", hits.len == 1,
           if hits.len > 0: hits[0].title else: "no hits")
    report("search is scoped to subscriptions",
           g.searchArticles(bob, "Elsewhere").len == 0,
           "bob is not subscribed to feed B")
    report("an empty query returns nothing", g.searchArticles(alice, "  ").len == 0)

  # --- unsubscribe --------------------------------------------------------
  block:
    g.deleteSubscription(alice, feedA)
    report("unsubscribing decrements the count",
           g.getFeed(feedA).get.subscriberCount == 1,
           $g.getFeed(feedA).get.subscriberCount)
    report("unsubscribing hides the feed's articles",
           g.listArticles(alice, limit = 50).len == 1)

    # The counts are maintained incrementally, so the repair pass must agree.
    g.db.run("UPDATE articles.feeds SET subscriber_count = 99")
    g.recountSubscriberCounts()
    report("the recount repairs drifted counts",
           g.getFeed(feedA).get.subscriberCount == 1,
           $g.getFeed(feedA).get.subscriberCount)

  # --- dead feeds ---------------------------------------------------------
  block:
    for _ in 0 ..< 3:
      g.markFeedFetchError(feedB, "still broken")
    let dead = g.listDeadFeeds(alice, threshold = 3)
    report("dead feeds are listed for the subscriber",
           dead.len == 1 and dead[0].feedUrl == feedB, $dead.len)
    report("a healthy feed is not listed as dead",
           g.listDeadFeeds(bob, threshold = 3).len == 0)

  echo ""
  if failures == 0:
    echo "RESULT: the article and feed queries behave."
  else:
    echo &"RESULT: {failures} check(s) failed."
    quit 1

when isMainModule:
  main()
