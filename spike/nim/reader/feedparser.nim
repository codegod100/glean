## Parsing RSS, RDF, Atom and JSON Feed, ported from internal/feed/parser.go.
##
## Feeds in the wild are not well-formed in the way a spec implies. The rules
## that matter here are the fallbacks rather than the happy paths: a missing
## GUID falls back to the link, a missing date stays empty rather than
## becoming "now", and an unparseable date does not discard the article.
##
## Namespaces are handled by suffix rather than resolution. Go's XML decoder
## resolves `content:encoded` to its namespace URI; Nim's keeps the literal
## prefixed name, and feeds use prefixes inconsistently enough (`content:`,
## `media:`, occasionally none) that matching the local name is both simpler
## and more forgiving.

import std/[json, options, strutils, times, xmltree, xmlparser]

type
  FeedKind* = enum
    fkRss = "rss"
    fkAtom = "atom"
    fkRdf = "rdf"
    fkJson = "json"

  ParsedFeed* = object
    url*: string
    title*: string
    siteUrl*: string
    description*: string
    kind*: FeedKind
    faviconUrl*: string

  ParsedArticle* = object
    feedUrl*: string
    guid*: string
    title*: string
    url*: string
    author*: string
    content*: string
    summary*: string
    published*: Option[DateTime]
    updated*: Option[DateTime]

  ParseResult* = object
    feed*: ParsedFeed
    articles*: seq[ParsedArticle]

  FeedParseError* = object of CatchableError

# --- dates -----------------------------------------------------------------

const timeFormats = [
  "yyyy-MM-dd'T'HH:mm:sszzz",        # RFC3339
  "yyyy-MM-dd'T'HH:mm:ss'Z'",
  "yyyy-MM-dd'T'HH:mm:ss'.'fffzzz",
  "yyyy-MM-dd'T'HH:mm:ss'.'fff'Z'",
  # ZZZ accepts "-0500", zzz requires "-05:00"; feeds use both.
  "ddd, dd MMM yyyy HH:mm:ss ZZZ",   # RFC1123Z / RFC822
  "ddd, d MMM yyyy HH:mm:ss ZZZ",
  "ddd, dd MMM yyyy HH:mm:ss zzz",
  "ddd, d MMM yyyy HH:mm:ss zzz",
  "yyyy-MM-dd HH:mm:ss",
  "yyyy-MM-dd",
]

## Named zones that turn up in RSS pubDate. Nim's parser has no table for
## them, and dropping the date entirely over a timezone abbreviation would
## lose the ordering of every article in the feed.
const namedZones = {
  "GMT": 0, "UTC": 0, "UT": 0, "Z": 0,
  "EST": -5, "EDT": -4, "CST": -6, "CDT": -5,
  "MST": -7, "MDT": -6, "PST": -8, "PDT": -7,
}

proc normaliseZone(s: string): string =
  ## Rewrite a trailing named zone into the numeric offset Nim can parse.
  result = s.strip()
  for (name, hours) in namedZones:
    if result.endsWith(" " & name):
      let sign = if hours < 0: "-" else: "+"
      let h = abs(hours)
      return result[0 ..< result.len - name.len] &
             sign & align($h, 2, '0') & ":00"

proc parseFeedTime*(raw: string): Option[DateTime] =
  ## Best-effort. An unparseable date is not an error -- plenty of feeds ship
  ## malformed ones, and dropping the article over it would be worse than
  ## showing it undated.
  let s = normaliseZone(raw)
  if s.len == 0: return none(DateTime)
  for fmt in timeFormats:
    try:
      return some(parse(s, fmt, utc()))
    except TimeParseError, TimeFormatParseError:
      continue
  none(DateTime)

# --- XML helpers -----------------------------------------------------------

proc localName(tag: string): string =
  ## "content:encoded" -> "encoded". Feeds disagree about prefixes, and the
  ## local name is what actually identifies the element.
  let i = tag.rfind(':')
  if i < 0: tag else: tag[i + 1 .. ^1]

