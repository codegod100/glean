## The similarity computations behind recommendations, ported from
## internal/cluster/jaccard.go and the pure helpers of scoring.go.
##
## Two kinds of similarity are precomputed on a schedule, because both are
## O(pairs) and neither needs to be fresh to the second:
##
##   feed_similarity   which feeds are subscribed to by the same people
##   user_similarity   which people subscribe to the same feeds
##
## Both are time-decayed. An overlap from three years ago says much less
## about what someone wants now than one from last week, and without decay a
## long-lived account's early subscriptions dominate its recommendations
## forever.
##
## The decay constant is `exp(-0.023 * days)`, a half-life of about a month
## (ln 2 / 0.023 ≈ 30). It is written inline in the SQL rather than named,
## the same way the Go implementation writes it.

import std/[algorithm, math, options, random]
import ./gleandb
import ./sqlite

type
  Config* = object
    followBoost*: float
    likesWeight*: float
    tagsWeight*: float
    descriptionWeight*: float

  SignalWeights* = object
    wSub*: float
    wLike*: float
    wTag*: float
    wSocial*: float
    wPop*: float
    wCategory*: float
    wContent*: float

  FeedRecommendation* = object
    feedUrl*: string
    title*: string
    siteUrl*: string
    description*: string
    subscriberCount*: int
    faviconUrl*: string
    score*: float

  PersonRecommendation* = object
    did*: string
    handle*: string
    displayName*: string
    avatarUrl*: string
    commonFeeds*: int
    commonLikes*: int
    commonTags*: int
    isFollowed*: bool
    score*: float

const
  ## Half-life of roughly 30 days: ln(2) / 0.023.
  DecayPerDay* = 0.023

proc defaultConfig*(): Config =
  Config(followBoost: 0.5, likesWeight: 0.3, tagsWeight: 0.2,
         descriptionWeight: 0.15)

proc defaultWeights*(): SignalWeights =
  SignalWeights(wSub: 1.0, wLike: 0.5, wTag: 0.3, wSocial: 0.7,
                wPop: 0.2, wCategory: 0.4, wContent: 0.4)

proc decay*(ageDays: float): float =
  ## The weight an event of this age still carries.
  ##
  ## Exposed so the decay can be checked directly; the queries compute it in
  ## SQL because pulling every like into the application to weight it would
  ## defeat the point of precomputing.
  exp(-DecayPerDay * ageDays)

# --- similarity ------------------------------------------------------------

## Both computations stage into a temp table and swap in a second
## transaction. The first pass is the expensive one and holds no lock on the
## live table; readers keep seeing the previous generation until the swap,
## rather than an empty table for the length of the computation.

proc computeFeedSimilarity*(g: GleanDb) =
  ## For each pair of feeds, how strongly their subscriber sets overlap.
  ##
  ## The numerator is a decayed count of shared subscribers and the
  ## denominator is the union of the two subscriber counts, which makes this
  ## a Jaccard index with recency folded into the intersection.
  g.db.transaction:
    g.db.exec """CREATE TEMP TABLE IF NOT EXISTS _feed_sim_staging (
      feed_a TEXT NOT NULL, feed_b TEXT NOT NULL, jaccard REAL NOT NULL,
      PRIMARY KEY (feed_a, feed_b))"""
    g.db.exec "DELETE FROM _feed_sim_staging"
    g.db.exec """
      INSERT INTO _feed_sim_staging (feed_a, feed_b, jaccard)
      SELECT feed_a, feed_b, jaccard FROM (
        SELECT
          s1.feed_url AS feed_a,
          s2.feed_url AS feed_b,
          SUM(EXP(-0.023 * CAST(julianday('now') -
                                julianday(MIN(s1.added_at, s2.added_at)) AS REAL)))
          / NULLIF(f1.subscriber_count + f2.subscriber_count
                   - CAST(COUNT(*) AS REAL), 0) AS jaccard
        FROM articles.subscriptions s1
        -- feed_url < feed_url keeps one row per unordered pair, and drops
        -- the self-pair without a separate predicate.
        JOIN articles.subscriptions s2
          ON s1.user_did = s2.user_did AND s1.feed_url < s2.feed_url
        JOIN articles.feeds f1 ON f1.feed_url = s1.feed_url
        JOIN articles.feeds f2 ON f2.feed_url = s2.feed_url
        WHERE s1.added_at IS NOT NULL AND s2.added_at IS NOT NULL
        GROUP BY s1.feed_url, s2.feed_url
      ) WHERE jaccard IS NOT NULL"""

  g.db.transaction:
    g.db.exec "DELETE FROM recs.feed_similarity"
    g.db.exec """INSERT INTO recs.feed_similarity (feed_a, feed_b, jaccard)
                 SELECT feed_a, feed_b, jaccard FROM _feed_sim_staging"""

