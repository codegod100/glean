package db

import (
	"context"
	"time"

	"pkg.rbrt.fr/glean/internal/feed"
)

type FeedStoreAdapter struct {
	db *DB
}

func NewFeedStoreAdapter(db *DB) *FeedStoreAdapter {
	return &FeedStoreAdapter{db: db}
}

func (a *FeedStoreAdapter) GetFeedsToFetch(ctx context.Context, olderThan time.Duration, limit int) ([]*feed.Feed, error) {
	dbFeeds, err := a.db.GetFeedsToFetch(ctx, olderThan, limit)
	if err != nil {
		return nil, err
	}
	var feeds []*feed.Feed
	for _, df := range dbFeeds {
		feeds = append(feeds, &feed.Feed{
			URL:          df.FeedURL,
			Title:        df.Title.String,
			SiteURL:      df.SiteURL.String,
			Description:  df.Description.String,
			Type:         df.FeedType.String,
			FaviconURL:   df.FaviconURL.String,
			ETag:         df.Etag.String,
			LastModified: df.LastModified.String,
		})
	}
	return feeds, nil
}

func (a *FeedStoreAdapter) RecordFetchError(ctx context.Context, feedURL, lastError string) error {
	return a.db.MarkFeedFetchError(ctx, feedURL, lastError)
}

func (a *FeedStoreAdapter) StoreFetchResult(ctx context.Context, feedURL, etag, lastModified string, articles []feed.Article, faviconURL string) error {
	if err := a.db.MarkFeedFetched(ctx, feedURL, etag, lastModified); err != nil {
		return err
	}
	if len(articles) > 0 {
		if err := a.db.UpsertArticlesBatch(ctx, articles); err != nil {
			return err
		}
	}
	if faviconURL != "" {
		if err := a.db.UpdateFeedFavicon(ctx, feedURL, faviconURL); err != nil {
			return err
		}
	}
	return nil
}
