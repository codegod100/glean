## A small RSS reader.
##
##   nim c -r -d:ssl glean.nim
##   GLEAN_DB=~/.local/share/glean/db GLEAN_PORT=8080 ./glean
##
## Single user, no accounts, no auth. It runs on your machine and reads your
## feeds. That assumption is what keeps this a few hundred lines instead of a
## few thousand: no sessions, no CSRF, no per-user scoping, no ATProto.
##
## The pieces underneath -- fetching, parsing, storage, search, full-text
## extraction -- are the same modules the larger port built and tested.

import std/[asyncdispatch, asynchttpserver, httpclient, json, options, os,
            strformat, strutils, uri]
import atproto/[articlestore, feedfetcher, feedparser, feedstore, gleandb,
                scraper, sqlite]

const
  ## Everything is stored per-user underneath because the schema came from a
  ## multi-user server. One constant user keeps that plumbing satisfied
  ## without inventing accounts.
  LocalUser = "local"
  DefaultPort = 8080

type Reader = ref object
  db: GleanDb

proc jsonResponse(req: Request, code: HttpCode, node: JsonNode) {.async.} =
  await req.respond(code, $node, newHttpHeaders({
    "Content-Type": "application/json",
    # The reader may be opened from a local web page or a native client.
    "Access-Control-Allow-Origin": "*",
  }))

proc fail(req: Request, code: HttpCode, msg: string) {.async.} =
  await jsonResponse(req, code, %*{"error": msg})

proc toJson(a: Article): JsonNode =
  %*{
    "id": a.id,
    "feed_url": a.feedUrl,
    "feed_title": a.feedTitle,
    "title": a.title,
    "url": a.url,
    "author": a.author,
    "summary": a.summary,
    "content": (if a.fullContent.len > 0: a.fullContent else: a.content),
    "published": (if a.published.isSome: $a.published.get else: ""),
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

  case req.reqMethod
  of HttpGet:
    case segs[0]
    of "", "health":
      await jsonResponse(req, Http200, %*{"ok": true})

    of "feeds":
      var items = newJArray()
      for s in r.db.listSubscriptions(LocalUser, "", 500, 0):
        items.add toJson(s, s.unreadCount)
      await jsonResponse(req, Http200, items)

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
      await fail(req, Http404, "not found")

  of HttpPost:
    case segs[0]
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
                                   userAgent = "glean/0.1")
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
  let dbPath = getEnv("GLEAN_DB", getHomeDir() / ".local/share/glean/glean")
  createDir(dbPath.parentDir)
  let port = try: parseInt(getEnv("GLEAN_PORT", $DefaultPort))
             except ValueError: DefaultPort

  var db = gleandb.open(dbPath)
  db.migrate()
  db.db.run("INSERT OR IGNORE INTO users (did) VALUES (?)", p(LocalUser))
  let reader = Reader(db: db)

  echo &"glean reading from {dbPath}"
  echo &"listening on http://127.0.0.1:{port}"

  var server = newAsyncHttpServer()
  # Single-threaded server, single Reader: the cast is asserting that, not
  # claiming the Reader is safe to share across threads.
  # Single-threaded server with one Reader. The cast asserts that, rather
  # than claiming the modules underneath are thread-safe -- they are not, and
  # do not need to be.
  proc cb(req: Request) {.async, gcsafe.} =
    {.cast(gcsafe).}:
      await reader.handle(req)
  await server.serve(Port(port), cb, address = "127.0.0.1")

when isMainModule:
  waitFor main()
