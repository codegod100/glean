## Checks the fetch retry policy.
##
##   nim c -r -d:ssl feedfetcher_probe.nim
##
## The policy is the whole point, so almost everything here runs against a
## scripted transport rather than the network: the cases that matter are a
## server returning 503 twice then succeeding, a 404 that must not be
## retried, and a 429 naming its own delay. None of those are reproducible
## against a real host on demand.

import std/[strformat, strutils, tables]
import atproto/[feedfetcher, feedparser]

var failures = 0

proc report(name: string, ok: bool, detail = "") =
  if not ok: inc failures
  let label = if ok: "PASS" else: "FAIL"
  if detail.len > 0: echo &"  {label}  {name}  -- {detail}"
  else: echo &"  {label}  {name}"

const goodFeed = """<?xml version="1.0"?>
<rss version="2.0"><channel><title>T</title><link>https://x.test</link>
<item><title>One</title><link>https://x.test/1</link></item>
</channel></rss>"""

# --- a transport that answers from a script --------------------------------

var attempts = 0
var slept: seq[int] = @[]

proc scripted(responses: seq[tuple[status: int, body, retryAfter: string]]): Transport =
  attempts = 0
  result = proc(url: string): tuple[status: int, body: string,
                                    retryAfter: string] {.gcsafe.} =
    {.cast(gcsafe).}:
      let i = min(attempts, responses.high)
      inc attempts
      result = responses[i]

proc failing(): Transport =
  attempts = 0
  result = proc(url: string): tuple[status: int, body: string,
                                    retryAfter: string] {.gcsafe.} =
    {.cast(gcsafe).}:
      inc attempts
    raise newException(IOError, "connection refused")

proc recordSleep(ms: int) {.gcsafe.} =
  ## The probe is single-threaded; the annotation is what the callback
  ## type demands, not a claim about concurrency.
  {.cast(gcsafe).}:
    slept.add ms

proc fastConfig(): FetchConfig =
  result = defaultConfig()
  result.baseDelayMs = 10        # the probe asserts the curve, not the wait

