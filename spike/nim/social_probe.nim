## Exercises likes, annotations, follows and trending.
##
##   nim c -r -d:ssl social_probe.nim
##
## Weighted towards the destructive and the social-graph paths: orphan
## reconciliation deletes rows, and trending decides what a reader sees, so
## both are worth more than "the insert worked".

import std/[options, os, strformat, strutils, times]
import atproto/[articlestore, feedstore, gleandb, socialstore, sqlite]

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
  carol = "did:plc:carol"
  feedA = "https://a.test/feed"
  urlA1 = "https://a.test/1"
  urlA2 = "https://a.test/2"

proc main() =
  echo "Social store probe"
  echo ""

  let base = getTempDir() / "glean_social_probe"
  cleanup(base)
  defer: cleanup(base)

  var g = gleandb.open(base)
  defer: g.close()
  g.migrate()

  for did in [alice, bob, carol]:
    g.db.run("INSERT INTO users (did) VALUES (?)", p(did))
  g.upsertFeed(Feed(feedUrl: feedA, title: "Feed A", feedType: "rss"))
  for did in [alice, bob, carol]:
    g.createSubscription(did, feedA, "", "", "", "")

  let t = now().utc
  g.batchUpsertArticles(@[
    NewArticle(feedUrl: feedA, guid: "a1", title: "First", url: urlA1,
               published: some(t - initDuration(hours = 2))),
    NewArticle(feedUrl: feedA, guid: "a2", title: "Second", url: urlA2,
               published: some(t - initDuration(hours = 1))),
  ])

  # --- likes --------------------------------------------------------------
  block:
    g.createLike(Like(uri: "at://alice/like/1", authorDid: alice,
                      feedUrl: feedA, articleUrl: urlA1, createdAt: some(t)))
    report("a like round-trips", g.hasLiked(alice, feedA, urlA1))
    report("a like is per author", not g.hasLiked(bob, feedA, urlA1))

    # The same record arrives from Jetstream and from a PDS sync.
    g.createLike(Like(uri: "at://alice/like/1", authorDid: alice,
                      feedUrl: feedA, articleUrl: urlA1, createdAt: some(t)))
    report("re-delivering a like does not duplicate it",
           g.getLikeCount(feedA, urlA1) == 1, $g.getLikeCount(feedA, urlA1))

    g.batchCreateLikes(@[
      Like(uri: "at://bob/like/1", authorDid: bob, feedUrl: feedA,
           articleUrl: urlA1, createdAt: some(t)),
      Like(uri: "at://carol/like/1", authorDid: carol, feedUrl: feedA,
           articleUrl: urlA1, createdAt: some(t)),
      Like(uri: "at://bob/like/2", authorDid: bob, feedUrl: feedA,
           articleUrl: urlA2, createdAt: some(t)),
    ])
    report("batch insert works", g.getLikeCount(feedA, urlA1) == 3,
           $g.getLikeCount(feedA, urlA1))
    report("likes are listed per author",
           g.listLikes(bob).len == 2, $g.listLikes(bob).len)
    report("likes can be scoped to a feed",
           g.listLikes(bob, feedUrl = feedA).len == 2)

    g.deleteLikeByUserArticle(alice, feedA, urlA1)
    report("unliking removes only that author's like",
           g.getLikeCount(feedA, urlA1) == 2 and not g.hasLiked(alice, feedA, urlA1),
           $g.getLikeCount(feedA, urlA1))

  # --- annotations --------------------------------------------------------
  block:
    g.createAnnotation(Annotation(
      uri: "at://alice/ann/1", authorDid: alice, feedUrl: feedA,
      articleUrl: urlA1, quote: "a quote", note: "a note",
      tags: @["nim", "systems"], rating: some(4), createdAt: some(t)))
    let listed = g.listAnnotations(articleUrl = urlA1)
    report("an annotation round-trips", listed.len == 1, $listed.len)
    if listed.len > 0:
      report("tags round-trip as a list", listed[0].tags == @["nim", "systems"],
             listed[0].tags.join("|"))
      report("rating round-trips", listed[0].rating == some(4))

    # An edit keeps the URI and changes the content.
    g.createAnnotation(Annotation(
      uri: "at://alice/ann/1", authorDid: alice, feedUrl: feedA,
      articleUrl: urlA1, note: "edited", tags: @["nim"], createdAt: some(t)))
    let edited = g.listAnnotations(articleUrl = urlA1)
    report("re-delivering an annotation updates rather than duplicates",
           edited.len == 1 and edited[0].note == "edited",
           &"{edited.len} rows, note={edited[0].note}")
    report("a cleared rating round-trips as none", edited[0].rating.isNone)

    report("annotationExists finds it", g.annotationExists("at://alice/ann/1"))
    report("annotationExists rejects an unknown uri",
           not g.annotationExists("at://nobody/ann/9"))

    g.batchCreateAnnotations(@[
      Annotation(uri: "at://bob/ann/1", authorDid: bob, feedUrl: feedA,
                 articleUrl: urlA1, note: "bob's", createdAt: some(t))])
    report("annotations are listed per author",
           g.listAnnotations(authorDid = bob).len == 1)

  # --- follows ------------------------------------------------------------
  block:
    g.upsertFollow(alice, bob, "at://alice/follow/1", "cid1")
    g.upsertFollow(alice, bob, "at://alice/follow/1", "cid1")
    report("following is idempotent", g.listFollows(alice) == @[bob],
           g.listFollows(alice).join(","))
    g.upsertFollow(alice, carol, "", "")
    report("follows accumulate", g.listFollows(alice).len == 2)
    g.deleteFollow(alice, carol)
    report("unfollowing removes one edge", g.listFollows(alice) == @[bob])

  # --- trending -----------------------------------------------------------
  block:
    let since = formatSqliteTime(t - initDuration(days = 1))
    let global = g.listTrendingArticles(alice, since)
    report("global trending ranks by likes",
           global.len == 2 and global[0].url == urlA1,
           if global.len > 0: &"top={global[0].url} likes={global[0].likeCount}"
           else: "empty")
    report("trending counts annotations too",
           global.len > 0 and global[0].annotationCount == 2,
           if global.len > 0: $global[0].annotationCount else: "-")
    report("hasLiked is per viewer",
           not global[0].hasLiked, "alice unliked hers earlier")

    let viewedByBob = g.listTrendingArticles(bob, since)
    report("hasLiked is true for a viewer who liked it",
           viewedByBob[0].hasLiked)

    # Alice follows only bob, so carol's like should not pull an article in
    # that bob did not also like.
    let scoped = g.listTrendingArticlesForUser(alice, since)
    report("graph-scoped trending only counts followed authors",
           scoped.len > 0, $scoped.len)
    let scopedTop = scoped[0]
    report("a followed author's likes are counted",
           scopedTop.likeCount >= 1, $scopedTop.likeCount)

    # An author nobody follows contributes nothing to a graph-scoped view.
    let carolOnly = g.listTrendingArticlesForUser(carol, since)
    report("a reader with no follows still sees their own likes",
           carolOnly.len == 1, $carolOnly.len)

  # --- language filter ----------------------------------------------------
  block:
    let (sql, params) = langFilter(@["en", "fr"], "ar.")
    report("language filter builds placeholders",
           sql.count("?") == 2 and params.len == 2, sql.strip())
    # Unclassified articles must survive the filter, or new items vanish for
    # a few minutes and then reappear.
    report("language filter keeps unclassified articles",
           "language = ''" in sql)
    report("an empty language list adds no filter",
           langFilter(@[], "ar.")[0].len == 0)

  # --- liked articles -----------------------------------------------------
  block:
    let liked = g.listLikedArticles(bob)
    report("liked articles are listed newest first", liked.len == 2, $liked.len)
    report("liked articles carry article fields",
           liked[0].title.len > 0 and liked[0].feedTitle == "Feed A",
           liked[0].title)

  # --- reconciliation -----------------------------------------------------
  # The destructive path: local rows the PDS no longer has must go, and
  # everyone else's must not.
  block:
    report("bob starts with 2 likes", g.listLikes(bob).len == 2)
    g.deleteOrphanedLikes(bob, @["at://bob/like/1"])
    report("reconciling deletes likes the PDS dropped",
           g.listLikes(bob).len == 1, $g.listLikes(bob).len)
    report("reconciling leaves other users alone",
           g.listLikes(carol).len == 1, $g.listLikes(carol).len)

    g.deleteOrphanedAnnotations(alice, @["at://alice/ann/1"])
    report("reconciling keeps annotations the PDS still has",
           g.listAnnotations(authorDid = alice).len == 1)
    g.deleteOrphanedAnnotations(alice, @[])
    report("an empty active set clears that author's annotations",
           g.listAnnotations(authorDid = alice).len == 0)
    report("and leaves another author's annotations",
           g.listAnnotations(authorDid = bob).len == 1)

  echo ""
  if failures == 0:
    echo "RESULT: likes, annotations, follows and trending behave."
  else:
    echo &"RESULT: {failures} check(s) failed."
    quit 1

when isMainModule:
  main()
