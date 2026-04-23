package db

import (
	"context"
	"time"

	"pkg.rbrt.fr/glean/internal/feed"
)

type FeedStoreAdapter struct {
	store *ArticleStore
}

func NewFeedStoreAdapter(store *ArticleStore) *FeedStoreAdapter {
	return &FeedStoreAdapter{store: store}
}

func (a *FeedStoreAdapter) GetFeedsToFetch(ctx context.Context, olderThan time.Duration, limit int) ([]*feed.Feed, error) {
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

func (a *FeedStoreAdapter) RecordFetchError(ctx context.Context, feedURL, lastError string) error {
	return a.store.MarkFeedFetchError(ctx, feedURL, lastError)
}

func (a *FeedStoreAdapter) StoreFetchResult(ctx context.Context, feedURL string, articles []feed.Article, faviconURL string) error {
	if err := a.store.MarkFeedFetched(ctx, feedURL); err != nil {
		return err
	}
	if len(articles) > 0 {
		if err := a.store.UpsertArticlesBatch(ctx, articles); err != nil {
			return err
		}
	}
	if faviconURL != "" {
		if err := a.store.UpdateFeedFavicon(ctx, feedURL, faviconURL); err != nil {
			return err
		}
	}
	return nil
}
