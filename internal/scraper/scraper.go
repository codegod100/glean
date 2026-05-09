package scraper

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	"pkg.rbrt.fr/glean/internal/httpclient"

	"golang.org/x/net/html"
	"golang.org/x/net/html/charset"
)

type Scraper struct {
	client     *http.Client
	logger     *slog.Logger
	archiveURL string
}

func New(logger *slog.Logger) *Scraper {
	return &Scraper{
		client: &http.Client{
			Timeout:   15 * time.Second,
			Transport: httpclient.NewTransport(),
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 10 {
					return fmt.Errorf("too many redirects")
				}
				return nil
			},
		},
		logger:     logger,
		archiveURL: "https://archive.is/newest/",
	}
}

func (s *Scraper) Scrape(ctx context.Context, articleURL string) (string, error) {
	content, err := s.scrapeDirect(ctx, articleURL)
	if err != nil {
		s.logger.Warn("direct scrape failed, trying archive.is", "error", err, "url", articleURL)
		return s.scrapeArchive(ctx, articleURL)
	}
	if content == "" {
		s.logger.Warn("direct scrape returned empty content, trying archive.is", "url", articleURL)
		return s.scrapeArchive(ctx, articleURL)
	}
	return content, nil
}

func (s *Scraper) scrapeDirect(ctx context.Context, articleURL string) (string, error) {
	resp, err := s.fetch(ctx, articleURL)
	if err != nil {
		return "", fmt.Errorf("fetching article: %w", err)
	}
	return extractContent(resp.Body, resp.ContentType)
}

func (s *Scraper) scrapeArchive(ctx context.Context, articleURL string) (string, error) {
	resp, err := s.fetch(ctx, s.archiveURL+articleURL)
	if err != nil {
		return "", fmt.Errorf("fetching from archive.is: %w", err)
	}
	return extractContent(resp.Body, resp.ContentType)
}

type fetchResult struct {
	Body        io.Reader
	ContentType string
}

func (s *Scraper) fetch(ctx context.Context, url string) (*fetchResult, error) {
	resp, err := s.doFetch(ctx, url)
	if err == nil {
		return resp, nil
	}

	var se *httpclient.StatusError
	if !errors.As(err, &se) || !httpclient.IsRetryable(se.StatusCode) {
		return nil, err
	}

	backoff := 2 * time.Second
	if se.RetryAfter > 0 {
		backoff = min(se.RetryAfter, 5*time.Second)
	}

	if err := httpclient.SleepWithContext(ctx, backoff); err != nil {
		return nil, err
	}

	return s.doFetch(ctx, url)
}

func (s *Scraper) doFetch(ctx context.Context, url string) (*fetchResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	httpclient.SetDefaultHeaders(req)
	req.Header.Set("Accept", httpclient.AcceptHTML)

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		data, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
		if err != nil {
			return nil, fmt.Errorf("reading body: %w", err)
		}
		return &fetchResult{
			Body:        bytes.NewReader(data),
			ContentType: resp.Header.Get("Content-Type"),
		}, nil
	}

	se := &httpclient.StatusError{StatusCode: resp.StatusCode}
	if resp.StatusCode == http.StatusTooManyRequests {
		se.RetryAfter = httpclient.ParseRetryAfter(resp.Header.Get("Retry-After"))
	}
	return nil, se
}

func extractContent(r io.Reader, contentType string) (string, error) {
	doc, err := parseHTML(r, contentType)
	if err != nil {
		return "", fmt.Errorf("parsing HTML: %w", err)
	}

	removeUnwanted(doc)

	if content := findArticle(doc); content != nil {
		return renderNode(content), nil
	}

	if content := findLargestTextNode(doc); content != nil {
		return renderNode(content), nil
	}

	return "", nil
}

func parseHTML(r io.Reader, contentType string) (*html.Node, error) {
	ct := contentType
	if ct == "" {
		ct = "text/html; charset=utf-8"
	} else if !strings.Contains(ct, "charset") {
		ct += "; charset=utf-8"
	}

	decoded, err := charset.NewReader(r, ct)
	if err != nil {
		return nil, fmt.Errorf("decoding body: %w", err)
	}

	return html.Parse(decoded)
}

func findArticle(doc *html.Node) *html.Node {
	for _, strategy := range articleFindStrategies {
		if node := strategy(doc); node != nil {
			return node
		}
	}
	return nil
}

