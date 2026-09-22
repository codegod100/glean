## Extracting an article from a page, ported from internal/scraper/scraper.go.
##
## Feeds routinely ship a first paragraph and a "read more" link, so the
## reader view needs the page itself. This is a readability-style extractor:
## find the node most likely to be the article, strip the furniture around
## it, and re-render a small whitelisted subset of HTML.
##
## Two things are deliberate:
##
## * The output is sanitised HTML, not text. Paragraphs, headings, lists,
##   code blocks and images are what make a long article readable, and
##   flattening them to text loses more than the markup costs.
## * Nothing is trusted. Attributes are whitelisted rather than blacklisted,
##   and `script`, `style`, `iframe` and friends never survive, because this
##   renders HTML fetched from an arbitrary site into the reader's client.

import std/[htmlparser, options, strtabs, strutils, xmltree]

type
  ScrapeError* = object of CatchableError

const
  MinContentLength* = 200
    ## Below this a candidate is furniture, not an article.

  ## Ordered: the first that matches wins. Semantic markup beats guessing,
  ## so <article> and role=main come before class-name heuristics.
  ContentPatterns = [
    "post-content", "entry-content", "article-content",
    "article-body", "article__body", "article__content",
    "story-body", "story-content", "content-body",
    "post-body", "post-entry", "entry-body",
    "blog-content", "blog-post", "wp-content",
    "main-content", "page-content", "body-content",
  ]

  UnwantedTags = [
    "script", "style", "nav", "header", "footer", "aside", "noscript",
    "iframe", "form", "svg", "button", "input", "textarea", "select",
  ]

  UnwantedRoles = ["complementary", "banner", "contentinfo", "navigation"]

  UnwantedClassPatterns = [
    "comment", "sidebar", "advertisement", "ad-banner",
    "social-share", "share-button", "newsletter", "popup",
    "cookie", "paywall", "related-post", "related-article",
    "taboola", "outbrain", "disqus",
  ]

  ## Narrower than the class list: an id of "header" is reliably the page
  ## header, while a *class* of "header" is often the article's own.
  UnwantedIdPatterns = ["comment", "sidebar", "footer", "header", "nav", "disqus"]

  VoidElements = [
    "br", "hr", "img", "input", "meta", "link", "area", "base", "col",
    "embed", "source", "track", "wbr",
  ]

  AllowedAttrs = [
    "href", "src", "alt", "title", "class", "id", "width", "height",
    "type", "controls", "preload", "poster", "cite", "datetime",
    "colspan", "rowspan", "loading", "decoding", "itemprop",
    "role", "aria-label", "aria-hidden",
  ]

  ## Images keep only presentational attributes; `src` is resolved separately
  ## so that lazy-loading placeholders do not survive as broken images.
  ImgAllowedAttrs = ["alt", "width", "height", "class"]

proc attrOf(n: XmlNode, key: string): string =
  if n.kind != xnElement or n.attrs == nil: "" else: n.attrs.getOrDefault(key)

proc containsAny(s: string, patterns: openArray[string]): bool =
  let lower = s.toLowerAscii
  for p in patterns:
    if p in lower: return true
  false

iterator elements(n: XmlNode): XmlNode =
  ## Depth-first over element nodes, the root included.
  var stack = @[n]
  while stack.len > 0:
    let cur = stack.pop()
    if cur.kind == xnElement:
      yield cur
      for i in countdown(cur.len - 1, 0):
        stack.add cur[i]

proc textLength(n: XmlNode): int =
  case n.kind
  of xnText, xnCData: result = n.text.strip().len
  of xnElement:
    for c in n: result += textLength(c)
  else: discard

# --- finding the article ---------------------------------------------------

proc findByTag(root: XmlNode, tag: string): Option[XmlNode] =
  for el in elements(root):
    if el.tag == tag: return some(el)
  none(XmlNode)

proc findByAttr(root: XmlNode, key, value: string): Option[XmlNode] =
  for el in elements(root):
    if attrOf(el, key) == value: return some(el)
  none(XmlNode)

proc findByContentClass(root: XmlNode): Option[XmlNode] =
  ## The widest net, so it runs last: pick the largest element whose class or
  ## id looks like a content wrapper. Size is the tie-breaker because these
  ## patterns nest -- "main-content" often wraps "entry-content".
  var best: XmlNode
  var bestLen = 0
  for el in elements(root):
    for key in ["class", "id"]:
      let v = attrOf(el, key)
      if v.len == 0 or not containsAny(v, ContentPatterns): continue
      let tl = textLength(el)
      if tl > bestLen and tl >= MinContentLength:
        best = el
        bestLen = tl
  if best == nil: none(XmlNode) else: some(best)

proc findLargestTextNode(root: XmlNode): Option[XmlNode] =
  ## Last resort for pages with no semantic markup at all. Only div and
  ## section are considered: without that, the winner is always <body>.
  var best: XmlNode
  var bestLen = 0
  for el in elements(root):
    if el.tag notin ["div", "section"]: continue
    let tl = textLength(el)
    if tl > bestLen and tl >= MinContentLength:
      best = el
      bestLen = tl
  if best == nil: none(XmlNode) else: some(best)

