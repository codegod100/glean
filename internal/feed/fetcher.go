package feed

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"pkg.rbrt.fr/glean/internal/metrics"
)

type Fetcher struct {
	httpClient *http.Client
}

func NewFetcher() *Fetcher {
	return &Fetcher{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 10 {
					return fmt.Errorf("too many redirects")
				}
				return nil
			},
		},
	}
}

func (f *Fetcher) Fetch(ctx context.Context, feedURL, etag, lastModified string) (*ParseResult, string, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, feedURL, nil)
	if err != nil {
		return nil, "", "", fmt.Errorf("creating request: %w", err)
	}

	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	if lastModified != "" {
		req.Header.Set("If-Modified-Since", lastModified)
	}

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return nil, "", "", fmt.Errorf("fetching feed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotModified {
		return nil, "", "", nil
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", "", fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	newEtag := resp.Header.Get("ETag")
	newLastModified := resp.Header.Get("Last-Modified")

	result, err := Parse(resp.Body, feedURL)
	if err != nil {
		return nil, "", "", fmt.Errorf("parsing feed: %w", err)
	}

	return result, newEtag, newLastModified, nil
}

type FeedStore interface {
	GetFeedsToFetch(ctx context.Context, olderThan time.Duration, limit int) ([]*Feed, error)
	UpsertArticle(ctx context.Context, article *Article) (int64, error)
	MarkFeedFetched(ctx context.Context, feedURL, etag, lastModified string) error
	MarkFeedFetchError(ctx context.Context, feedURL, lastError string) error
	UpdateFeedFavicon(ctx context.Context, feedURL, faviconURL string) error
}

type fetchCall struct {
	done chan struct{}
}

type Scheduler struct {
	fetcher  *Fetcher
	store    FeedStore
	logger   *slog.Logger
	interval time.Duration
	inFlight sync.Map
}

func NewScheduler(store FeedStore, logger *slog.Logger) *Scheduler {
	return &Scheduler{
		fetcher:  NewFetcher(),
		store:    store,
		logger:   logger,
		interval: 30 * time.Minute,
		inFlight: sync.Map{},
	}
}

func (s *Scheduler) Run(ctx context.Context) error {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	s.fetchAll(ctx)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			s.fetchAll(ctx)
		}
	}
}

func (s *Scheduler) fetchAll(ctx context.Context) {
	feeds, err := s.store.GetFeedsToFetch(ctx, s.interval, 200)
	if err != nil {
		s.logger.Error("failed to get feeds", "error", err)
		return
	}

	sem := make(chan struct{}, 3)
	var wg sync.WaitGroup
	for _, f := range feeds {
		wg.Add(1)
		sem <- struct{}{}
		go func(feed *Feed) {
			defer func() {
				<-sem
				wg.Done()
			}()
			s.FetchFeed(ctx, feed)
		}(f)
	}
	wg.Wait()
}

func (s *Scheduler) FetchFeed(ctx context.Context, feed *Feed) {
	call := &fetchCall{done: make(chan struct{})}
	if actual, loaded := s.inFlight.LoadOrStore(feed.URL, call); loaded {
		<-actual.(*fetchCall).done
		return
	}
	defer func() {
		s.inFlight.Delete(feed.URL)
		close(call.done)
	}()

	start := time.Now()
	result, newEtag, newLastModified, err := s.fetcher.Fetch(ctx, feed.URL, feed.ETag, feed.LastModified)
	metrics.FeedsFetchedDuration.Observe(time.Since(start).Seconds())
	if err != nil {
		metrics.FeedsFetched.WithLabelValues("error").Inc()
		s.logger.Error("failed to fetch feed", "error", err, "feed", feed.URL)
		if updErr := s.store.MarkFeedFetchError(ctx, feed.URL, err.Error()); updErr != nil {
			s.logger.Error("failed to update feed fetch error", "error", updErr, "feed", feed.URL)
		}
		return
	}

	if result == nil {
		metrics.FeedsFetched.WithLabelValues("not_modified").Inc()
		if updErr := s.store.MarkFeedFetched(ctx, feed.URL, feed.ETag, feed.LastModified); updErr != nil {
			s.logger.Error("failed to update feed fetch result", "error", updErr, "feed", feed.URL)
		}
		return
	}

	metrics.FeedsFetched.WithLabelValues("success").Inc()

	for i := range result.Articles {
		result.Articles[i].FeedURL = feed.URL
		if _, upsertErr := s.store.UpsertArticle(ctx, &result.Articles[i]); upsertErr != nil {
			s.logger.Error("failed to upsert article", "error", upsertErr, "url", result.Articles[i].URL)
		} else {
			metrics.ArticlesUpserted.Inc()
		}
	}

	if err := s.store.MarkFeedFetched(ctx, feed.URL, newEtag, newLastModified); err != nil {
		s.logger.Error("failed to update feed fetch result", "error", err, "feed", feed.URL)
	}

	if result != nil && result.Feed.FaviconURL != "" {
		_ = s.store.UpdateFeedFavicon(ctx, feed.URL, result.Feed.FaviconURL)
	} else if feed.FaviconURL == "" {
		siteURL := feed.SiteURL
		if siteURL == "" {
			siteURL = feed.URL
		}
		go func() {
			discResult, err := Discover(context.Background(), siteURL)
			if err == nil && discResult.Favicon != "" {
				_ = s.store.UpdateFeedFavicon(context.Background(), feed.URL, discResult.Favicon)
			}
		}()
	}
}
