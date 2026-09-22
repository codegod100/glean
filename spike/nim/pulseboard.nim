## A small RSS reader.
##
##   nim c -r -d:ssl pulseboard.nim
##   PULSEBOARD_DB=~/.local/share/pulseboard/db PULSEBOARD_PORT=8080 ./pulseboard
##
## Single user, no accounts. It runs on your machine and reads your feeds,
## and that assumption is what keeps this a few hundred lines instead of a
## few thousand: no sessions, no CSRF, no per-user scoping, no ATProto.
##
## PULSEBOARD_TOKEN puts a shared secret in front of everything, which is what
## makes it safe anywhere but loopback -- `/feeds` and `/fetch-content` will
## fetch any URL handed to them, so an open instance is an SSRF proxy as well
## as someone else's feed list. Unset, there is no gate at all and the local
## case stays exactly as simple as it was.
##
## PULSEBOARD_WEB must point at the Flutter web build. The reader refuses to start
## without it so deployments cannot silently serve a different client.
##
## The pieces underneath -- fetching, parsing, storage, search, full-text
## extraction -- are the same modules the larger port built and tested.

import std/[asyncdispatch, asynchttpserver, httpclient, json, options, os,
            strformat, strutils, times, uri]
import reader/[articlestore, feedfetcher, feedparser, feedstore, pulseboarddb,
                opml, scraper, sqlite]

const
  ## Everything is stored per-user underneath because the schema came from a
  ## multi-user server. One constant user keeps that plumbing satisfied
  ## without inventing accounts.
  LocalUser = "local"
  DefaultPort = 8080

type Reader = ref object
  db: PulseboardDb
  token: string    ## empty disables the gate entirely
  webRoot: string  ## required Flutter web bundle

proc cookieValue(header, name: string): string =
  for part in header.split(';'):
    let kv = part.strip()
    let i = kv.find('=')
    if i > 0 and kv[0 ..< i] == name: return kv[i + 1 .. ^1]
  ""

proc constantTimeEq(a, b: string): bool =
  ## Compare without leaking where the first difference is. A check that
  ## returns early is an oracle for recovering the token a byte at a time.
  if a.len != b.len: return false
  var diff = 0'u8
  for i in 0 ..< a.len:
    diff = diff or (a[i].uint8 xor b[i].uint8)
  diff == 0

proc presentedToken(req: Request, queryKey: string): string =
  ## Header first, then ?key=, then the cookie ?key= sets.
  ##
  ## Three ways because three callers: a native client sends a header, a
  ## person opens a link, and the browser then has a cookie.
  let h = req.headers.getOrDefault("X-Pulseboard-Token").string
  if h.len > 0: return h
  if queryKey.len > 0: return queryKey
  cookieValue(req.headers.getOrDefault("Cookie").string, "pulseboard_token")

proc jsonResponse(req: Request, code: HttpCode, node: JsonNode) {.async.} =
  await req.respond(code, $node, newHttpHeaders({
    "Content-Type": "application/json",
    # The reader may be opened from a local web page or a native client.
    "Access-Control-Allow-Origin": "*",
  }))

proc fail(req: Request, code: HttpCode, msg: string) {.async.} =
  await jsonResponse(req, code, %*{"error": msg})

proc contentTypeFor(path: string): string =
  ## Enough types for a Flutter web build. A wrong type here is not cosmetic:
  ## a browser will refuse to execute JavaScript served as text/plain.
  let ext = path.splitFile.ext.toLowerAscii
  case ext
  of ".html": "text/html; charset=utf-8"
  of ".js", ".mjs": "text/javascript; charset=utf-8"
  of ".css": "text/css; charset=utf-8"
  of ".json": "application/json"
  of ".wasm": "application/wasm"
  of ".png": "image/png"
  of ".jpg", ".jpeg": "image/jpeg"
  of ".svg": "image/svg+xml"
  of ".ico": "image/x-icon"
  of ".woff2": "font/woff2"
  of ".woff": "font/woff"
  of ".ttf": "font/ttf"
  of ".otf": "font/otf"
  of ".map": "application/json"
  else: "application/octet-stream"

