## Checks the feed parser against crafted fixtures and real feeds.
##
##   nim c -r -d:ssl feedparser_probe.nim
##
## The fixtures cover the fallbacks, which is where feeds actually differ
## from the spec: a missing guid, a self link listed first, a named timezone,
## content in the wrong element. The live fetch is a reminder that feeds in
## the wild are stranger than any fixture.

import std/[httpclient, options, sequtils, strformat, strutils]
import reader/feedparser

var failures = 0

proc report(name: string, ok: bool, detail = "") =
  if not ok: inc failures
  let label = if ok: "PASS" else: "FAIL"
  if detail.len > 0: echo &"  {label}  {name}  -- {detail}"
  else: echo &"  {label}  {name}"

const rssFixture = """<?xml version="1.0"?>
<rss version="2.0" xmlns:content="http://purl.org/rss/1.0/modules/content/"
     xmlns:dc="http://purl.org/dc/elements/1.1/">
  <channel>
    <title>Example Feed</title>
    <link>https://example.test</link>
    <description>A description</description>
    <image><url>https://example.test/icon.png</url></image>
    <item>
      <title>With everything</title>
      <link>https://example.test/1</link>
      <guid isPermaLink="false">tag:example.test,2026:1</guid>
      <description>the summary</description>
      <content:encoded><![CDATA[<p>the body</p>]]></content:encoded>
      <author>alice@example.test</author>
      <pubDate>Mon, 15 Sep 2026 10:30:00 GMT</pubDate>
    </item>
    <item>
      <title>No guid, dc:creator</title>
      <link>https://example.test/2</link>
      <description>only a description</description>
      <dc:creator>Bob</dc:creator>
      <pubDate>Tue, 16 Sep 2026 08:00:00 -0500</pubDate>
    </item>
    <item>
      <title>Unparseable date</title>
      <link>https://example.test/3</link>
      <pubDate>sometime last tuesday</pubDate>
    </item>
  </channel>
</rss>"""

const atomFixture = """<?xml version="1.0" encoding="utf-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <title>Atom Example</title>
  <subtitle>the subtitle</subtitle>
  <icon>https://example.test/icon.png</icon>
  <link rel="self" href="https://example.test/feed.xml"/>
  <link rel="alternate" href="https://example.test/"/>
  <entry>
    <title>An entry</title>
    <id>urn:uuid:1225c695</id>
    <link rel="self" href="https://example.test/entry.atom"/>
    <link href="https://example.test/post"/>
    <summary>a summary</summary>
    <content type="html">&lt;p&gt;body&lt;/p&gt;</content>
    <author><name>Carol</name></author>
    <updated>2026-09-15T10:30:00Z</updated>
  </entry>
</feed>"""

const rdfFixture = """<?xml version="1.0"?>
<rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"
         xmlns="http://purl.org/rss/1.0/"
         xmlns:dc="http://purl.org/dc/elements/1.1/">
  <channel rdf:about="https://example.test/rdf">
    <title>RDF Example</title>
    <link>https://example.test</link>
    <description>old school</description>
  </channel>
  <item rdf:about="https://example.test/rdf/1">
    <title>An RDF item</title>
    <link>https://example.test/rdf/1</link>
    <description>summary here</description>
    <dc:creator>Dave</dc:creator>
    <dc:date>2026-09-15T10:30:00Z</dc:date>
  </item>
</rdf:RDF>"""

const jsonFixture = """{
  "version": "https://jsonfeed.org/version/1.1",
  "title": "JSON Example",
  "home_page_url": "https://example.test",
  "description": "a json feed",
  "icon": "https://example.test/icon.png",
  "items": [
    {
      "id": "1",
      "url": "https://example.test/j1",
      "title": "A JSON item",
      "content_html": "<p>html body</p>",
      "summary": "the summary",
      "date_published": "2026-09-15T10:30:00Z",
      "authors": [{"name": "Erin"}]
    },
    {
      "id": "",
      "url": "https://example.test/j2",
      "title": "Text only, no id",
      "content_text": "plain body"
    }
  ]
}"""

