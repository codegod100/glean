## OPML is the portable backup format for feed subscriptions.  Keep this
## deliberately small: outlines can be nested arbitrarily, but only entries
## with xmlUrl are subscriptions.

import std/[strutils, xmlparser, xmltree]

type OpmlFeed* = object
  url*: string
  title*: string
  siteUrl*: string
  description*: string
  category*: string

proc escapeXml(s: string): string =
  s.replace("&", "&amp;").replace("\"", "&quot;").replace("<", "&lt;")

proc label(n: XmlNode): string =
  for key in ["title", "text", "htmlUrl", "xmlUrl"]:
    let value = n.attr(key).strip()
    if value.len > 0: return value
  ""

proc collect(n: XmlNode, category: string, result: var seq[OpmlFeed]) =
  if n == nil or n.kind != xnElement: return
  if n.tag.toLowerAscii == "outline":
    let url = n.attr("xmlUrl").strip()
    if url.len > 0:
      result.add OpmlFeed(url: url, title: label(n), siteUrl: n.attr("htmlUrl").strip(),
                          description: n.attr("description").strip(), category: category)
    else:
      let nextCategory = label(n)
      for child in n: collect(child, nextCategory, result)
  else:
    for child in n: collect(child, category, result)

proc parseOpml*(source: string): seq[OpmlFeed] =
  ## Raises XmlError for malformed XML; callers return it as a 400.
  let root = parseXml(source)
  if root == nil or root.tag.toLowerAscii != "opml":
    raise newException(ValueError, "expected an OPML document")
  collect(root, "", result)

proc renderOpml*(feeds: openArray[OpmlFeed], title = "Pulseboard subscriptions"): string =
  result = "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<opml version=\"2.0\">\n  <head><title>" &
           escapeXml(title) & "</title></head>\n  <body>\n"
  var uncategorized: seq[OpmlFeed]
  var categories: seq[string]
  for feed in feeds:
    if feed.category.len == 0:
      uncategorized.add feed
    elif feed.category notin categories:
      categories.add feed.category
  proc line(feed: OpmlFeed, indent: string): string =
    result = indent & "<outline text=\"" & escapeXml(feed.title) & "\" title=\"" &
             escapeXml(feed.title) & "\" type=\"rss\" xmlUrl=\"" & escapeXml(feed.url) & "\""
    if feed.siteUrl.len > 0: result.add " htmlUrl=\"" & escapeXml(feed.siteUrl) & "\""
    if feed.description.len > 0: result.add " description=\"" & escapeXml(feed.description) & "\""
    result.add "/>\n"
  for feed in uncategorized: result.add line(feed, "    ")
  for category in categories:
    result.add "    <outline text=\"" & escapeXml(category) & "\" title=\"" & escapeXml(category) & "\">\n"
    for feed in feeds:
      if feed.category == category: result.add line(feed, "      ")
    result.add "    </outline>\n"
  result.add "  </body>\n</opml>\n"