proc computeUserSimilarity*(g: GleanDb) =
  ## For each pair of people, how strongly their subscriptions overlap.
  ##
  ## Undecayed, unlike feed similarity: a subscription is a standing choice
  ## rather than an event, so its age says little. Shared *likes* are decayed
  ## separately below, because those are events.
  g.db.transaction:
    g.db.exec """CREATE TEMP TABLE IF NOT EXISTS _user_sim_staging (
      user_a TEXT NOT NULL, user_b TEXT NOT NULL, jaccard REAL NOT NULL,
      common_feeds INT NOT NULL DEFAULT 0,
      common_likes INT NOT NULL DEFAULT 0,
      common_tags INT NOT NULL DEFAULT 0,
      PRIMARY KEY (user_a, user_b))"""
    g.db.exec "DELETE FROM _user_sim_staging"
    g.db.exec """
      INSERT INTO _user_sim_staging
        (user_a, user_b, jaccard, common_feeds, common_likes, common_tags)
      SELECT
        s1.user_did, s2.user_did,
        CAST(COUNT(*) AS REAL) / (
          (SELECT COUNT(*) FROM articles.subscriptions WHERE user_did = s1.user_did) +
          (SELECT COUNT(*) FROM articles.subscriptions WHERE user_did = s2.user_did) -
          CAST(COUNT(*) AS REAL)
        ),
        COUNT(*), 0, 0
      FROM articles.subscriptions s1
      JOIN articles.subscriptions s2
        ON s1.feed_url = s2.feed_url AND s1.user_did < s2.user_did
      GROUP BY s1.user_did, s2.user_did"""

    # Shared likes, decayed on both sides: two people who liked the same
    # thing years apart have less in common than two who liked it the same
    # week, and the product of the two decays expresses that.
    #
    # ROUND, not CAST AS INTEGER. The Go implementation truncates, and since
    # each decay factor is at most 1 their product is always just under it --
    # so a pair sharing one recent like sums to 0.9999 and stores 0. That
    # number is shown to readers as the reason for a recommendation ("N
    # shared likes"), so truncation makes a real overlap read as none.
    g.db.exec """CREATE TEMP TABLE IF NOT EXISTS _likes_overlap (
      user_a TEXT, user_b TEXT, common INT, PRIMARY KEY(user_a, user_b))"""
    g.db.exec "DELETE FROM _likes_overlap"
    g.db.exec """
      INSERT INTO _likes_overlap (user_a, user_b, common)
      SELECT l1.author_did, l2.author_did,
        CAST(ROUND(SUM(
            EXP(-0.023 * CAST(julianday('now') - julianday(l1.created_at) AS REAL))
          * EXP(-0.023 * CAST(julianday('now') - julianday(l2.created_at) AS REAL))
        )) AS INTEGER)
      FROM articles.likes l1
      JOIN articles.likes l2
        ON l1.feed_url = l2.feed_url AND l1.article_url = l2.article_url
       AND l1.author_did < l2.author_did
      WHERE l1.created_at IS NOT NULL AND l2.created_at IS NOT NULL
      GROUP BY l1.author_did, l2.author_did"""

    # A pair that shares likes but no feeds has no staging row yet, so this
    # inserts before it updates.
    g.db.exec """
      INSERT INTO _user_sim_staging (user_a, user_b, jaccard, common_feeds,
                                     common_likes, common_tags)
      SELECT o.user_a, o.user_b, 0, 0, o.common, 0
      FROM _likes_overlap o
      WHERE NOT EXISTS (SELECT 1 FROM _user_sim_staging s
                        WHERE s.user_a = o.user_a AND s.user_b = o.user_b)"""
    g.db.exec """
      UPDATE _user_sim_staging SET common_likes = (
        SELECT o.common FROM _likes_overlap o
        WHERE o.user_a = _user_sim_staging.user_a
          AND o.user_b = _user_sim_staging.user_b)
      WHERE EXISTS (SELECT 1 FROM _likes_overlap o
                    WHERE o.user_a = _user_sim_staging.user_a
                      AND o.user_b = _user_sim_staging.user_b)"""

  g.db.transaction:
    g.db.exec "DELETE FROM recs.user_similarity"
    g.db.exec """INSERT INTO recs.user_similarity
                   (user_a, user_b, jaccard, common_feeds, common_likes, common_tags)
                 SELECT user_a, user_b, jaccard, common_feeds, common_likes,
                        common_tags
                 FROM _user_sim_staging"""