proc child(n: XmlNode, name: string): Option[XmlNode] =
  if n == nil or n.kind != xnElement: return none(XmlNode)
  for c in n:
    if c.kind == xnElement and localName(c.tag) == name:
      return some(c)
  none(XmlNode)

iterator children(n: XmlNode, name: string): XmlNode =
  if n != nil and n.kind == xnElement:
    for c in n:
      if c.kind == xnElement and localName(c.tag) == name:
        yield c

proc deepText(n: XmlNode): string =
  ## Text content including CDATA sections.
  ##
  ## xmltree's `innerText` skips xnCData entirely, which would be a quiet
  ## disaster here: RSS ships article bodies inside CDATA precisely because
  ## they contain HTML, so relying on innerText loses the content of most
  ## real feeds while the surrounding fields parse perfectly.
  case n.kind
  of xnText, xnCData: result = n.text
  of xnElement:
    for c in n: result.add deepText(c)
  else: discard

proc text(n: XmlNode, name: string): string =
  let c = child(n, name)
  if c.isSome: deepText(c.get).strip() else: ""

proc attrOr(n: XmlNode, name: string, fallback = ""): string =
  if n == nil or n.kind != xnElement: return fallback
  let v = n.attr(name)
  if v.len > 0: v else: fallback

proc cleanFavicon(url: string): string =
  let u = url.strip()
  if u.startsWith("http://") or u.startsWith("https://"): u else: ""

proc pickAtomLink(entry: XmlNode): string =
  ## Prefer rel="alternate" (or no rel, which means alternate), then fall
  ## back to the first link. A feed whose first link is rel="self" would
  ## otherwise point every article at the feed itself.
  var first = ""
  for l in children(entry, "link"):
    let href = l.attr("href")
    if href.len == 0: continue
    if first.len == 0: first = href
    let rel = l.attr("rel")
    if rel.len == 0 or rel == "alternate":
      return href
  first

# --- RSS -------------------------------------------------------------------

proc convertRss(root: XmlNode, feedUrl: string): ParseResult =
  let channel = child(root, "channel").get(newElement("channel"))
  result.feed = ParsedFeed(
    url: feedUrl,
    title: text(channel, "title"),
    siteUrl: text(channel, "link"),
    description: text(channel, "description"),
    kind: fkRss,
    faviconUrl: cleanFavicon(
      (let img = child(channel, "image"); if img.isSome: text(img.get, "url") else: "")),
  )

  for item in children(channel, "item"):
    var a = ParsedArticle(
      feedUrl: feedUrl,
      guid: text(item, "guid"),
      title: text(item, "title"),
      url: text(item, "link"),
      # content:encoded is the full body; description is the summary. Feeds
      # that only ship one put it in description.
      content: text(item, "encoded"),
      summary: text(item, "description"),
      author: text(item, "author"),
      published: parseFeedTime(text(item, "pubDate")),
    )
    if a.author.len == 0:
      a.author = text(item, "creator")   # dc:creator
    if a.guid.len == 0:
      a.guid = a.url
    result.articles.add a

# --- RDF (RSS 1.0) ---------------------------------------------------------

proc convertRdf(root: XmlNode, feedUrl: string): ParseResult =
  let channel = child(root, "channel").get(newElement("channel"))
  result.feed = ParsedFeed(
    url: feedUrl,
    title: text(channel, "title"),
    siteUrl: text(channel, "link"),
    description: text(channel, "description"),
    kind: fkRdf,
  )

  # In RDF the items are siblings of <channel>, not inside it.
  for item in children(root, "item"):
    var a = ParsedArticle(
      feedUrl: feedUrl,
      title: text(item, "title"),
      url: text(item, "link"),
      content: text(item, "encoded"),
      summary: text(item, "description"),
      author: text(item, "creator"),
      published: parseFeedTime(text(item, "date")),
    )
    # RDF items are identified by rdf:about rather than a guid element.
    a.guid = attrOr(item, "rdf:about", attrOr(item, "about", a.url))
    if a.guid.len == 0:
      a.guid = a.url
    result.articles.add a