proc serveStatic(r: Reader, req: Request, rel: string): Future[bool] {.async.} =
  ## Serve `rel` from the web root, if it is there. Returns false when it is
  ## not, so the caller can fall through.
  if r.webRoot.len == 0: return false
  # Resolve and confirm the result is still inside the root: "../" in a
  # request path is how a static file server turns into a file server for
  # the whole disk.
  let full = absolutePath(r.webRoot / rel)
  let root = absolutePath(r.webRoot)
  if not full.startsWith(root) or not fileExists(full): return false
  var headers = newHttpHeaders({"Content-Type": contentTypeFor(full)})
  # Flutter's bootstrap and main bundle keep stable filenames. Revalidate
  # them so a Modal deploy cannot leave browsers running yesterday's client.
  let ext = full.splitFile.ext.toLowerAscii
  if ext in [".html", ".js", ".mjs"]:
    headers["Cache-Control"] = "no-cache"
  await req.respond(Http200, readFile(full), headers)
  true

proc serveApp(r: Reader, req: Request) {.async.} =
  if not await serveStatic(r, req, "index.html"):
    await fail(req, Http500, "Flutter web bundle is unavailable")

proc toJson(a: Article): JsonNode =
  %*{
    "id": a.id,
    "feed_url": a.feedUrl,
    "feed_title": a.feedTitle,
    "title": a.title,
    "url": a.url,
    "author": a.author,
    "summary": a.summary,
    # Sanitised on the way out, whichever source it came from. Scraped text
    # has already been through the whitelist, but feed content has not, and
    # it is markup from a stranger's server heading for a browser.
    "content": sanitizeFragment(
      if a.fullContent.len > 0: a.fullContent else: a.content),
    # Explicitly ISO 8601. Nim's `$` on a DateTime dumps the struct's fields,
    # which reaches the browser as "Invalid Date".
    "published": (if a.published.isSome:
                    a.published.get.format("yyyy-MM-dd'T'HH:mm:ss'Z'")
                  else: ""),
    "is_read": a.isRead,
  }

proc toJson(s: Subscription, unread: int): JsonNode =
  %*{
    "feed_url": s.feedUrl,
    "title": s.feedTitle,
    "category": s.category,
    "unread": unread,
    "favicon_url": s.faviconUrl,
  }

# --- feed refresh ----------------------------------------------------------

proc refreshFeed(r: Reader, feedUrl: string): tuple[added: int, err: string] =
  ## Fetch one feed and store what came back.
  ##
  ## A failure is recorded rather than raised: one unreachable feed should
  ## not stop a refresh of the others, and the error belongs on the feed so
  ## the reader can see which one is broken.
  try:
    let parsed = fetchFeed(feedUrl)
    var batch: seq[NewArticle]
    for a in parsed.articles:
      batch.add NewArticle(
        feedUrl: feedUrl, guid: a.guid, title: a.title, url: a.url,
        author: a.author, summary: a.summary, content: a.content,
        published: a.published, updated: a.updated)
    let before = r.db.getUnreadCount(LocalUser, feedUrl, "")
    r.db.batchUpsertArticles(batch)
    r.db.upsertFeed(Feed(feedUrl: feedUrl, title: parsed.feed.title,
                         siteUrl: parsed.feed.siteUrl,
                         description: parsed.feed.description,
                         feedType: $parsed.feed.kind,
                         faviconUrl: parsed.feed.faviconUrl))
    r.db.markFeedFetched(feedUrl)
    (r.db.getUnreadCount(LocalUser, feedUrl, "") - before, "")
  except CatchableError as e:
    r.db.markFeedFetchError(feedUrl, e.msg)
    (0, e.msg)

# --- routes ----------------------------------------------------------------

