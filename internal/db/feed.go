package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

type Feed struct {
	FeedURL                 string
	Title                   sql.NullString
	SiteURL                 sql.NullString
	Description             sql.NullString
	FeedType                sql.NullString
	LastFetchedAt           sql.NullTime
	LastError               sql.NullString
	SubscriberCount         int
	Etag                    sql.NullString
	LastModified            sql.NullString
	FetchIntervalMinutes    int
	NextFetchAt             sql.NullTime
	ConsecutiveEmptyFetches int
	ErrorCount              int
}

type Subscription struct {
	ID         int64
	UserDID    string
	FeedURL    string
	FeedTitle  string
	Category   sql.NullString
	AddedAt    sql.NullTime
	UnreadCount int
}

func (db *DB) UpsertFeed(ctx context.Context, feed *Feed) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO feeds (feed_url, title, site_url, description, feed_type)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(feed_url) DO UPDATE SET
			title = excluded.title,
			site_url = excluded.site_url,
			description = excluded.description,
			feed_type = excluded.feed_type
	`, feed.FeedURL, feed.Title, feed.SiteURL, feed.Description, feed.FeedType)
	return err
}

func (db *DB) GetFeed(ctx context.Context, feedURL string) (*Feed, error) {
	f := &Feed{}
	err := db.QueryRowContext(ctx, `
		SELECT feed_url, title, site_url, description, feed_type,
			last_fetched_at, last_error, subscriber_count, etag, last_modified,
			fetch_interval_minutes, next_fetch_at, consecutive_empty_fetches, error_count
		FROM feeds WHERE feed_url = ?
	`, feedURL).Scan(&f.FeedURL, &f.Title, &f.SiteURL, &f.Description, &f.FeedType,
		&f.LastFetchedAt, &f.LastError, &f.SubscriberCount, &f.Etag, &f.LastModified,
		&f.FetchIntervalMinutes, &f.NextFetchAt, &f.ConsecutiveEmptyFetches, &f.ErrorCount)
	if err != nil {
		return nil, err
	}
	return f, nil
}

func (db *DB) GetFeedsToFetch(ctx context.Context, limit int) ([]*Feed, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT feed_url, title, site_url, description, feed_type,
			last_fetched_at, last_error, subscriber_count, etag, last_modified,
			fetch_interval_minutes, next_fetch_at, consecutive_empty_fetches, error_count
		FROM feeds
		WHERE next_fetch_at <= CURRENT_TIMESTAMP
		ORDER BY next_fetch_at
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var feeds []*Feed
	for rows.Next() {
		f := &Feed{}
		if err := rows.Scan(&f.FeedURL, &f.Title, &f.SiteURL, &f.Description, &f.FeedType,
			&f.LastFetchedAt, &f.LastError, &f.SubscriberCount, &f.Etag, &f.LastModified,
			&f.FetchIntervalMinutes, &f.NextFetchAt, &f.ConsecutiveEmptyFetches, &f.ErrorCount); err != nil {
			return nil, err
		}
		feeds = append(feeds, f)
	}
	return feeds, rows.Err()
}

func (db *DB) UpdateFeedFetchResult(ctx context.Context, feedURL, etag, lastModified string, intervalMinutes, consecutiveEmpty, errorCount int, lastError string) error {
	nextFetch := time.Now().Add(time.Duration(intervalMinutes) * time.Minute)
	_, err := db.ExecContext(ctx, `
		UPDATE feeds SET
			etag = ?,
			last_modified = ?,
			fetch_interval_minutes = ?,
			next_fetch_at = ?,
			consecutive_empty_fetches = ?,
			error_count = ?,
			last_error = ?,
			last_fetched_at = CURRENT_TIMESTAMP
		WHERE feed_url = ?
	`, etag, lastModified, intervalMinutes, nextFetch, consecutiveEmpty, errorCount, lastError, feedURL)
	return err
}

func (db *DB) IncrementSubscriberCount(ctx context.Context, feedURL string) error {
	_, err := db.ExecContext(ctx, `
		UPDATE feeds SET subscriber_count = subscriber_count + 1 WHERE feed_url = ?
	`, feedURL)
	return err
}

func (db *DB) DecrementSubscriberCount(ctx context.Context, feedURL string) error {
	_, err := db.ExecContext(ctx, `
		UPDATE feeds SET subscriber_count = MAX(subscriber_count - 1, 0) WHERE feed_url = ?
	`, feedURL)
	return err
}

func (db *DB) CreateSubscription(ctx context.Context, userDID, feedURL, category string) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO subscriptions (user_did, feed_url, category)
		VALUES (?, ?, ?)
	`, userDID, feedURL, category)
	if err != nil {
		return err
	}
	return db.IncrementSubscriberCount(ctx, feedURL)
}

func (db *DB) DeleteSubscription(ctx context.Context, userDID, feedURL string) error {
	_, err := db.ExecContext(ctx, `
		DELETE FROM subscriptions WHERE user_did = ? AND feed_url = ?
	`, userDID, feedURL)
	if err != nil {
		return err
	}
	return db.DecrementSubscriberCount(ctx, feedURL)
}

func (db *DB) ListSubscriptions(ctx context.Context, userDID, category string, limit, offset int) ([]*Subscription, error) {
	query := `SELECT s.id, s.user_did, s.feed_url, COALESCE(f.title, ''), s.category, s.added_at
		FROM subscriptions s
		LEFT JOIN feeds f ON s.feed_url = f.feed_url
		WHERE s.user_did = ?`
	args := []any{userDID}

	if category != "" {
		query += ` AND s.category = ?`
		args = append(args, category)
	}

	query += fmt.Sprintf(` ORDER BY s.added_at DESC LIMIT %d OFFSET %d`, limit, offset)

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var subs []*Subscription
	for rows.Next() {
		s := &Subscription{}
		if err := rows.Scan(&s.ID, &s.UserDID, &s.FeedURL, &s.FeedTitle, &s.Category, &s.AddedAt); err != nil {
			return nil, err
		}
		subs = append(subs, s)
	}
	return subs, rows.Err()
}

func (db *DB) GetSubscriptionCount(ctx context.Context, userDID string) (int, error) {
	var count int
	err := db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM subscriptions WHERE user_did = ?
	`, userDID).Scan(&count)
	return count, err
}

func (db *DB) ListAllFeeds(ctx context.Context, limit, offset int) ([]*Feed, error) {
	rows, err := db.QueryContext(ctx, fmt.Sprintf(`
		SELECT feed_url, title, site_url, description, feed_type,
			last_fetched_at, last_error, subscriber_count, etag, last_modified,
			fetch_interval_minutes, next_fetch_at, consecutive_empty_fetches, error_count
		FROM feeds
		ORDER BY subscriber_count DESC
		LIMIT %d OFFSET %d
	`, limit, offset))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var feeds []*Feed
	for rows.Next() {
		f := &Feed{}
		if err := rows.Scan(&f.FeedURL, &f.Title, &f.SiteURL, &f.Description, &f.FeedType,
			&f.LastFetchedAt, &f.LastError, &f.SubscriberCount, &f.Etag, &f.LastModified,
			&f.FetchIntervalMinutes, &f.NextFetchAt, &f.ConsecutiveEmptyFetches, &f.ErrorCount); err != nil {
			return nil, err
		}
		feeds = append(feeds, f)
	}
	return feeds, rows.Err()
}