var articleFindStrategies = []func(*html.Node) *html.Node{
	func(n *html.Node) *html.Node { return findByTag(n, "article") },
	func(n *html.Node) *html.Node { return findByRole(n, "main") },
	func(n *html.Node) *html.Node { return findByTag(n, "main") },
	func(n *html.Node) *html.Node { return findByAttr(n, "itemprop", "articleBody") },
	func(n *html.Node) *html.Node { return findByContentClass(n) },
}

func findByContentClass(root *html.Node) *html.Node {
	var best *html.Node
	bestLen := 0

	forEachElement(root, func(n *html.Node) {
		for _, attr := range n.Attr {
			if attr.Key != "class" && attr.Key != "id" {
				continue
			}
			val := strings.ToLower(attr.Val)
			if !matchesContentPattern(val) {
				continue
			}
			tl := textLength(n)
			if tl > bestLen && tl >= minContentLength {
				best = n
				bestLen = tl
			}
		}
	})
	return best
}

var contentPatterns = []string{
	"post-content", "entry-content", "article-content",
	"article-body", "article__body", "article__content",
	"story-body", "story-content", "content-body",
	"post-body", "post-entry", "entry-body",
	"blog-content", "blog-post", "wp-content",
	"main-content", "page-content", "body-content",
}

func matchesContentPattern(val string) bool {
	for _, p := range contentPatterns {
		if strings.Contains(val, p) {
			return true
		}
	}
	return false
}

var unwantedTags = map[string]bool{
	"script": true, "style": true, "nav": true, "header": true, "footer": true,
	"aside": true, "noscript": true, "iframe": true, "form": true, "svg": true,
	"button": true, "input": true, "textarea": true, "select": true,
}

var unwantedRoles = map[string]bool{
	"complementary": true, "banner": true, "contentinfo": true, "navigation": true,
}

var unwantedClassPatterns = []string{
	"comment", "sidebar", "advertisement", "ad-banner",
	"social-share", "share-button", "newsletter", "popup",
	"cookie", "paywall", "related-post", "related-article",
	"taboola", "outbrain", "disqus",
}

var unwantedIDPatterns = []string{
	"comment", "sidebar", "footer", "header", "nav", "disqus",
}

func removeUnwanted(n *html.Node) {
	var remove []*html.Node
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if isUnwanted(c) {
			remove = append(remove, c)
			continue
		}
		removeUnwanted(c)
	}
	for _, c := range remove {
		n.RemoveChild(c)
	}
}

func isUnwanted(n *html.Node) bool {
	if n.Type != html.ElementNode {
		return false
	}
	if unwantedTags[n.Data] {
		return true
	}
	for _, attr := range n.Attr {
		switch attr.Key {
		case "class":
			if containsAny(strings.ToLower(attr.Val), unwantedClassPatterns) {
				return true
			}
		case "id":
			if containsAny(strings.ToLower(attr.Val), unwantedIDPatterns) {
				return true
			}
		case "role":
			if unwantedRoles[strings.ToLower(attr.Val)] {
				return true
			}
		}
	}
	return false
}

func containsAny(s string, patterns []string) bool {
	return slices.ContainsFunc(patterns, func(p string) bool {
		return strings.Contains(s, p)
	})
}

const minContentLength = 200

func findLargestTextNode(root *html.Node) *html.Node {
	var best *html.Node
	bestLen := 0

	forEachElement(root, func(n *html.Node) {
		if n.Data != "div" && n.Data != "section" && n.Data != "td" {
			return
		}
		tl := textLength(n)
		if tl > bestLen && tl >= minContentLength {
			best = n
			bestLen = tl
		}
	})
	return best
}

func textLength(n *html.Node) int {
	total := 0
	forEachText(n, func(t string) {
		total += len(strings.TrimSpace(t))
	})
	return total
}

func forEachElement(n *html.Node, fn func(*html.Node)) {
	if n.Type == html.ElementNode {
		fn(n)
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		forEachElement(c, fn)
	}
}

func forEachText(n *html.Node, fn func(string)) {
	if n.Type == html.TextNode {
		fn(n.Data)
		return
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		forEachText(c, fn)
	}
}

func findByTag(n *html.Node, tag string) *html.Node {
	if n.Type == html.ElementNode && n.Data == tag {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if found := findByTag(c, tag); found != nil {
			return found
		}
	}
	return nil
}

func findByRole(n *html.Node, role string) *html.Node {
	return findByAttr(n, "role", role)
}