proc main() =
  echo "Feed parser probe"
  echo ""

  # --- RSS ----------------------------------------------------------------
  block:
    let r = parseFeed(rssFixture, "https://example.test/feed.xml")
    report("RSS is detected", r.feed.kind == fkRss, $r.feed.kind)
    report("channel metadata is read",
           r.feed.title == "Example Feed" and
           r.feed.siteUrl == "https://example.test", r.feed.title)
    report("channel image becomes the favicon",
           r.feed.faviconUrl == "https://example.test/icon.png")
    report("all items are read", r.articles.len == 3, $r.articles.len)

    let a0 = r.articles[0]
    report("guid is preferred over the link",
           a0.guid == "tag:example.test,2026:1", a0.guid)
    # content:encoded is the body; description is the summary. Collapsing
    # them would give readers a truncated article.
    report("content:encoded becomes content",
           "the body" in a0.content, a0.content.strip())
    report("description stays the summary", a0.summary == "the summary")
    report("a GMT pubDate parses", a0.published.isSome,
           if a0.published.isSome: $a0.published.get else: "none")

    let a1 = r.articles[1]
    report("a missing guid falls back to the link",
           a1.guid == "https://example.test/2", a1.guid)
    report("dc:creator is used when author is absent", a1.author == "Bob",
           a1.author)
    report("a numeric-offset pubDate parses", a1.published.isSome)

    let a2 = r.articles[2]
    # Dropping the article over a bad date would be worse than showing it
    # undated.
    report("an unparseable date leaves the article intact",
           a2.title == "Unparseable date" and a2.published.isNone)

  # --- Atom ---------------------------------------------------------------
  block:
    let r = parseFeed(atomFixture, "https://example.test/feed.xml")
    report("Atom is detected", r.feed.kind == fkAtom, $r.feed.kind)
    # A feed whose first link is rel="self" would otherwise point at itself.
    report("rel=alternate wins over rel=self for the site URL",
           r.feed.siteUrl == "https://example.test/", r.feed.siteUrl)
    report("icon becomes the favicon",
           r.feed.faviconUrl == "https://example.test/icon.png")

    let e = r.articles[0]
    report("entry id becomes the guid", e.guid == "urn:uuid:1225c695", e.guid)
    report("a link with no rel counts as alternate",
           e.url == "https://example.test/post", e.url)
    report("nested author name is read", e.author == "Carol", e.author)
    report("escaped content is unescaped", "<p>body</p>" in e.content,
           e.content)
    # Atom requires updated but not published.
    report("updated stands in for a missing published date",
           e.published.isSome and e.published == e.updated)

  # --- RDF ----------------------------------------------------------------
  block:
    let r = parseFeed(rdfFixture, "https://example.test/rdf")
    report("RDF is detected", r.feed.kind == fkRdf, $r.feed.kind)
    report("RDF channel metadata is read", r.feed.title == "RDF Example")
    # In RDF the items are siblings of <channel>, not children.
    report("RDF items are found outside the channel", r.articles.len == 1,
           $r.articles.len)
    if r.articles.len > 0:
      report("rdf:about becomes the guid",
             r.articles[0].guid == "https://example.test/rdf/1",
             r.articles[0].guid)
      report("dc:date parses", r.articles[0].published.isSome)
      report("dc:creator is the author", r.articles[0].author == "Dave")

  # --- JSON Feed ----------------------------------------------------------
  block:
    let r = parseFeed(jsonFixture, "https://example.test/feed.json")
    report("JSON Feed is detected", r.feed.kind == fkJson, $r.feed.kind)
    report("JSON metadata is read", r.feed.title == "JSON Example")
    report("both items are read", r.articles.len == 2, $r.articles.len)
    report("content_html becomes content",
           "html body" in r.articles[0].content)
    report("authors[0].name is the author", r.articles[0].author == "Erin")
    report("content_text is a fallback for content",
           r.articles[1].content == "plain body", r.articles[1].content)
    report("an empty id falls back to the url",
           r.articles[1].guid == "https://example.test/j2",
           r.articles[1].guid)

  # --- malformed input ----------------------------------------------------
  block:
    proc rejects(body: string): bool =
      try:
        discard parseFeed(body, "https://x.test")
        false
      except FeedParseError:
        true
    report("empty input is rejected", rejects("   "))
    report("non-feed XML is rejected", rejects("<html><body>hi</body></html>"))
    report("broken XML is rejected", rejects("<rss><channel>"))
    report("broken JSON is rejected", rejects("{not json"))

  # --- real feeds ---------------------------------------------------------
  # Fixtures only contain what their author thought of. These are deliberately
  # a mix of generators and formats.
  block:
    const feeds = [
      "https://blog.rust-lang.org/feed.xml",
      "https://nim-lang.org/feed.xml",
      "https://lwn.net/headlines/rss",
      "https://news.ycombinator.com/rss",
    ]
    var parsed = 0
    for url in feeds:
      try:
        let client = newHttpClient(timeout = 20_000,
                                   userAgent = "pulseboard-nim-spike/0.1")
        defer: client.close()
        let body = client.getContent(url)
        let r = parseFeed(body, url)

        let withGuid = r.articles.countIt(it.guid.len > 0)
        let withTitle = r.articles.countIt(it.title.len > 0)
        let dated = r.articles.countIt(it.published.isSome)
        let bodied = r.articles.countIt(
          it.content.len > 0 or it.summary.len > 0)

        # A guid is what dedupes an article across fetches, so a feed where
        # any item lacks one would re-insert that item forever.
        let ok = r.articles.len > 0 and withGuid == r.articles.len and
                 withTitle == r.articles.len
        if ok: inc parsed
        report(url.split('/')[2], ok,
               &"{r.feed.kind}, {r.articles.len} items, " &
               &"{dated} dated, {bodied} with text")
      except CatchableError as e:
        report(url.split('/')[2], false, e.msg)
    report("every real feed parsed", parsed == feeds.len,
           &"{parsed}/{feeds.len}")

  echo ""
  if failures == 0:
    echo "RESULT: the feed parser handles all four formats."
  else:
    echo &"RESULT: {failures} check(s) failed."
    quit 1

when isMainModule:
  main()