proc handle(r: Reader, req: Request) {.async.} =
  let path = req.url.path.strip(chars = {'/'})
  let segs = path.split('/')
  # Query string and form body both, the way Go's FormValue does: a caller
  # should not have to care which one a given route wanted.
  var q: seq[(string, string)] = @[]
  for kv in decodeQuery(req.url.query): q.add kv
  if req.body.len > 0:
    for kv in decodeQuery(req.body): q.add kv
  proc param(name: string, fallback = ""): string =
    for (k, v) in q:
      if k == name: return v
    fallback
  proc intParam(name: string, fallback: int): int =
    let raw = param(name)
    if raw.len == 0: fallback
    else:
      try: parseInt(raw) except ValueError: fallback

  # The gate, before anything reads the database or fetches a URL.
  if r.token.len > 0:
    if not constantTimeEq(presentedToken(req, param("key")), r.token):
      await fail(req, Http401, "unauthorized")
      return
    # Arriving with ?key= means a person followed a link; remember it so the
    # rest of the app's requests carry it without the token sitting in every
    # URL (and in the browser history, and in any Referer it sends).
    if param("key").len > 0:
      await req.respond(Http303, "", newHttpHeaders({
        "Location": "/",
        "Set-Cookie": "pulseboard_token=" & r.token &
                      "; Path=/; HttpOnly; SameSite=Lax; Max-Age=2592000",
      }))
      return

  case req.reqMethod
  of HttpGet:
    case segs[0]
    of "":
      await serveApp(r, req)
    of "health":
      await jsonResponse(req, Http200, %*{"ok": true})

    of "feeds":
      var items = newJArray()
      for s in r.db.listSubscriptions(LocalUser, "", 500, 0):
        items.add toJson(s, s.unreadCount)
      await jsonResponse(req, Http200, items)

    of "opml":
      var feeds: seq[OpmlFeed]
      for s in r.db.listSubscriptions(LocalUser, "", 5000, 0):
        feeds.add OpmlFeed(url: s.feedUrl, title: s.feedTitle, category: s.category)
      await req.respond(Http200, renderOpml(feeds), newHttpHeaders({
        "Content-Type": "application/xml; charset=utf-8",
        "Content-Disposition": "attachment; filename=pulseboard-subscriptions.opml",
      }))

    of "articles":
      if segs.len == 2:
        # One article, with its full text if we have it.
        let id = try: parseInt(segs[1]) except ValueError: 0
        let a = r.db.getArticle(LocalUser, id)
        if a.isNone:
          await fail(req, Http404, "no such article")
        else:
          await jsonResponse(req, Http200, toJson(a.get))
        return

      let search = param("q")
      let articles =
        if search.len > 0:
          r.db.searchArticles(LocalUser, search, intParam("limit", 50), 0)
        else:
          r.db.listArticles(
            LocalUser,
            filter = (if param("status") == "all": afAll
                      elif param("status") == "read": afRead
                      else: afUnread),
            feedUrl = param("feed"),
            limit = intParam("limit", 50))
      var items = newJArray()
      for a in articles: items.add toJson(a)
      await jsonResponse(req, Http200, items)

    of "unread":
      await jsonResponse(req, Http200,
                         %*{"count": r.db.getUnreadCount(LocalUser, "", "")})
    else:
      # Not an API route: it may be an asset of the web client.
      if not await serveStatic(r, req, path):
        await fail(req, Http404, "not found")

  of HttpPost:
    case segs[0]
    of "opml":
      let source = param("opml")
      if source.len == 0:
        await fail(req, Http400, "OPML required")
        return
      if source.len > 1_000_000:
        await fail(req, Http413, "OPML is too large")
        return
      var imported: seq[OpmlFeed]
      try:
        imported = parseOpml(source)
      except CatchableError as e:
        await fail(req, Http400, "invalid OPML: " & e.msg)
        return
      var added = 0
      var errors = newJArray()
      for feed in imported:
        let exists = r.db.getSubscription(LocalUser, feed.url).isSome
        r.db.upsertFeed(Feed(feedUrl: feed.url, title: feed.title,
                             siteUrl: feed.siteUrl, description: feed.description))
        r.db.createSubscription(LocalUser, feed.url, feed.title, feed.category, "", "")
        if not exists: inc added
        let (_, err) = r.refreshFeed(feed.url)
        if err.len > 0: errors.add %*{"feed_url": feed.url, "error": err}
      await jsonResponse(req, Http200,
                         %*{"imported": imported.len, "added": added, "errors": errors})

    of "feeds":
      let url = param("url")
      if url.len == 0:
        await fail(req, Http400, "url required")
        return
      # Subscribe first so a feed that fails to fetch is still listed, with
      # its error visible, rather than vanishing.
      r.db.upsertFeed(Feed(feedUrl: url, title: url))
      r.db.createSubscription(LocalUser, url, "", param("category"), "", "")
      let (added, err) = r.refreshFeed(url)
      if err.len > 0:
        await jsonResponse(req, Http200,
                           %*{"feed_url": url, "added": 0, "error": err})
      else:
        await jsonResponse(req, Http200, %*{"feed_url": url, "added": added})

    of "refresh":
      var total = 0
      var errors = newJArray()
      for s in r.db.listSubscriptions(LocalUser, "", 500, 0):
        let (added, err) = r.refreshFeed(s.feedUrl)
        total += added
        if err.len > 0:
          errors.add %*{"feed_url": s.feedUrl, "error": err}
      await jsonResponse(req, Http200, %*{"added": total, "errors": errors})

    of "read":
      let id = intParam("id", 0)
      if id == 0:
        await fail(req, Http400, "id required")
        return
      if param("undo") == "1":
        r.db.markArticleUnread(LocalUser, id)
      else:
        r.db.markArticleRead(LocalUser, id)
      await jsonResponse(req, Http200, %*{"id": id})

    of "read-all":
      r.db.markAllRead(LocalUser, param("feed"))
      await jsonResponse(req, Http200,
                         %*{"unread": r.db.getUnreadCount(LocalUser, "", "")})

    of "fetch-content":
      # Most feeds ship an excerpt; this pulls the real article.
      let id = intParam("id", 0)
      let a = r.db.getArticle(LocalUser, id)
      if a.isNone or a.get.url.len == 0:
        await fail(req, Http404, "no such article")
        return
      try:
        let client = newHttpClient(timeout = 20_000,
                                   userAgent = "pulseboard/0.1")
        defer: client.close()
        let text = extractArticle(client.getContent(a.get.url))
        r.db.updateArticleFullContent(id, text)
        await jsonResponse(req, Http200, %*{"id": id, "content": text})
      except CatchableError as e:
        await fail(req, Http502, e.msg)
    else:
      await fail(req, Http404, "not found")

  of HttpDelete:
    if segs[0] == "feeds" and param("url").len > 0:
      r.db.deleteSubscription(LocalUser, param("url"))
      await jsonResponse(req, Http200, %*{"ok": true})
    else:
      await fail(req, Http400, "url required")

  else:
    await fail(req, Http405, "method not allowed")