func findByAttr(n *html.Node, key, val string) *html.Node {
	if n.Type == html.ElementNode {
		for _, attr := range n.Attr {
			if attr.Key == key && attr.Val == val {
				return n
			}
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if found := findByAttr(c, key, val); found != nil {
			return found
		}
	}
	return nil
}

var voidElements = map[string]bool{
	"br": true, "hr": true, "img": true, "input": true, "meta": true,
	"link": true, "area": true, "base": true, "col": true, "embed": true,
	"source": true, "track": true, "wbr": true,
}

func renderNode(n *html.Node) string {
	var buf strings.Builder
	renderChildren(&buf, n)
	return strings.TrimSpace(buf.String())
}

func renderChildren(buf *strings.Builder, n *html.Node) {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		renderNodeInto(buf, c)
	}
}

func renderNodeInto(buf *strings.Builder, node *html.Node) {
	if node.Type == html.TextNode {
		buf.WriteString(node.Data)
		return
	}
	if node.Type != html.ElementNode {
		return
	}

	tag := node.Data

	if tag == "a" {
		renderLink(buf, node)
		return
	}
	if tag == "img" {
		renderImage(buf, node)
		return
	}
	if tag == "source" && !isMediaSource(node) {
		return
	}

	writeOpenTag(buf, node, filterTagAttrs(tag))
	renderChildren(buf, node)
	if !voidElements[tag] {
		buf.WriteString("</")
		buf.WriteString(tag)
		buf.WriteString(">")
	}
}

func renderLink(buf *strings.Builder, node *html.Node) {
	href := getAttr(node, "href")
	href = strings.TrimSpace(href)

	if isHTTPURL(href) {
		writeOpenTag(buf, node, filterTagAttrs("a"))
		renderChildren(buf, node)
		buf.WriteString("</a>")
		return
	}

	renderChildren(buf, node)
}

func renderImage(buf *strings.Builder, node *html.Node) {
	src := resolveImgSrc(node)
	if src == "" {
		return
	}

	buf.WriteString("<img")
	writeFilteredAttrs(buf, node, filterTagAttrs("img"))
	buf.WriteString(` src="`)
	buf.WriteString(html.EscapeString(src))
	buf.WriteString(`"`)

	if getAttr(node, "alt") == "" {
		buf.WriteString(` alt=""`)
	}
	if !hasDimensions(node) {
		buf.WriteString(` loading="lazy"`)
	}
	buf.WriteString(">")
}

func resolveImgSrc(node *html.Node) string {
	for _, key := range []string{"src", "data-src", "data-lazy-src"} {
		v := getAttr(node, key)
		if v != "" && isReachableURL(v) {
			return v
		}
	}
	return ""
}

func isReachableURL(s string) bool {
	return isHTTPURL(s) || strings.HasPrefix(s, "//")
}

func isHTTPURL(s string) bool {
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
}

func filterTagAttrs(tag string) func(string) bool {
	if tag == "img" {
		return func(key string) bool {
			return key == "alt" || key == "width" || key == "height" || key == "class"
		}
	}
	return defaultAttrFilter
}

var defaultAttrFilter = func() func(string) bool {
	allowed := map[string]bool{
		"href": true, "src": true, "alt": true, "title": true,
		"class": true, "id": true, "width": true, "height": true,
		"type": true, "controls": true, "preload": true, "poster": true,
		"cite": true, "datetime": true, "colspan": true, "rowspan": true,
		"loading": true, "decoding": true, "itemprop": true,
		"role": true, "aria-label": true, "aria-hidden": true,
	}
	return func(key string) bool { return allowed[key] }
}()

func writeOpenTag(buf *strings.Builder, node *html.Node, allow func(string) bool) {
	buf.WriteString("<")
	buf.WriteString(node.Data)
	writeFilteredAttrs(buf, node, allow)
	buf.WriteString(">")
}

func writeFilteredAttrs(buf *strings.Builder, node *html.Node, allow func(string) bool) {
	for _, attr := range node.Attr {
		if !allow(attr.Key) {
			continue
		}
		buf.WriteString(" ")
		buf.WriteString(attr.Key)
		buf.WriteString(`="`)
		buf.WriteString(html.EscapeString(attr.Val))
		buf.WriteString(`"`)
	}
}

func hasDimensions(node *html.Node) bool {
	w, h := getAttr(node, "width"), getAttr(node, "height")
	return w != "" && w != "0" && h != "" && h != "0"
}

func isMediaSource(node *html.Node) bool {
	t := getAttr(node, "type")
	return strings.HasPrefix(t, "video/") || strings.HasPrefix(t, "audio/")
}

func getAttr(n *html.Node, key string) string {
	for _, attr := range n.Attr {
		if attr.Key == key {
			return attr.Val
		}
	}
	return ""
}
