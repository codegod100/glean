## Checks the similarity computations and scoring helpers.
##
##   nim c -r -d:ssl cluster_probe.nim
##
## Recommendations are the one part of this system with no obviously correct
## answer, so the checks are about properties rather than values: that decay
## actually decays, that a pair is counted once, that normalisation does not
## produce NaN, that discovery is not just a mirror of who you already follow.

import std/[math, options, os, random, sequtils, strformat, strutils, times]
import atproto/[articlestore, cluster, feedstore, gleandb, socialstore, sqlite]

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

proc main() =
  echo "Clustering probe"
  echo ""

  # --- pure helpers -------------------------------------------------------
  block:
    report("decay at zero days is 1", abs(decay(0.0) - 1.0) < 1e-12)
    # ln(2)/0.023 is about 30 days, so a month-old event is worth half.
    report("decay halves in about a month",
           abs(decay(30.1) - 0.5) < 0.01, &"{decay(30.1):.4f}")
    report("decay keeps shrinking", decay(365.0) < decay(30.0))
    report("decay never reaches zero", decay(3650.0) > 0.0,
           &"{decay(3650.0):.3e}")

  block:
    var s = @[0.0, 5.0, 10.0]
    normalizeScores(s)
    report("normalisation maps to 0..1", s == @[0.0, 0.5, 1.0], $s)

    # The case that would produce NaN: no spread to divide by.
    var flat = @[3.0, 3.0, 3.0]
    normalizeScores(flat)
    report("an all-equal set is left alone", flat == @[3.0, 3.0, 3.0], $flat)
    for v in flat:
      report("and produces no NaN", not v.isNaN)

    var one = @[7.0]
    normalizeScores(one)
    report("a single score is left alone", one == @[7.0])

    var empty: seq[float] = @[]
    normalizeScores(empty)
    report("an empty set does not crash", empty.len == 0)

    var negatives = @[-10.0, -5.0, 0.0]
    normalizeScores(negatives)
    report("negative scores normalise too", negatives == @[0.0, 0.5, 1.0],
           $negatives)

  block:
    var rng = initRand(42)
    var pool: seq[PersonRecommendation]
    for i in 0 ..< 10:
      pool.add PersonRecommendation(did: &"did:plc:in{i}", isFollowed: true)
    for i in 0 ..< 10:
      pool.add PersonRecommendation(did: &"did:plc:out{i}", isFollowed: false)

    let sampled = samplePeople(pool, 3, rng)
    let followed = sampled.countIt(it.isFollowed)
    let strangers = sampled.len - followed
    report("sampling caps each side at the half", sampled.len == 6,
           $sampled.len)
    # Ranking purely by similarity returns the network the reader already
    # has; reserving half the slots is what makes this discovery.
    report("half the slots go to people not followed",
           followed == 3 and strangers == 3,
           &"{followed} followed, {strangers} strangers")

    # Fewer than `half` on one side must not pad from the other.
    var lopsided: seq[PersonRecommendation]
    lopsided.add PersonRecommendation(did: "did:plc:only", isFollowed: true)
    for i in 0 ..< 10:
      lopsided.add PersonRecommendation(did: &"did:plc:s{i}", isFollowed: false)
    let small = samplePeople(lopsided, 3, rng)
    report("a thin side is not padded from the other",
           small.countIt(it.isFollowed) == 1 and small.len == 4, $small.len)

    # Shuffled, so the same faces do not recur every visit.
    var rngA = initRand(1)
    var rngB = initRand(2)
    let a = samplePeople(pool, 3, rngA).mapIt(it.did).join(",")
    let b = samplePeople(pool, 3, rngB).mapIt(it.did).join(",")
    report("sampling varies between runs", a != b)

  block:
    var recs = @[
      FeedRecommendation(feedUrl: "c", score: 0.1),
      FeedRecommendation(feedUrl: "a", score: 0.9),
      FeedRecommendation(feedUrl: "b", score: 0.5),
    ]
    rankByScore(recs)
    report("ranking puts the highest score first",
           recs.mapIt(it.feedUrl) == @["a", "b", "c"],
           recs.mapIt(it.feedUrl).join(","))

  # --- similarity against a real database ---------------------------------
  let base = getTempDir() / "glean_cluster_probe"
  cleanup(base)
  defer: cleanup(base)

  var g = gleandb.open(base)
  defer: g.close()
  g.migrate()

  for did in [alice, bob, carol]:
    g.db.run("INSERT INTO users (did) VALUES (?)", p(did))

  # alice and bob share two feeds; carol shares one with each.
  for i in 1 .. 3:
    g.upsertFeed(Feed(feedUrl: &"https://f{i}.test/feed", title: &"Feed {i}",
                      feedType: "rss"))
  g.createSubscription(alice, "https://f1.test/feed", "", "", "", "")
  g.createSubscription(alice, "https://f2.test/feed", "", "", "", "")
  g.createSubscription(bob, "https://f1.test/feed", "", "", "", "")
  g.createSubscription(bob, "https://f2.test/feed", "", "", "", "")
  g.createSubscription(carol, "https://f1.test/feed", "", "", "", "")
  g.createSubscription(carol, "https://f3.test/feed", "", "", "", "")

  block:
    g.computeUserSimilarity()
    let ab = g.userSimilarity(alice, bob)
    let ac = g.userSimilarity(alice, carol)
    # alice/bob share 2 of 2; alice/carol share 1 of 3.
    report("identical subscribers score 1.0", abs(ab - 1.0) < 1e-9, &"{ab:.4f}")
    report("partial overlap scores lower",
           abs(ac - (1.0 / 3.0)) < 1e-9, &"{ac:.4f}")
    report("similarity is symmetric on lookup",
           abs(g.userSimilarity(bob, alice) - ab) < 1e-12)
    report("an unrelated pair scores zero",
           g.userSimilarity(alice, "did:plc:nobody") == 0.0)

    # One row per unordered pair, or every score would be counted twice
    # downstream.
    let rows = g.db.queryInt("SELECT count(*) FROM recs.user_similarity").get(0)
    report("each pair is stored once", rows == 3, &"{rows} rows")

  block:
    # Recomputing must replace rather than accumulate.
    g.computeUserSimilarity()
    let rows = g.db.queryInt("SELECT count(*) FROM recs.user_similarity").get(0)
    report("recomputing replaces the previous generation", rows == 3,
           &"{rows} rows")

  block:
    g.computeFeedSimilarity()
    let f12 = g.feedSimilarity("https://f1.test/feed", "https://f2.test/feed")
    let f13 = g.feedSimilarity("https://f1.test/feed", "https://f3.test/feed")
    # f1 and f2 share alice and bob; f1 and f3 share only carol.
    report("feeds with more shared subscribers score higher", f12 > f13,
           &"f1/f2={f12:.4f} f1/f3={f13:.4f}")
    let reversed = g.feedSimilarity("https://f2.test/feed", "https://f1.test/feed")
    report("feed similarity is symmetric on lookup", abs(reversed - f12) < 1e-12)
    report("an unshared pair is absent",
           g.feedSimilarity("https://f2.test/feed", "https://f3.test/feed") == 0.0)

  # --- decay in the query, not just in Nim --------------------------------
  block:
    # Two pairs of people who liked the same articles, one recently and one
    # long ago. The old overlap must weigh less -- that is the whole reason
    # EXP is registered as a SQL function.
    g.upsertFeed(Feed(feedUrl: "https://n.test/feed", title: "News"))
    let fresh = now().utc
    let stale = now().utc - initDuration(days = 400)
    for (who, at, n) in [(alice, fresh, "r"), (bob, fresh, "r"),
                         (carol, stale, "s")]:
      g.createLike(Like(uri: &"at://{who}/like/{n}", authorDid: who,
                        feedUrl: "https://n.test/feed",
                        articleUrl: &"https://n.test/{n}",
                        createdAt: some(at)))
    # carol needs a partner on the stale article for a pair to exist.
    g.createLike(Like(uri: "at://dave/like/s", authorDid: "did:plc:dave",
                      feedUrl: "https://n.test/feed",
                      articleUrl: "https://n.test/s",
                      createdAt: some(stale)))

    g.computeUserSimilarity()
    proc overlapOf(a, b: string): float =
      let (x, y) = if a < b: (a, b) else: (b, a)
      g.db.queryFirst(
        "SELECT common_likes FROM recs.user_similarity WHERE user_a = ? AND user_b = ?",
        [p(x), p(y)], proc(r: Row): float = r.f(0)).get(-1.0)

    let recent = overlapOf(alice, bob)
    let old = overlapOf(carol, "did:plc:dave")
    # The check that was vacuous before: both sides were 0 because the Go
    # query truncates a decayed sum that is always just under 1.
    report("a recent shared like counts as one", recent == 1.0, &"{recent}")
    report("a 400-day-old shared like decays to nothing", old == 0.0, &"{old}")
    report("decay makes old overlap weigh less than new", recent > old,
           &"recent={recent} old={old}")

  echo ""
  if failures == 0:
    echo "RESULT: similarity and scoring behave."
  else:
    echo &"RESULT: {failures} check(s) failed."
    quit 1

when isMainModule:
  main()