proc main() {.async.} =
  let dbPath = getEnv("PULSEBOARD_DB", getHomeDir() / ".local/share/pulseboard/pulseboard")
  createDir(dbPath.parentDir)
  let port = try: parseInt(getEnv("PULSEBOARD_PORT", $DefaultPort))
             except ValueError: DefaultPort

  var db = pulseboarddb.open(dbPath)
  db.migrate()
  db.db.run("INSERT OR IGNORE INTO users (did) VALUES (?)", p(LocalUser))

  let token = getEnv("PULSEBOARD_TOKEN")
  let webRoot = getEnv("PULSEBOARD_WEB")
  if webRoot.len == 0:
    quit("PULSEBOARD_WEB is required and must point at a Flutter web build")
  if not dirExists(webRoot) or not fileExists(webRoot / "index.html"):
    quit(&"PULSEBOARD_WEB points at {webRoot}, which has no index.html")

  let reader = Reader(db: db, token: token, webRoot: webRoot)

  echo &"pulseboard reading from {dbPath}"
  if webRoot.len > 0: echo &"serving the client from {webRoot}"
  if token.len > 0:
    echo "token required: append ?key=… once to set the cookie"
  else:
    # Worth saying out loud, because the difference between this and an open
    # instance on a public address is the whole of the security model.
    echo "no token set — anyone who can reach this port has full access"
  echo &"open http://127.0.0.1:{port}"

  var server = newAsyncHttpServer()
  # Single-threaded server, single Reader: the cast is asserting that, not
  # claiming the Reader is safe to share across threads.
  # Single-threaded server with one Reader. The cast asserts that, rather
  # than claiming the modules underneath are thread-safe -- they are not, and
  # do not need to be.
  proc cb(req: Request) {.async, gcsafe.} =
    {.cast(gcsafe).}:
      await reader.handle(req)
  # Loopback unless a token guards it. Modal and other containers need a
  # listener on all interfaces, and requiring the token for that is what
  # keeps "reachable" from meaning "open".
  let address = if token.len > 0: "0.0.0.0" else: "127.0.0.1"
  await server.serve(Port(port), cb, address = address)

when isMainModule:
  waitFor main()
