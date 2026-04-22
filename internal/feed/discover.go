package feed

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"time"
)

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

	faviconPaths  = []string{"/favicon.ico", "/favicon.png", "/apple-touch-icon.png"}
	discoverClient = &http.Client{Timeout: 15 * time.Second}
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
			return u.String()
		}
	}

	origin := *base
	origin.Path = ""
	origin.RawQuery = ""
	origin.Fragment = ""

	for _, path := range faviconPaths {
		u, _ := url.Parse(path)
		resolved := origin.ResolveReference(u)
		req, err := http.NewRequestWithContext(ctx, http.MethodHead, resolved.String(), nil)
		if err != nil {
			continue
		}
		resp, err := discoverClient.Do(req)
		if err != nil {
			continue
		}
		resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			return resolved.String()
		}
	}
	return ""
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
