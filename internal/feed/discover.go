package feed

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

type DiscoveryResult struct {
	FeedURLs []string
	Favicon  string
}

var (
	linkRe       = regexp.MustCompile(`<link[^>]+>`)
	hrefRe       = regexp.MustCompile(`href="([^"]*)"`)
	relFeedRe    = regexp.MustCompile(`rel="(alternate|feed)"`)
	typeFeedRe   = regexp.MustCompile(`type="([^"]*(?:rss|atom|feed|xml)[^"]*)"`)
	relIconRe    = regexp.MustCompile(`rel="[^"]*icon[^"]*"`)
	faviconPaths = []string{"/favicon.ico", "/favicon.png", "/apple-touch-icon.png"}
)

func Discover(ctx context.Context, siteURL string) (*DiscoveryResult, error) {
	client := &http.Client{Timeout: 15 * time.Second}

	result := &DiscoveryResult{}

	favicon := discoverFavicon(ctx, client, siteURL)
	if favicon != "" {
		result.Favicon = favicon
	}

	feeds := discoverFeedLinks(ctx, client, siteURL)
	result.FeedURLs = feeds

	return result, nil
}

func discoverFeedLinks(ctx context.Context, client *http.Client, siteURL string) []string {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, siteURL, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("Accept", "text/html")

	resp, err := client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1024*512))
	if err != nil {
		return nil
	}

	html := string(body)
	baseURL := resolveBaseURL(siteURL, html)

	var feeds []string
	links := linkRe.FindAllString(html, -1)
	for _, link := range links {
		if !relFeedRe.MatchString(link) && !typeFeedRe.MatchString(link) {
			continue
		}

		hrefMatch := hrefRe.FindStringSubmatch(link)
		if len(hrefMatch) < 2 {
			continue
		}

		feedURL := resolveURL(baseURL, hrefMatch[1])
		if feedURL != "" {
			feeds = append(feeds, feedURL)
		}
	}

	return feeds
}

func discoverFavicon(ctx context.Context, client *http.Client, siteURL string) string {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, siteURL, nil)
	if err != nil {
		return tryDefaultFavicons(ctx, client, siteURL)
	}
	req.Header.Set("Accept", "text/html")

	resp, err := client.Do(req)
	if err != nil {
		return tryDefaultFavicons(ctx, client, siteURL)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return tryDefaultFavicons(ctx, client, siteURL)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1024*512))
	if err != nil {
		return tryDefaultFavicons(ctx, client, siteURL)
	}

	html := string(body)
	baseURL := resolveBaseURL(siteURL, html)

	links := linkRe.FindAllString(html, -1)
	for _, link := range links {
		if !relIconRe.MatchString(link) {
			continue
		}

		hrefMatch := hrefRe.FindStringSubmatch(link)
		if len(hrefMatch) < 2 {
			continue
		}

		iconURL := resolveURL(baseURL, hrefMatch[1])
		if iconURL != "" {
			return iconURL
		}
	}

	return tryDefaultFavicons(ctx, client, siteURL)
}

func tryDefaultFavicons(ctx context.Context, client *http.Client, siteURL string) string {
	parsed := siteURL
	if !strings.HasPrefix(parsed, "http") {
		parsed = "https://" + parsed
	}

	base := parsed
	idx := strings.Index(base[8:], "/")
	if idx >= 0 {
		base = base[:8+idx]
	}

	for _, path := range faviconPaths {
		url := base + path
		req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
		if err != nil {
			continue
		}

		resp, err := client.Do(req)
		if err != nil {
			continue
		}
		resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			return url
		}
	}

	return ""
}

func resolveBaseURL(siteURL, html string) string {
	baseRe := regexp.MustCompile(`<base[^>]+href="([^"]*)"`)
	match := baseRe.FindStringSubmatch(html)
	if len(match) >= 2 && match[1] != "" {
		return resolveURL(siteURL, match[1])
	}
	return siteURL
}

func resolveURL(base, ref string) string {
	if strings.HasPrefix(ref, "http://") || strings.HasPrefix(ref, "https://") {
		return ref
	}

	base = strings.TrimRight(base, "/")
	if strings.HasPrefix(ref, "//") {
		return "https:" + ref
	}
	if strings.HasPrefix(ref, "/") {
		idx := strings.Index(base[8:], "/")
		if idx >= 0 {
			return base[:8+idx] + ref
		}
		return base + ref
	}

	idx := strings.LastIndex(base, "/")
	if idx > 8 {
		return base[:idx+1] + ref
	}
	return fmt.Sprintf("%s/%s", base, ref)
}
