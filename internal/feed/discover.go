package feed

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"pkg.rbrt.fr/glean/internal/httpclient"
)

func isImageContentType(ct string) bool {
	return strings.HasPrefix(ct, "image/")
}

type DiscoveryResult struct {
	FeedURLs []string
	Favicon  string
}

var (
	linkRe     = regexp.MustCompile(`<link[^>]+>`)
	hrefRe     = regexp.MustCompile(`href="([^"]*)"`)
	relFeedRe  = regexp.MustCompile(`rel="(alternate|feed)"`)
	typeFeedRe = regexp.MustCompile(`type="[^"]*(?:rss|atom|feed|xml)[^"]*"`)
	relIconRe  = regexp.MustCompile(`rel="[^"]*icon[^"]*"`)
	baseHrefRe = regexp.MustCompile(`<base[^>]+href="([^"]*)"`)

	faviconPaths   = []string{"/favicon.ico", "/favicon.png", "/apple-touch-icon.png"}
	discoverClient = &http.Client{
		Timeout:   15 * time.Second,
		Transport: httpclient.NewTransport(),
	}
)

func Discover(ctx context.Context, siteURL string) (*DiscoveryResult, error) {
	base, html := fetchHTML(ctx, siteURL)
	if base == nil {
		return &DiscoveryResult{}, nil
	}

	if m := baseHrefRe.FindStringSubmatch(html); len(m) >= 2 && m[1] != "" {
		if u, err := base.Parse(m[1]); err == nil {
			base = u
		}
	}

	links := linkRe.FindAllString(html, -1)
	return &DiscoveryResult{
		FeedURLs: findFeedURLs(base, links),
		Favicon:  findFavicon(ctx, base, links),
	}, nil
}

func cleanFavicon(s string) string {
	return strings.TrimRight(s, "/")
}

func ResolveFavicon(ctx context.Context, feedURL, siteURL string) string {
	target := siteURL
	if target == "" {
		target = feedURL
	}
	result, err := Discover(ctx, target)
	if err != nil {
		return ""
	}
	return cleanFavicon(result.Favicon)
}

func findFeedURLs(base *url.URL, links []string) []string {
	var feeds []string
	for _, link := range links {
		if !relFeedRe.MatchString(link) && !typeFeedRe.MatchString(link) {
			continue
		}
		href := extractHref(link)
		if href == "" {
			continue
		}
		if u, err := base.Parse(href); err == nil {
			feeds = append(feeds, u.String())
		}
	}
	return feeds
}

func findFavicon(ctx context.Context, base *url.URL, links []string) string {
	for _, link := range links {
		if !relIconRe.MatchString(link) {
			continue
		}
		href := extractHref(link)
		if href == "" {
			continue
		}
		if u, err := base.Parse(href); err == nil {
			if checkContentType(ctx, u.String()) {
				return cleanFavicon(u.String())
			}
		}
	}

	origin := *base
	origin.Path = ""
	origin.RawQuery = ""
	origin.Fragment = ""

	type result struct {
		url   string
		found bool
	}
	found := make(chan result, 1)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var wg sync.WaitGroup
	for _, path := range faviconPaths {
		wg.Add(1)
		go func(path string) {
			defer wg.Done()
			u, _ := url.Parse(path)
			resolved := origin.ResolveReference(u)
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, resolved.String(), nil)
			if err != nil {
				return
			}
			httpclient.SetDefaultHeaders(req)
			resp, err := discoverClient.Do(req)
			if err != nil {
				return
			}
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK && isImageContentType(resp.Header.Get("Content-Type")) {
				select {
				case found <- result{url: cleanFavicon(resolved.String()), found: true}:
				default:
				}
			}
		}(path)
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case r := <-found:
		if r.found {
			return r.url
		}
	case <-done:
	case <-ctx.Done():
	}
	return ""
}

func checkContentType(ctx context.Context, url string) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return false
	}
	httpclient.SetDefaultHeaders(req)
	resp, err := discoverClient.Do(req)
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return resp.StatusCode == http.StatusOK && isImageContentType(resp.Header.Get("Content-Type"))
}

func extractHref(link string) string {
	m := hrefRe.FindStringSubmatch(link)
	if len(m) >= 2 {
		return m[1]
	}
	return ""
}

func fetchHTML(ctx context.Context, siteURL string) (*url.URL, string) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, siteURL, nil)
	if err != nil {
		return nil, ""
	}
	httpclient.SetDefaultHeaders(req)
	req.Header.Set("Accept", "text/html")

	resp, err := discoverClient.Do(req)
	if err != nil {
		return nil, ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, ""
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1024*512))
	if err != nil {
		return nil, ""
	}

	base := resp.Request.URL
	return base, string(body)
}
