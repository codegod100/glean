## Checks the article extractor.
##
##   nim c -r -d:ssl scraper_probe.nim
##
## Two things are being tested, and the second matters more. Extraction: does
## the right node win, and does the furniture go. Sanitisation: this renders
## HTML from an arbitrary site into a reader's client, so anything that could
## carry script must not survive -- and that has to be asserted, because a
## page that renders correctly proves nothing about what was stripped.

import std/[htmlparser, httpclient, options, strformat, strutils, xmltree]
import reader/[feedparser, scraper]

var failures = 0

proc report(name: string, ok: bool, detail = "") =
  if not ok: inc failures
  let label = if ok: "PASS" else: "FAIL"
  if detail.len > 0: echo &"  {label}  {name}  -- {detail}"
  else: echo &"  {label}  {name}"

# Long enough to clear MinContentLength.
const body = """<p>Lorem ipsum dolor sit amet, consectetur adipiscing elit,
sed do eiusmod tempor incididunt ut labore et dolore magna aliqua. Ut enim ad
minim veniam, quis nostrud exercitation ullamco laboris nisi ut aliquip ex ea
commodo consequat. Duis aute irure dolor in reprehenderit.</p>"""

proc page(inner: string): string =
  &"""<!DOCTYPE html><html><head><title>t</title></head><body>{inner}</body></html>"""

