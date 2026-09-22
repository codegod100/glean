## Fetching feeds over HTTP, ported from internal/feed/fetcher.go.
##
## Retries exist for servers that are briefly unwell -- 429 and 5xx -- and
## for nothing else. A 404 will still be a 404 in one second, and a feed that
## does not parse will not parse differently on a second read, so both fail
## immediately.
##
## ## A divergence from the Go implementation, deliberately
##
## The Go fetcher's retry policy does not work as written.
## `executeRequest` returns its `*http.Response` only on success, so in
## `Fetch` the guard
##
##     if resp != nil && !httpclient.IsRetryable(resp.StatusCode)
##
## can never be true when `err != nil`: the response is always nil on the
## error path. Three things follow. Every failure retries the full four
## attempts regardless of status, so a permanently-404 feed costs four
## requests per refresh. `retryBackoff`'s Retry-After handling is likewise
## unreachable, because `lastResp` is only non-nil once the function has
## already returned. And the HTTP status is never checked at all, so an error
## page is handed to the parser and fails there instead.
##
## This port checks the status, honours Retry-After, and retries only what is
## worth retrying. It is a behaviour change, not a translation, which is why
## it is written down here rather than left to be discovered.

import std/[httpclient, os, strutils, times]
import ./feedparser

type
  FetchError* = object of CatchableError
    status*: int          ## 0 when the request never got a response
    retryable*: bool

  FetchConfig* = object
    timeoutMs*: int
    maxRetries*: int
    baseDelayMs*: int
    maxRetryAfterMs*: int
    userAgent*: string

  ## Injectable so the probe can drive the retry logic without a network, and
  ## so at:// feeds can be served by the ATProto client instead.
  Transport* = proc(url: string): tuple[status: int, body: string,
                                        retryAfter: string] {.gcsafe.}

const
  AcceptFeed* = "application/xml,application/atom+xml,application/rss+xml," &
                "application/rdf+xml,application/feed+json,text/html;q=0.9"
  DefaultUserAgent* = "pulseboard-nim/0.1"

proc defaultConfig*(): FetchConfig =
  FetchConfig(
    timeoutMs: 10_000,
    maxRetries: 3,
    baseDelayMs: 1000,
    maxRetryAfterMs: 10_000,
    userAgent: DefaultUserAgent,
  )

proc isRetryable*(status: int): bool =
  ## 429 and 5xx only. Everything else is the server's settled opinion.
  status == 429 or status >= 500

proc parseRetryAfter*(value: string): int =
  ## Milliseconds, or 0 when absent or unparseable.
  ##
  ## Retry-After is either delta-seconds or an HTTP date; both are in the
  ## wild, and a server that sends one is telling us something more useful
  ## than our own backoff curve.
  let v = value.strip()
  if v.len == 0: return 0
  try:
    return parseInt(v) * 1000
  except ValueError:
    discard
  for fmt in ["ddd, dd MMM yyyy HH:mm:ss 'GMT'",
              "ddd, dd MMM yyyy HH:mm:ss zzz"]:
    try:
      let target = parse(v, fmt, utc())
      let delta = (target - now().utc).inMilliseconds.int
      return max(delta, 0)
    except TimeParseError, TimeFormatParseError:
      continue
  0

proc backoffMs*(cfg: FetchConfig, attempt: int, retryAfter: string,
                status: int): int =
  ## Exponential, unless the server named a delay on a 429.
  if status == 429:
    let named = parseRetryAfter(retryAfter)
    if named > 0:
      # Capped: a server asking for an hour should not hold a worker for one.
      return min(named, cfg.maxRetryAfterMs)
  cfg.baseDelayMs * (1 shl max(attempt - 1, 0))

proc httpTransport*(cfg: FetchConfig): Transport =
  result = proc(url: string): tuple[status: int, body: string,
                                    retryAfter: string] {.gcsafe.} =
    let client = newHttpClient(userAgent = cfg.userAgent,
                               timeout = cfg.timeoutMs,
                               maxRedirects = 5)
    defer: client.close()
    client.headers = newHttpHeaders({"Accept": AcceptFeed})
    let res = client.get(url)
    (res.code.int, res.body, res.headers.getOrDefault("Retry-After").string)

proc raiseFetch(status: int, msg: string) =
  var e = newException(FetchError, msg)
  e.status = status
  e.retryable = isRetryable(status)
  raise e

proc fetchFeed*(url: string, cfg = defaultConfig(),
                transport: Transport = nil,
                sleep: proc(ms: int) {.gcsafe.} = nil): ParseResult =
  ## Fetch and parse, retrying only what is worth retrying.
  let tr = if transport != nil: transport else: httpTransport(cfg)
  proc defaultSleep(ms: int) {.gcsafe.} = os.sleep(ms)
  let nap = if sleep != nil: sleep else: defaultSleep

  var lastMsg = "feed fetch failed"
  var lastStatus = 0

  for attempt in 0 .. cfg.maxRetries:
    var status = 0
    var body = ""
    var retryAfter = ""
    var transportFailed = false

    try:
      (status, body, retryAfter) = tr(url)
    except CatchableError as e:
      # A connection that never completed: worth retrying, since it says
      # nothing about whether the feed exists.
      transportFailed = true
      lastMsg = "fetching " & url & ": " & e.msg
      lastStatus = 0

    if not transportFailed:
      if status >= 200 and status < 300:
        try:
          return parseFeed(body, url)
        except FeedParseError as e:
          # Not retryable: the bytes will be the same next time.
          raiseFetch(status, "parsing " & url & ": " & e.msg)
      lastStatus = status
      lastMsg = "fetching " & url & ": HTTP " & $status
      if not isRetryable(status):
        raiseFetch(status, lastMsg)

    if attempt == cfg.maxRetries:
      break
    nap(backoffMs(cfg, attempt + 1, retryAfter, lastStatus))

  raiseFetch(lastStatus, lastMsg)

proc isAtProtoFeed*(url: string): bool = url.startsWith("at://")

proc fetchOrDiscover*(url: string, cfg = defaultConfig(),
                      transport: Transport = nil,
                      candidates: seq[string] = @[]): (ParseResult, string) =
  ## Fetch `url`; failing that, try feeds discovered at it.
  ##
  ## Returns which URL actually worked, because the caller stores that rather
  ## than what the user typed -- people paste a site's home page far more
  ## often than its feed.
  try:
    return (fetchFeed(url, cfg, transport), url)
  except FetchError:
    if isAtProtoFeed(url) or candidates.len == 0:
      raise
  for candidate in candidates:
    try:
      return (fetchFeed(candidate, cfg, transport), candidate)
    except FetchError:
      continue
  raiseFetch(0, "no working feed found at " & url)
  (ParseResult(), url)   # unreachable; satisfies the return type
