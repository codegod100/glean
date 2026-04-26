package db

import (
	"context"
	"fmt"
	"time"

	"pkg.rbrt.fr/glean/internal/feed"
)

type FeedAdapter struct {
	store *ArticleStore
}

func NewFeedAdapter(store *ArticleStore) *FeedAdapter {
	return &FeedAdapter{store: store}
}

func (a *FeedAdapter) GetFeedsToFetch(ctx context.Context, olderThan time.Duration, limit int) ([]*feed.Feed, error) {
	dbFeeds, err := a.store.GetFeedsToFetch(ctx, olderThan, limit)
	if err != nil {
		return nil, err
	}
	var feeds []*feed.Feed
	for _, df := range dbFeeds {
		feeds = append(feeds, df.ToFeed())
	}
	return feeds, nil
}

func (a *FeedAdapter) RecordFetchError(ctx context.Context, feedURL, lastError string) error {
	return a.store.MarkFeedFetchError(ctx, feedURL, lastError)
}

func (a *FeedAdapter) StoreFetchResult(ctx context.Context, feedURL string, articles []feed.Article, faviconURL string) error {
	if len(articles) > 0 {
		if err := a.store.BatchUpsertArticles(ctx, articles); err != nil {
			return fmt.Errorf("failed to save articles: %w", err)
		}
	}

	if err := a.store.MarkFeedFetched(ctx, feedURL); err != nil {
		return fmt.Errorf("failed to mark as fetched: %w", err)
	}

	if faviconURL != "" {
		if err := a.store.UpdateFeedFavicon(ctx, feedURL, faviconURL); err != nil {
			return fmt.Errorf("failed to save favicon: %w", err)
		}
	}

	return nil
}