proc main() =
  echo "Scraper probe"
  echo ""

  # --- which node wins ----------------------------------------------------
  block:
    let html = page(&"""
      <nav>menu menu menu</nav>
      <article><h1>The headline</h1>{body}</article>
      <footer>footer text</footer>""")
    let got = extractArticle(html)
    report("<article> is preferred", "The headline" in got)
    report("nav is stripped", "menu menu" notin got)
    report("footer is stripped", "footer text" notin got)

  block:
    # Semantic markup should beat a class-name guess.
    let html = page(&"""
      <div class="post-content">wrong one, but long enough {body}</div>
      <main><h1>Main wins</h1>{body}</main>""")
    let got = extractArticle(html)
    report("role/main beats a content class", "Main wins" in got)
    report("the losing candidate is not included", "wrong one" notin got)

  block:
    let html = page(&"""<div role="main"><h1>By role</h1>{body}</div>""")
    report("role=main is found", "By role" in extractArticle(html))

  block:
    let html = page(&"""<div itemprop="articleBody"><h1>By itemprop</h1>{body}</div>""")
    report("itemprop=articleBody is found",
           "By itemprop" in extractArticle(html))

  block:
    let html = page(&"""<div class="entry-content"><h1>By class</h1>{body}</div>""")
    report("a content class is found", "By class" in extractArticle(html))

  block:
    # These patterns nest; the larger should win.
    let html = page(&"""
      <div class="main-content"><div class="entry-content">
        <h1>Inner</h1>{body}{body}
      </div></div>""")
    let got = extractArticle(html)
    report("the largest matching container wins", "Inner" in got)

  block:
    # No semantic markup at all.
    let html = page(&"""<div><h1>Largest</h1>{body}</div><div>tiny</div>""")
    report("it falls back to the largest text block",
           "Largest" in extractArticle(html))

  # --- furniture ----------------------------------------------------------
  block:
    let html = page(&"""<article>{body}
      <div class="related-articles">related junk</div>
      <div id="disqus_thread">comments here</div>
      <aside>aside junk</aside>
      <div role="complementary">complementary junk</div>
      <section class="newsletter">subscribe now</section>
    </article>""")
    let got = extractArticle(html)
    for junk in ["related junk", "comments here", "aside junk",
                 "complementary junk", "subscribe now"]:
      report(&"stripped: {junk}", junk notin got)

  block:
    # Removal is back-to-front for a reason: deleting by index shifts the
    # siblings after it, so a forward walk skips one after each removal.
    let html = page(&"""<article>
      <nav>one</nav><nav>two</nav><nav>three</nav><nav>four</nav>{body}
    </article>""")
    let got = extractArticle(html)
    report("consecutive removals do not skip siblings",
           "one" notin got and "two" notin got and
           "three" notin got and "four" notin got)

  # --- sanitisation -------------------------------------------------------
  block:
    let html = page(&"""<article>
      <script>alert('xss')</script>
      <style>body {{ color: red }}</style>
      <iframe src="https://evil.test"></iframe>
      <p onclick="steal()" data-tracking="123">clean paragraph</p>
      {body}
    </article>""")
    let got = extractArticle(html)
    report("script content never survives",
           "alert" notin got and "<script" notin got)
    report("style content never survives", "color: red" notin got)
    report("iframes never survive", "iframe" notin got and "evil.test" notin got)
    # Whitelisted, not blacklisted: an unknown attribute is dropped by
    # default rather than needing to be anticipated.
    report("event handlers are dropped", "onclick" notin got)
    report("unknown data attributes are dropped", "data-tracking" notin got)
    report("the text itself is kept", "clean paragraph" in got)

  block:
    let html = page(&"""<article>
      <a href="javascript:alert(1)">bad link</a>
      <a href="/relative">relative link</a>
      <a href="https://ok.test/x">good link</a>
      {body}</article>""")
    let got = extractArticle(html)
    report("javascript: URLs are not rendered as links",
           "javascript:" notin got)
    report("but their text survives", "bad link" in got)
    report("relative links are flattened to text",
           "relative link" in got and "\"/relative\"" notin got)
    report("absolute links are kept", "https://ok.test/x" in got)
    # An anchor into a scraped page should not hand the target window a
    # reference back.
    report("links carry rel=noopener", "noopener" in got)

  block:
    let html = page(&"""<article>
      <img src="data:image/gif;base64,R0lGOD" data-src="https://cdn.test/real.jpg" alt="real">
      <img src="https://cdn.test/plain.jpg" alt="plain">
      <img alt="no source at all">
      {body}</article>""")
    let got = extractArticle(html)
    # A placeholder rendered as-is is a broken image where the picture goes.
    report("a lazy-loaded image resolves to its real source",
           "cdn.test/real.jpg" in got and "data:image" notin got)
    report("a plain image is kept", "cdn.test/plain.jpg" in got)
    report("an image with no usable source is dropped",
           "no source at all" notin got)
    report("alt text is kept", "alt=\"real\"" in got)

  block:
    let html = page(&"""<article><p>a &lt;b&gt; &amp; c</p>{body}</article>""")
    let got = extractArticle(html)
    report("text is re-escaped, not passed through",
           "&lt;b&gt;" in got and "<b>" notin got, "")

  # --- structure is preserved ---------------------------------------------
  block:
    let html = page(&"""<article>
      <h2>A heading</h2>
      <ul><li>first</li><li>second</li></ul>
      <pre><code>let x = 1</code></pre>
      <blockquote>quoted</blockquote>
      {body}</article>""")
    let got = extractArticle(html)
    for tag in ["<h2>", "<ul>", "<li>", "<pre>", "<code>", "<blockquote>"]:
      report(&"structure kept: {tag}", tag in got)

  # --- refusal ------------------------------------------------------------
  block:
    proc refuses(html: string): bool =
      try:
        discard extractArticle(html)
        false
      except ScrapeError:
        true
    report("an empty document is refused", refuses("   "))
    # Returning a nav menu would be indistinguishable to the caller from a
    # short article; an error lets it fall back to the feed summary.
    report("a page with no article is refused",
           refuses(page("<nav>just a menu</nav>")))
    report("a too-short article is refused",
           refuses(page("<article><p>tiny</p></article>")))

  # --- a real page --------------------------------------------------------
  # The URL is taken from the feed rather than hard-coded, so this cannot rot
  # when a blog reorganises its permalinks.
  block:
    try:
      let client = newHttpClient(timeout = 20_000,
                                 userAgent = "pulseboard-nim-spike/0.1")
      defer: client.close()
      let feed = parseFeed(client.getContent("https://blog.rust-lang.org/feed.xml"),
                           "https://blog.rust-lang.org/feed.xml")
      if feed.articles.len == 0 or feed.articles[0].url.len == 0:
        echo "  SKIP  a real article page  -- feed had no article URLs"
      else:
        let url = feed.articles[0].url
        let got = extractArticle(client.getContent(url))
        report("a real article page extracts", got.len > 1000,
               &"{got.len} chars from {url.split('/')[^2]}")
        report("and carries no script", "<script" notin got)
        report("and keeps its paragraphs", "<p>" in got)
    except CatchableError as e:
      echo &"  SKIP  a real article page  -- {e.msg}"

  echo ""
  if failures == 0:
    echo "RESULT: the scraper extracts and sanitises."
  else:
    echo &"RESULT: {failures} check(s) failed."
    quit 1

when isMainModule:
  main()