# --- Atom ------------------------------------------------------------------

proc convertAtom(root: XmlNode, feedUrl: string): ParseResult =
  var favicon = text(root, "icon")
  if favicon.len == 0:
    favicon = text(root, "logo")

  result.feed = ParsedFeed(
    url: feedUrl,
    title: text(root, "title"),
    siteUrl: pickAtomLink(root),
    description: text(root, "subtitle"),
    kind: fkAtom,
    faviconUrl: cleanFavicon(favicon),
  )

  for entry in children(root, "entry"):
    var a = ParsedArticle(
      feedUrl: feedUrl,
      guid: text(entry, "id"),
      title: text(entry, "title"),
      url: pickAtomLink(entry),
      content: text(entry, "content"),
      summary: text(entry, "summary"),
      published: parseFeedTime(text(entry, "published")),
      updated: parseFeedTime(text(entry, "updated")),
    )
    let author = child(entry, "author")
    if author.isSome:
      a.author = text(author.get, "name")
    if a.guid.len == 0:
      a.guid = a.url
    # Atom requires <updated> but not <published>; treating updated as the
    # publication date keeps such feeds orderable.
    if a.published.isNone:
      a.published = a.updated
    result.articles.add a

# --- JSON Feed -------------------------------------------------------------

proc convertJson(doc: JsonNode, feedUrl: string): ParseResult =
  result.feed = ParsedFeed(
    url: feedUrl,
    title: doc{"title"}.getStr,
    siteUrl: doc{"home_page_url"}.getStr,
    description: doc{"description"}.getStr,
    kind: fkJson,
    faviconUrl: cleanFavicon(doc{"icon"}.getStr),
  )

  for item in doc{"items"}.getElems:
    var a = ParsedArticle(
      feedUrl: feedUrl,
      guid: item{"id"}.getStr,
      title: item{"title"}.getStr,
      url: item{"url"}.getStr,
      content: item{"content_html"}.getStr,
      summary: item{"summary"}.getStr,
      published: parseFeedTime(item{"date_published"}.getStr),
      updated: parseFeedTime(item{"date_modified"}.getStr),
    )
    if a.content.len == 0:
      a.content = item{"content_text"}.getStr
    let author = item{"author"}
    if author != nil and author.kind == JObject:
      a.author = author{"name"}.getStr
    if a.author.len == 0:
      let authors = item{"authors"}.getElems
      if authors.len > 0:
        a.author = authors[0]{"name"}.getStr
    if a.guid.len == 0:
      a.guid = a.url
    result.articles.add a

# --- entry point -----------------------------------------------------------

proc parseFeed*(body, feedUrl: string): ParseResult =
  ## Sniff the format and parse. The shape of the document decides, not the
  ## Content-Type header: servers mislabel feeds constantly, and a feed
  ## rejected for its header is indistinguishable to a reader from a feed
  ## that is broken.
  let trimmed = body.strip()
  if trimmed.len == 0:
    raise newException(FeedParseError, "empty document")

  if trimmed[0] == '{':
    let doc =
      try: parseJson(trimmed)
      except CatchableError:
        raise newException(FeedParseError, "not valid JSON")
    return convertJson(doc, feedUrl)

  let root =
    try: parseXml(trimmed)
    except CatchableError as e:
      raise newException(FeedParseError, "not valid XML: " & e.msg)

  case localName(root.tag)
  of "rss": convertRss(root, feedUrl)
  of "feed": convertAtom(root, feedUrl)
  of "RDF": convertRdf(root, feedUrl)
  else:
    raise newException(FeedParseError,
      "unrecognised feed root element: " & root.tag)
