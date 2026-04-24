package feed

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
	"pkg.rbrt.fr/glean/internal/httpclient"
	"pkg.rbrt.fr/glean/internal/metrics"
)

const (
	maxRetries     = 3
	baseRetryDelay = 1 * time.Second
)

type Fetcher struct {
	httpClient *http.Client
}

func NewFetcher() *Fetcher {
	return &Fetcher{
		httpClient: &http.Client{
			Timeout:   10 * time.Second,
			Transport: httpclient.NewTransport(),
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 5 {
					return fmt.Errorf("too many redirects")
				}
				return nil
			},
		},
	}
}

func (f *Fetcher) Fetch(ctx context.Context, feedURL string) (*ParseResult, error) {
	var lastResp *http.Response
	var lastErr error

	for attempt := range maxRetries + 1 {
		if attempt > 0 {
			backoff := retryBackoff(attempt, lastResp)
			if err := httpclient.SleepWithContext(ctx, backoff); err != nil {
				return nil, err
			}
		}

		if lastResp != nil {
			lastResp.Body.Close()
		}

		result, resp, err := f.executeRequest(ctx, feedURL)
		lastResp = resp
		if err == nil {
			return result, nil
		}

		if resp != nil && !httpclient.IsRetryable(resp.StatusCode) {
			return nil, err
		}

		lastErr = err
	}

	return nil, lastErr
}

func (f *Fetcher) executeRequest(ctx context.Context, feedURL string) (*ParseResult, *http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, feedURL, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("creating request: %w", err)
	}

	httpclient.SetDefaultHeaders(req)
	req.Header.Set("Accept", httpclient.AcceptFeed)

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("fetching feed: %w", err)
	}
	defer resp.Body.Close()

	result, err := Parse(resp.Body, feedURL)
	if err != nil {
		return nil, nil, fmt.Errorf("parsing feed: %w", err)
	}

	return result, resp, nil
}

func retryBackoff(attempt int, lastResp *http.Response) time.Duration {
	if lastResp != nil && lastResp.StatusCode == http.StatusTooManyRequests {
		if v := lastResp.Header.Get("Retry-After"); v != "" {
			if d := httpclient.ParseRetryAfter(v); d > 0 {
				return min(d, 10*time.Second)
			}
		}
	}
	return baseRetryDelay * time.Duration(1<<(attempt-1))
}

type FeedStore interface {
	GetFeedsToFetch(ctx context.Context, olderThan time.Duration, limit int) ([]*Feed, error)
	StoreFetchResult(ctx context.Context, feedURL string, articles []Article, faviconURL string) error
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
	s.logger.Info("starting initial feed refresh")
	s.fetchAll(ctx, s.staleInterval)

	ticker := time.NewTicker(s.tickInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			s.fetchAll(ctx, s.staleInterval)
		}
	}
}

func (s *Scheduler) fetchAll(ctx context.Context, olderThan time.Duration) {
	feeds, err := s.store.GetFeedsToFetch(ctx, olderThan, 1000)
	if err != nil {
		s.logger.Error("failed to get feeds", "error", err)
		return
	}

	start := time.Now()
	s.logger.Info("fetching feeds", "count", len(feeds), "older_than", olderThan)

	g, gCtx := errgroup.WithContext(ctx)
	g.SetLimit(50)
	for _, f := range feeds {
		g.Go(func() error {
			s.FetchFeed(gCtx, f)
			return nil
		})
	}
	_ = g.Wait()

	s.logger.Info("fetching feeds complete", "duration", time.Since(start).Seconds())
}

func (s *Scheduler) FetchFeed(ctx context.Context, feed *Feed) {
	call := &fetchCall{done: make(chan struct{})}
	if actual, loaded := s.inFlight.LoadOrStore(feed.URL, call); loaded {
		s.logger.Debug("feed already in flight, skipping", "feed", feed.URL)
		select {
		case <-actual.(*fetchCall).done:
		case <-ctx.Done():
		}
		return
	}
	defer func() {
		s.inFlight.Delete(feed.URL)
		close(call.done)
	}()

	start := time.Now()
	result, err := s.fetcher.Fetch(ctx, feed.URL)
	metrics.FeedsFetchedDuration.Observe(time.Since(start).Seconds())
	metrics.FeedsFetched.Inc()
	metrics.FeedsFetchedLast.Set(float64(time.Now().Unix()))
	if err != nil {
		s.logger.Error("failed to fetch feed", "error", err, "feed", feed.URL)
		_ = s.store.RecordFetchError(ctx, feed.URL, err.Error())
		return
	}

	if result == nil {
		s.logger.Info("fetched articles", "feed", feed.URL, "count", 0)
		if err := s.store.StoreFetchResult(ctx, feed.URL, nil, ""); err != nil {
			s.logger.Error("failed to store feed fetch result", "error", err, "feed", feed.URL)
		}
		return
	}

	faviconURL := result.Feed.FaviconURL
	if faviconURL == "" && feed.FaviconURL == "" {
		faviconURL = ResolveFavicon(ctx, feed.URL, feed.SiteURL)
	}

	if err := s.store.StoreFetchResult(ctx, feed.URL, result.Articles, faviconURL); err != nil {
		s.logger.Error("failed to store feed fetch result", "error", err, "feed", feed.URL)
	} else {
		articleCount := len(result.Articles)
		s.logger.Info("fetched articles", "feed", feed.URL, "count", articleCount)
		if articleCount > 0 {
			metrics.ArticlesUpserted.Add(float64(articleCount))
		}
	}
}
