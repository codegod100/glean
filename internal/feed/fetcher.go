package feed

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"
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

	if resp.StatusCode != http.StatusOK {
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
	GetFeedsToFetch(ctx context.Context, limit int) ([]*Feed, error)
	UpsertArticle(ctx context.Context, article *Article) (int64, error)
	UpdateFeedFetchResult(ctx context.Context, feedURL string, etag, lastModified string, intervalMinutes, consecutiveEmpty, errorCount int, lastError string) error
}

type feedState struct {
	interval         time.Duration
	consecutiveEmpty int
	errorCount       int
}

type Scheduler struct {
	fetcher  *Fetcher
	store    FeedStore
	logger   *slog.Logger
	interval time.Duration
	states   map[string]*feedState
}

func NewScheduler(store FeedStore, logger *slog.Logger) *Scheduler {
	return &Scheduler{
		fetcher:  NewFetcher(),
		store:    store,
		logger:   logger,
		interval: 5 * time.Minute,
		states:   make(map[string]*feedState),
	}
}

func (s *Scheduler) Run(ctx context.Context) error {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			feeds, err := s.store.GetFeedsToFetch(ctx, 100)
			if err != nil {
				s.logger.Error("failed to get feeds", "error", err)
				continue
			}
			for _, feed := range feeds {
				s.FetchFeed(ctx, feed)
			}
		}
	}
}

func (s *Scheduler) FetchFeed(ctx context.Context, feed *Feed) {
	state, ok := s.states[feed.URL]
	if !ok {
		state = &feedState{
			interval: 30 * time.Minute,
		}
		s.states[feed.URL] = state
	}

	result, newEtag, newLastModified, err := s.fetcher.Fetch(ctx, feed.URL, feed.ETag, feed.LastModified)
	if err != nil {
		state.errorCount++
		state.interval *= 2
		if state.interval > 24*time.Hour {
			state.interval = 24 * time.Hour
		}

		updErr := s.store.UpdateFeedFetchResult(
			ctx, feed.URL,
			feed.ETag, feed.LastModified,
			int(state.interval.Minutes()), state.consecutiveEmpty, state.errorCount,
			err.Error(),
		)
		if updErr != nil {
			s.logger.Error("failed to update feed fetch result", "error", updErr, "feed", feed.URL)
		}
		return
	}

	if result == nil {
		return
	}

	newCount := 0
	for i := range result.Articles {
		result.Articles[i].FeedURL = feed.URL
		id, upsertErr := s.store.UpsertArticle(ctx, &result.Articles[i])
		if upsertErr != nil {
			s.logger.Error("failed to upsert article", "error", upsertErr, "url", result.Articles[i].URL)
			continue
		}
		if id > 0 {
			newCount++
		}
	}

	if newCount > 0 {
		state.errorCount = 0
		state.consecutiveEmpty = 0
		state.interval = 30 * time.Minute
	} else {
		state.consecutiveEmpty++
		if state.consecutiveEmpty > 3 {
			state.interval *= 2
			if state.interval > 6*time.Hour {
				state.interval = 6 * time.Hour
			}
		}
	}

	updErr := s.store.UpdateFeedFetchResult(
		ctx, feed.URL,
		newEtag, newLastModified,
		int(state.interval.Minutes()), state.consecutiveEmpty, state.errorCount,
		"",
	)
	if updErr != nil {
		s.logger.Error("failed to update feed fetch result", "error", updErr, "feed", feed.URL)
	}
}