proc findArticle*(doc: XmlNode): Option[XmlNode] =
  for strategy in [
    proc(n: XmlNode): Option[XmlNode] = findByTag(n, "article"),
    proc(n: XmlNode): Option[XmlNode] = findByAttr(n, "role", "main"),
    proc(n: XmlNode): Option[XmlNode] = findByTag(n, "main"),
    proc(n: XmlNode): Option[XmlNode] = findByAttr(n, "itemprop", "articleBody"),
    findByContentClass,
    findLargestTextNode,
  ]:
    let found = strategy(doc)
    if found.isSome: return found
  none(XmlNode)

# --- stripping furniture ---------------------------------------------------

proc isUnwanted*(n: XmlNode): bool =
  if n.kind != xnElement: return false
  if n.tag in UnwantedTags: return true
  if containsAny(attrOf(n, "class"), UnwantedClassPatterns): return true
  if containsAny(attrOf(n, "id"), UnwantedIdPatterns): return true
  if attrOf(n, "role").toLowerAscii in UnwantedRoles: return true
  false

proc removeUnwanted*(n: XmlNode) =
  ## Depth-first, back to front: deleting by index invalidates the indices
  ## after it, so walking forwards would skip a sibling after every removal.
  if n.kind != xnElement: return
  for i in countdown(n.len - 1, 0):
    if isUnwanted(n[i]):
      n.delete(i)
    else:
      removeUnwanted(n[i])

# --- rendering -------------------------------------------------------------

proc isHttpUrl(s: string): bool =
  s.startsWith("http://") or s.startsWith("https://")

proc isReachableUrl(s: string): bool =
  isHttpUrl(s) or s.startsWith("//")

proc resolveImgSrc(n: XmlNode): string =
  ## Lazy-loaded images put a placeholder in `src` and the real URL in a
  ## data attribute. Taking the first *reachable* one avoids rendering a
  ## 1x1 gif where the picture should be.
  for key in ["src", "data-src", "data-lazy-src"]:
    let v = attrOf(n, key)
    if v.len > 0 and isReachableUrl(v): return v
  ""

proc escapeText(s: string): string =
  for c in s:
    case c
    of '<': result.add "&lt;"
    of '>': result.add "&gt;"
    of '&': result.add "&amp;"
    else: result.add c

proc escapeAttr(s: string): string =
  escapeText(s).replace("\"", "&quot;")

proc allowedFor(tag, key: string): bool =
  if tag == "img": key in ImgAllowedAttrs else: key in AllowedAttrs

proc renderInto(buf: var string, n: XmlNode)

proc renderChildren(buf: var string, n: XmlNode) =
  for c in n:
    renderInto(buf, c)

proc renderInto(buf: var string, n: XmlNode) =
  case n.kind
  of xnText, xnCData:
    buf.add escapeText(n.text)
    return
  of xnElement: discard
  else:
    # Comments, entities and processing instructions carry nothing a reader
    # needs and are a needless way to smuggle markup through.
    return

  let tag = n.tag

  if tag == "a":
    let href = attrOf(n, "href")
    # A link that goes nowhere useful is rendered as its text, so the words
    # survive without an anchor pointing at the scraped site's internals.
    if not isReachableUrl(href):
      renderChildren(buf, n)
      return
    buf.add "<a href=\"" & escapeAttr(href) & "\" rel=\"noopener noreferrer\">"
    renderChildren(buf, n)
    buf.add "</a>"
    return

  if tag == "img":
    let src = resolveImgSrc(n)
    if src.len == 0: return
    buf.add "<img src=\"" & escapeAttr(src) & "\""
    if n.attrs != nil:
      for key, value in n.attrs:
        if allowedFor(tag, key):
          buf.add " " & key & "=\"" & escapeAttr(value) & "\""
    buf.add ">"
    return

  buf.add "<" & tag
  if n.attrs != nil:
    for key, value in n.attrs:
      if allowedFor(tag, key):
        buf.add " " & key & "=\"" & escapeAttr(value) & "\""
  buf.add ">"

  if tag in VoidElements:
    return
  renderChildren(buf, n)
  buf.add "</" & tag & ">"

proc renderNode*(n: XmlNode): string =
  renderChildren(result, n)
  result = result.strip()

# --- entry point -----------------------------------------------------------

proc sanitizeFragment*(html: string): string =
  ## Run arbitrary HTML through the same whitelist the extractor uses.
  ##
  ## Feed content needs this as much as scraped content does, and is easier
  ## to forget: an entry's `content:encoded` is attacker-controlled markup
  ## from a stranger's server, and it reaches the reader without ever passing
  ## through extractArticle. Rendering it raw is a script tag away from
  ## running in the reader's browser.
  ##
  ## Returns "" for input that will not parse, rather than raising: a feed
  ## with one malformed entry should lose that entry's body, not the article.
  if html.strip().len == 0: return ""
  let doc =
    try: parseHtml(html)
    except CatchableError: return ""
  removeUnwanted(doc)
  renderNode(doc)

proc extractArticle*(body: string): string =
  ## Pull the readable article out of a page.
  ##
  ## Raises rather than returning something thin: a caller that gets a nav
  ## menu back has no way to tell it apart from a short article, whereas an
  ## error lets it fall back to the feed's own summary.
  if body.strip().len == 0:
    raise newException(ScrapeError, "empty document")

  let doc =
    try: parseHtml(body)
    except CatchableError as e:
      raise newException(ScrapeError, "could not parse HTML: " & e.msg)

  let article = findArticle(doc)
  if article.isNone:
    raise newException(ScrapeError, "no article content found")

  let node = article.get
  removeUnwanted(node)
  result = renderNode(node)

  if textLength(node) < MinContentLength:
    raise newException(ScrapeError, "extracted content too short")
