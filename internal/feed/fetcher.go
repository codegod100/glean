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
	StoreFetchResult(ctx context.Context, feedURL, etag, lastModified string, articles []Article, faviconURL string) error
	RecordFetchError(ctx context.Context, feedURL, lastError string) error
}

type fetchCall struct {
	done chan struct{}
}

type Scheduler struct {
	fetcher       *Fetcher
	store         FeedStore
	logger        *slog.Logger
	tickInterval  time.Duration
	staleInterval time.Duration
	inFlight      sync.Map
}

func NewScheduler(store FeedStore, logger *slog.Logger, tickInterval, staleInterval time.Duration) *Scheduler {
	return &Scheduler{
		fetcher:       NewFetcher(),
		store:         store,
		logger:        logger,
		tickInterval:  tickInterval,
		staleInterval: staleInterval,
		inFlight:      sync.Map{},
	}
}

func (s *Scheduler) Run(ctx context.Context) error {
	ticker := time.NewTicker(s.tickInterval)
	defer ticker.Stop()

	// fetch all at startup
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
	feeds, err := s.store.GetFeedsToFetch(ctx, s.staleInterval, 10_000)
	if err != nil {
		s.logger.Error("failed to get feeds", "error", err)
		return
	}

	sem := make(chan struct{}, 10)
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
	metrics.FeedsFetched.Inc()
	metrics.FeedsFetchedLast.Set(float64(time.Now().Unix()))
	if err != nil {
		s.logger.Error("failed to fetch feed", "error", err, "feed", feed.URL)
		s.store.RecordFetchError(ctx, feed.URL, err.Error())
		return
	}

	if result == nil {
		s.logger.Info("fetched articles", "feed", feed.URL, "count", 0)
		if err := s.store.StoreFetchResult(ctx, feed.URL, newEtag, newLastModified, nil, ""); err != nil {
			s.logger.Error("failed to store feed fetch result", "error", err, "feed", feed.URL)
		}
		return
	}

	faviconURL := result.Feed.FaviconURL
	if faviconURL == "" && feed.FaviconURL == "" {
		faviconURL = ResolveFavicon(context.Background(), feed.URL, feed.SiteURL)
	}

	if err := s.store.StoreFetchResult(ctx, feed.URL, newEtag, newLastModified, result.Articles, faviconURL); err != nil {
		s.logger.Error("failed to store feed fetch result", "error", err, "feed", feed.URL)
	} else {
		articleCount := len(result.Articles)
		s.logger.Info("fetched articles", "feed", feed.URL, "count", articleCount)
		if articleCount > 0 {
			metrics.ArticlesUpserted.Add(float64(articleCount))
		}
	}
}
