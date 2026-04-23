package scraper

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"pkg.rbrt.fr/glean/internal/httpclient"

	"golang.org/x/net/html"
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
	body, err := s.fetch(ctx, articleURL)
	if err != nil {
		return "", fmt.Errorf("fetching article: %w", err)
	}
	return extractContent(body)
}

func (s *Scraper) scrapeArchive(ctx context.Context, articleURL string) (string, error) {
	archiveURL := s.archiveURL + articleURL
	body, err := s.fetch(ctx, archiveURL)
	if err != nil {
		return "", fmt.Errorf("fetching from archive.is: %w", err)
	}
	return extractContent(body)
}

func (s *Scraper) fetch(ctx context.Context, url string) (io.Reader, error) {
	reader, err := s.doFetch(ctx, url)
	if err == nil {
		return reader, nil
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

func (s *Scraper) doFetch(ctx context.Context, url string) (io.Reader, error) {
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
		return bytes.NewReader(data), nil
	}

	se := &httpclient.StatusError{StatusCode: resp.StatusCode}
	if resp.StatusCode == http.StatusTooManyRequests {
		se.RetryAfter = httpclient.ParseRetryAfter(resp.Header.Get("Retry-After"))
	}
	return nil, se
}

func extractContent(r io.Reader) (string, error) {
	doc, err := html.Parse(r)
	if err != nil {
		return "", fmt.Errorf("parsing HTML: %w", err)
	}

	removeUnwanted(doc)

	if content := findElement(doc, "article"); content != nil {
		return renderNode(content), nil
	}

	if content := findElementByRole(doc, "main"); content != nil {
		return renderNode(content), nil
	}

	if content := findElement(doc, "main"); content != nil {
		return renderNode(content), nil
	}

	if content := findLargestTextNode(doc); content != nil {
		return renderNode(content), nil
	}

	return "", nil
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
	switch n.Data {
	case "script", "style", "nav", "header", "footer", "aside",
		"noscript", "iframe", "form", "svg", "button", "input",
		"textarea", "select":
		return true
	}
	for _, attr := range n.Attr {
		if attr.Key == "class" {
			cls := strings.ToLower(attr.Val)
			if strings.Contains(cls, "comment") ||
				strings.Contains(cls, "sidebar") ||
				strings.Contains(cls, "advertisement") ||
				strings.Contains(cls, "ad-banner") ||
				strings.Contains(cls, "social-share") ||
				strings.Contains(cls, "newsletter") ||
				strings.Contains(cls, "popup") ||
				strings.Contains(cls, "cookie") ||
				strings.Contains(cls, "paywall") {
				return true
			}
		}
		if attr.Key == "id" {
			id := strings.ToLower(attr.Val)
			if strings.Contains(id, "comment") ||
				strings.Contains(id, "sidebar") ||
				strings.Contains(id, "footer") ||
				strings.Contains(id, "header") ||
				strings.Contains(id, "nav") {
				return true
			}
		}
	}
	return false
}

func findElement(n *html.Node, tag string) *html.Node {
	if n.Type == html.ElementNode && n.Data == tag {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if found := findElement(c, tag); found != nil {
			return found
		}
	}
	return nil
}

func findElementByRole(n *html.Node, role string) *html.Node {
	if n.Type == html.ElementNode {
		for _, attr := range n.Attr {
			if attr.Key == "role" && attr.Val == role {
				return n
			}
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if found := findElementByRole(c, role); found != nil {
			return found
		}
	}
	return nil
}

func findLargestTextNode(root *html.Node) *html.Node {
	var best *html.Node
	bestLen := 0

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "div", "section", "td":
				textLen := textLength(n)
				if textLen > bestLen && textLen > 200 {
					best = n
					bestLen = textLen
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)
	return best
}

func textLength(n *html.Node) int {
	total := 0
	var walk func(*html.Node)
	walk = func(c *html.Node) {
		if c.Type == html.TextNode {
			total += len(strings.TrimSpace(c.Data))
		}
		for child := c.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(n)
	return total
}

func renderNode(n *html.Node) string {
	var buf strings.Builder
	var write func(*html.Node)
	write = func(node *html.Node) {
		if node.Type == html.TextNode {
			buf.WriteString(node.Data)
			return
		}
		if node.Type == html.ElementNode {
			if node.Data == "a" && isDeadLink(node) {
				for c := node.FirstChild; c != nil; c = c.NextSibling {
					write(c)
				}
				return
			}
			if node.Data == "img" && isDeadImage(node) {
				return
			}
			buf.WriteString("<")
			buf.WriteString(node.Data)
			for _, attr := range node.Attr {
				buf.WriteString(" ")
				buf.WriteString(attr.Key)
				buf.WriteString(`="`)
				buf.WriteString(html.EscapeString(attr.Val))
				buf.WriteString(`"`)
			}
			buf.WriteString(">")
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			write(c)
		}
		if node.Type == html.ElementNode && !isVoidElement(node.Data) {
			buf.WriteString("</")
			buf.WriteString(node.Data)
			buf.WriteString(">")
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		write(c)
	}
	return strings.TrimSpace(buf.String())
}

func isVoidElement(tag string) bool {
	switch tag {
	case "br", "hr", "img", "input", "meta", "link", "area",
		"base", "col", "embed", "source", "track", "wbr":
		return true
	}
	return false
}

func isDeadLink(n *html.Node) bool {
	for _, attr := range n.Attr {
		if attr.Key != "href" {
			continue
		}
		href := strings.TrimSpace(attr.Val)
		if strings.HasPrefix(href, "http://") || strings.HasPrefix(href, "https://") {
			return false
		}
		return true
	}
	return true
}

func isDeadImage(n *html.Node) bool {
	for _, attr := range n.Attr {
		if attr.Key != "src" {
			continue
		}
		src := strings.TrimSpace(attr.Val)
		if strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://") {
			return false
		}
		return true
	}
	return true
}