proc feedSimilarity*(g: GleanDb, feedA, feedB: string): float =
  ## Stored pairs are ordered, so look up whichever way round the caller asks.
  let (a, b) = if feedA < feedB: (feedA, feedB) else: (feedB, feedA)
  let row = g.db.queryFirst(
    "SELECT jaccard FROM recs.feed_similarity WHERE feed_a = ? AND feed_b = ?",
    [p(a), p(b)], proc(r: Row): float = r.f(0))
  if row.isSome: row.get else: 0.0

proc userSimilarity*(g: GleanDb, userA, userB: string): float =
  let (a, b) = if userA < userB: (userA, userB) else: (userB, userA)
  let row = g.db.queryFirst(
    "SELECT jaccard FROM recs.user_similarity WHERE user_a = ? AND user_b = ?",
    [p(a), p(b)], proc(r: Row): float = r.f(0))
  if row.isSome: row.get else: 0.0

# --- scoring helpers -------------------------------------------------------

proc normalizeScores*(scores: var openArray[float]) =
  ## Rescale to 0..1.
  ##
  ## Scores from different signals are not comparable in magnitude -- a
  ## like-overlap sum and a log-popularity term live on unrelated scales --
  ## so they are normalised before being blended or ranked against each
  ## other. A single item, or a set where every score is equal, is left
  ## alone: there is no spread to stretch, and dividing by zero would turn
  ## the whole list into NaN.
  if scores.len < 2: return
  var lo = scores[0]
  var hi = scores[0]
  for s in scores:
    lo = min(lo, s)
    hi = max(hi, s)
  if hi == lo: return
  let span = hi - lo
  for i in 0 ..< scores.len:
    scores[i] = (scores[i] - lo) / span

proc normalizeFeedScores*(recs: var seq[FeedRecommendation]) =
  var s = newSeq[float](recs.len)
  for i, r in recs: s[i] = r.score
  normalizeScores(s)
  for i in 0 ..< recs.len: recs[i].score = s[i]

proc normalizePersonScores*(recs: var seq[PersonRecommendation]) =
  var s = newSeq[float](recs.len)
  for i, r in recs: s[i] = r.score
  normalizeScores(s)
  for i in 0 ..< recs.len: recs[i].score = s[i]

proc samplePeople*(pool: seq[PersonRecommendation], half: int,
                   rng: var Rand): seq[PersonRecommendation] =
  ## Up to `half` from inside the reader's network and `half` from outside.
  ##
  ## The split is the point: ranking people purely by similarity returns the
  ## reader's existing network, which they already have. Reserving half the
  ## slots for strangers is what makes this discovery rather than a mirror.
  ##
  ## Shuffled rather than taken in order so the same faces do not appear
  ## every time -- the pool changes far more slowly than the reader revisits.
  var inNet, outNet: seq[PersonRecommendation]
  for p in pool:
    if p.isFollowed: inNet.add p else: outNet.add p

  rng.shuffle(inNet)
  rng.shuffle(outNet)
  if inNet.len > half: inNet.setLen(half)
  if outNet.len > half: outNet.setLen(half)
  inNet & outNet

proc rankByScore*[T](items: var seq[T]) =
  ## Highest first, which is what every recommendation listing wants.
  items.sort(proc(a, b: T): int = cmp(b.score, a.score))