proc main() =
  echo "Feed fetcher probe"
  echo ""

  # --- the happy path -----------------------------------------------------
  block:
    slept = @[]
    let r = fetchFeed("https://x.test/feed", fastConfig(),
                      scripted(@[(200, goodFeed, "")]), recordSleep)
    report("a 200 parses", r.articles.len == 1, r.feed.title)
    report("success does not retry", attempts == 1, $attempts)
    report("success does not sleep", slept.len == 0)

  # --- transient failures -------------------------------------------------
  block:
    slept = @[]
    let r = fetchFeed("https://x.test/feed", fastConfig(),
                      scripted(@[(503, "down", ""), (503, "down", ""),
                                 (200, goodFeed, "")]), recordSleep)
    report("a 5xx is retried until it succeeds",
           r.articles.len == 1 and attempts == 3, &"{attempts} attempts")
    report("backoff doubles", slept == @[10, 20], $slept)

  block:
    slept = @[]
    var raised = false
    try:
      discard fetchFeed("https://x.test/feed", fastConfig(),
                        scripted(@[(500, "down", "")]), recordSleep)
    except FetchError as e:
      raised = true
      report("a persistent 5xx eventually gives up", e.status == 500, $e.status)
    report("it raised rather than returning empty", raised)
    # maxRetries=3 means four attempts and three waits.
    report("it stops after maxRetries", attempts == 4, &"{attempts} attempts")
    report("it waits between each", slept == @[10, 20, 40], $slept)

  # --- permanent failures -------------------------------------------------
  # The case the Go version gets wrong: a 404 is not going to change.
  block:
    slept = @[]
    var status = 0
    try:
      discard fetchFeed("https://x.test/feed", fastConfig(),
                        scripted(@[(404, "nope", "")]), recordSleep)
    except FetchError as e:
      status = e.status
    report("a 404 is not retried", attempts == 1, &"{attempts} attempts")
    report("a 404 reports its status", status == 404, $status)
    report("a 404 does not sleep", slept.len == 0)

  block:
    attempts = 0
    var status = -1
    try:
      discard fetchFeed("https://x.test/feed", fastConfig(),
                        scripted(@[(403, "denied", "")]), recordSleep)
    except FetchError as e:
      status = e.status
    report("a 403 is not retried", attempts == 1 and status == 403,
           &"{attempts} attempts, status {status}")

  # An error page that is not a feed must fail as a parse error, not be
  # retried: the bytes are the same every time.
  block:
    slept = @[]
    var msg = ""
    try:
      discard fetchFeed("https://x.test/feed", fastConfig(),
                        scripted(@[(200, "<html>not a feed</html>", "")]),
                        recordSleep)
    except FetchError as e:
      msg = e.msg
    report("an unparseable 200 is not retried", attempts == 1,
           &"{attempts} attempts")
    report("and reports a parse failure", "parsing" in msg, msg.split(':')[0])

  # --- rate limiting ------------------------------------------------------
  block:
    slept = @[]
    discard fetchFeed("https://x.test/feed", fastConfig(),
                      scripted(@[(429, "slow down", "2"), (200, goodFeed, "")]),
                      recordSleep)
    report("a 429 is retried", attempts == 2, &"{attempts} attempts")
    # The server's own number beats our curve.
    report("Retry-After overrides the backoff curve", slept == @[2000], $slept)

  block:
    slept = @[]
    discard fetchFeed("https://x.test/feed", fastConfig(),
                      scripted(@[(429, "slow", "3600"), (200, goodFeed, "")]),
                      recordSleep)
    # A server asking for an hour must not hold a worker for one.
    report("Retry-After is capped", slept == @[10_000], $slept)

  block:
    slept = @[]
    discard fetchFeed("https://x.test/feed", fastConfig(),
                      scripted(@[(429, "slow", ""), (200, goodFeed, "")]),
                      recordSleep)
    report("a 429 without Retry-After falls back to the curve",
           slept == @[10], $slept)

  # --- transport errors ---------------------------------------------------
  block:
    slept = @[]
    var raised = false
    try:
      discard fetchFeed("https://x.test/feed", fastConfig(), failing(),
                        recordSleep)
    except FetchError:
      raised = true
    # A connection that never completed says nothing about the feed, so it
    # is worth retrying.
    report("a connection failure is retried", attempts == 4,
           &"{attempts} attempts")
    report("and eventually raises", raised)

  # --- Retry-After parsing ------------------------------------------------
  block:
    report("Retry-After seconds parse", parseRetryAfter("120") == 120_000,
           $parseRetryAfter("120"))
    report("an absent Retry-After is zero", parseRetryAfter("") == 0)
    report("a nonsense Retry-After is zero", parseRetryAfter("soon") == 0)
    # An HTTP-date in the past means "now", not a negative wait.
    report("a past Retry-After date clamps to zero",
           parseRetryAfter("Wed, 01 Jan 2020 00:00:00 GMT") == 0)

  block:
    report("429 is retryable", isRetryable(429))
    report("503 is retryable", isRetryable(503))
    report("404 is not retryable", not isRetryable(404))
    report("200 is not retryable", not isRetryable(200))

  # --- discovery fallback -------------------------------------------------
  block:
    var served = initTable[string, tuple[status: int, body, retryAfter: string]]()
    served["https://site.test"] = (404, "no feed here", "")
    served["https://site.test/rss"] = (200, goodFeed, "")
    let tr: Transport = proc(url: string): tuple[status: int, body: string,
                                                 retryAfter: string] {.gcsafe.} =
      {.cast(gcsafe).}:
        result = if url in served: served[url] else: (404, "", "")

    let (res, usedUrl) = fetchOrDiscover(
      "https://site.test", fastConfig(), tr,
      candidates = @["https://site.test/atom", "https://site.test/rss"])
    report("discovery falls through to a working candidate",
           res.articles.len == 1, usedUrl)
    # The caller stores what worked, not what the user typed.
    report("it reports the URL that worked",
           usedUrl == "https://site.test/rss", usedUrl)

  # --- one real fetch -----------------------------------------------------
  block:
    try:
      let r = fetchFeed("https://blog.rust-lang.org/feed.xml")
      report("a real feed fetches over HTTP", r.articles.len > 0,
             &"{r.feed.kind}, {r.articles.len} articles")
    except CatchableError as e:
      report("a real feed fetches over HTTP", false, e.msg)

  echo ""
  if failures == 0:
    echo "RESULT: the fetcher retries what it should and nothing else."
  else:
    echo &"RESULT: {failures} check(s) failed."
    quit 1

when isMainModule:
  main()
