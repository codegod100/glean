package db

import (
	"context"
	"database/sql"
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

func (a *FeedStoreAdapter) UpsertArticle(ctx context.Context, article *feed.Article) (int64, error) {
	dbArticle := &Article{
		FeedURL: article.FeedURL,
		GUID:    article.GUID,
		Title:   article.Title,
		Summary: sql.NullString{String: article.Summary, Valid: article.Summary != ""},
		Content: sql.NullString{String: article.Content, Valid: article.Content != ""},
		Author:  sql.NullString{String: article.Author, Valid: article.Author != ""},
		URL:     sql.NullString{String: article.URL, Valid: article.URL != ""},
	}
	if !article.Published.IsZero() {
		dbArticle.Published = sql.NullTime{Time: article.Published, Valid: true}
	}
	if !article.Updated.IsZero() {
		dbArticle.Updated = sql.NullTime{Time: article.Updated, Valid: true}
	}
	return a.db.UpsertArticle(ctx, dbArticle)
}

func (a *FeedStoreAdapter) MarkFeedFetched(ctx context.Context, feedURL, etag, lastModified string) error {
	return a.db.MarkFeedFetched(ctx, feedURL, etag, lastModified)
}

func (a *FeedStoreAdapter) MarkFeedFetchError(ctx context.Context, feedURL, lastError string) error {
	return a.db.MarkFeedFetchError(ctx, feedURL, lastError)
}

func (a *FeedStoreAdapter) UpdateFeedFavicon(ctx context.Context, feedURL, faviconURL string) error {
	return a.db.UpdateFeedFavicon(ctx, feedURL, faviconURL)
}
