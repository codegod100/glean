package db

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

var ErrDuplicateSubscription = errors.New("already subscribed to this feed")

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
	FaviconURL              sql.NullString
}

type Subscription struct {
	ID          int64
	UserDID     string
	FeedURL     string
	FeedTitle   string
	Category    sql.NullString
	AddedAt     sql.NullTime
	UnreadCount int
	URI         sql.NullString
	CID         sql.NullString
	FaviconURL  sql.NullString
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
			fetch_interval_minutes, next_fetch_at, consecutive_empty_fetches, error_count, favicon_url
		FROM feeds WHERE feed_url = ?
	`, feedURL).Scan(&f.FeedURL, &f.Title, &f.SiteURL, &f.Description, &f.FeedType,
		&f.LastFetchedAt, &f.LastError, &f.SubscriberCount, &f.Etag, &f.LastModified,
		&f.FetchIntervalMinutes, &f.NextFetchAt, &f.ConsecutiveEmptyFetches, &f.ErrorCount, &f.FaviconURL)
	if err != nil {
		return nil, err
	}
	return f, nil
}

func (db *DB) GetFeedsToFetch(ctx context.Context, olderThan time.Duration, limit int) ([]*Feed, error) {
	cutoff := time.Now().Add(-olderThan)
	rows, err := db.QueryContext(ctx, `
		SELECT feed_url, title, site_url, description, feed_type,
			last_fetched_at, last_error, subscriber_count, etag, last_modified,
			fetch_interval_minutes, next_fetch_at, consecutive_empty_fetches, error_count, favicon_url
		FROM feeds
		WHERE subscriber_count > 0 AND error_count < 25 AND (last_fetched_at IS NULL OR last_fetched_at <= ?)
		ORDER BY last_fetched_at ASC NULLS FIRST
		LIMIT ?
	`, cutoff, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var feeds []*Feed
	for rows.Next() {
		f := &Feed{}
		if err := rows.Scan(&f.FeedURL, &f.Title, &f.SiteURL, &f.Description, &f.FeedType,
			&f.LastFetchedAt, &f.LastError, &f.SubscriberCount, &f.Etag, &f.LastModified,
			&f.FetchIntervalMinutes, &f.NextFetchAt, &f.ConsecutiveEmptyFetches, &f.ErrorCount, &f.FaviconURL); err != nil {
			return nil, err
		}
		feeds = append(feeds, f)
	}
	return feeds, rows.Err()
}

func (db *DB) MarkFeedFetched(ctx context.Context, feedURL, etag, lastModified string) error {
	_, err := db.ExecContext(ctx, `
		UPDATE feeds SET
			etag = ?,
			last_modified = ?,
			error_count = 0,
			last_error = '',
			last_fetched_at = CURRENT_TIMESTAMP
		WHERE feed_url = ?
	`, etag, lastModified, feedURL)
	return err
}

func (db *DB) MarkFeedFetchError(ctx context.Context, feedURL, lastError string) error {
	_, err := db.ExecContext(ctx, `
		UPDATE feeds SET
			error_count = error_count + 1,
			last_error = ?,
			last_fetched_at = CURRENT_TIMESTAMP
		WHERE feed_url = ?
	`, lastError, feedURL)
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

func (db *DB) CreateSubscription(ctx context.Context, userDID, feedURL, title, category, uri, cid string) error {
	result, err := db.ExecContext(ctx, `
		INSERT OR IGNORE INTO subscriptions (user_did, feed_url, title, category, uri, cid)
		VALUES (?, ?, ?, ?, ?, ?)
	`, userDID, feedURL, nilIfEmpty(title), category, uriOrNil(category, uri), uriOrNil(category, cid))
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return ErrDuplicateSubscription
	}
	return db.IncrementSubscriberCount(ctx, feedURL)
}

func (db *DB) UpdateSubscriptionURI(ctx context.Context, userDID, feedURL, uri, cid string) error {
	_, err := db.ExecContext(ctx, `
		UPDATE subscriptions SET uri = ?, cid = ? WHERE user_did = ? AND feed_url = ?
	`, uri, cid, userDID, feedURL)
	return err
}

func uriOrNil(category, v string) any {
	if v == "" {
		return nil
	}
	return v
}

func nilIfEmpty(v string) any {
	if v == "" {
		return nil
	}
	return v
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

func (db *DB) DeleteAllSubscriptions(ctx context.Context, userDID string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	rows, err := tx.QueryContext(ctx, `SELECT feed_url FROM subscriptions WHERE user_did = ?`, userDID)
	if err != nil {
		return err
	}
	var feedURLs []string
	for rows.Next() {
		var u string
		if err := rows.Scan(&u); err != nil {
			rows.Close()
			return err
		}
		feedURLs = append(feedURLs, u)
	}
	rows.Close()

	_, err = tx.ExecContext(ctx, `DELETE FROM subscriptions WHERE user_did = ?`, userDID)
	if err != nil {
		return err
	}

	if len(feedURLs) > 0 {
		ph := make([]string, len(feedURLs))
		args := make([]any, len(feedURLs))
		for i, u := range feedURLs {
			ph[i] = "?"
			args[i] = u
		}
		_, err = tx.ExecContext(ctx, `
			UPDATE feeds SET subscriber_count = MAX(subscriber_count - 1, 0)
			WHERE feed_url IN (`+strings.Join(ph, ",")+`)
		`, args...)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (db *DB) GetSubscriptionByURI(ctx context.Context, userDID, uri string) (*Subscription, error) {
	s := &Subscription{}
	err := db.QueryRowContext(ctx, `
		SELECT s.id, s.user_did, s.feed_url, COALESCE(s.title, f.title, ''), s.category, s.added_at,
		s.uri, s.cid
		FROM subscriptions s
		LEFT JOIN feeds f ON s.feed_url = f.feed_url
		WHERE s.user_did = ? AND s.uri = ?
	`, userDID, uri).Scan(&s.ID, &s.UserDID, &s.FeedURL, &s.FeedTitle, &s.Category, &s.AddedAt, &s.URI, &s.CID)
	if err != nil {
		return nil, err
	}
	return s, nil
}

func (db *DB) GetSubscription(ctx context.Context, userDID, feedURL string) (*Subscription, error) {
	s := &Subscription{}
	err := db.QueryRowContext(ctx, `
		SELECT s.id, s.user_did, s.feed_url, COALESCE(s.title, f.title, ''), s.category, s.added_at,
		s.uri, s.cid
		FROM subscriptions s
		LEFT JOIN feeds f ON s.feed_url = f.feed_url
		WHERE s.user_did = ? AND s.feed_url = ?
	`, userDID, feedURL).Scan(&s.ID, &s.UserDID, &s.FeedURL, &s.FeedTitle, &s.Category, &s.AddedAt, &s.URI, &s.CID)
	if err != nil {
		return nil, err
	}
	return s, nil
}

func (db *DB) ListSubscriptions(ctx context.Context, userDID, category string, limit, offset int) ([]*Subscription, error) {
	query := `SELECT s.id, s.user_did, s.feed_url, COALESCE(s.title, f.title, ''), s.category, s.added_at,
		s.uri, s.cid, f.favicon_url
		FROM subscriptions s
		LEFT JOIN feeds f ON s.feed_url = f.feed_url
		WHERE s.user_did = ?`
	args := []any{userDID}

	if category != "" {
		query += ` AND s.category = ?`
		args = append(args, category)
	}

	query += ` ORDER BY s.added_at DESC LIMIT ? OFFSET ?`
	args = append(args, limit, offset)

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var subs []*Subscription
	for rows.Next() {
		s := &Subscription{}
		if err := rows.Scan(&s.ID, &s.UserDID, &s.FeedURL, &s.FeedTitle, &s.Category, &s.AddedAt, &s.URI, &s.CID, &s.FaviconURL); err != nil {
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

func (db *DB) GetCategories(ctx context.Context, userDID string) ([]string, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT DISTINCT category FROM subscriptions
		WHERE user_did = ? AND category IS NOT NULL AND category != ''
		ORDER BY category
	`, userDID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var categories []string
	for rows.Next() {
		var cat string
		if err := rows.Scan(&cat); err != nil {
			return nil, err
		}
		categories = append(categories, cat)
	}
	return categories, rows.Err()
}

func (db *DB) UpdateFeedFavicon(ctx context.Context, feedURL, faviconURL string) error {
	_, err := db.ExecContext(ctx, `UPDATE feeds SET favicon_url = ? WHERE feed_url = ?`, faviconURL, feedURL)
	return err
}

func (db *DB) ListDeadFeeds(ctx context.Context, userDID string, threshold int) ([]*Feed, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT f.feed_url, f.title, f.site_url, f.description, f.feed_type,
			f.last_fetched_at, f.last_error, f.subscriber_count, f.etag, f.last_modified,
			f.fetch_interval_minutes, f.next_fetch_at, f.consecutive_empty_fetches, f.error_count, f.favicon_url
		FROM feeds f
		JOIN subscriptions s ON s.feed_url = f.feed_url AND s.user_did = ?
		WHERE f.error_count >= ?
		ORDER BY f.error_count DESC
	`, userDID, threshold)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var feeds []*Feed
	for rows.Next() {
		f := &Feed{}
		if err := rows.Scan(&f.FeedURL, &f.Title, &f.SiteURL, &f.Description, &f.FeedType,
			&f.LastFetchedAt, &f.LastError, &f.SubscriberCount, &f.Etag, &f.LastModified,
			&f.FetchIntervalMinutes, &f.NextFetchAt, &f.ConsecutiveEmptyFetches, &f.ErrorCount, &f.FaviconURL); err != nil {
			return nil, err
		}
		feeds = append(feeds, f)
	}
	return feeds, rows.Err()
}

func (db *DB) ListAllFeeds(ctx context.Context, limit, offset int) ([]*Feed, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT feed_url, title, site_url, description, feed_type,
			last_fetched_at, last_error, subscriber_count, etag, last_modified,
			fetch_interval_minutes, next_fetch_at, consecutive_empty_fetches, error_count, favicon_url
		FROM feeds
		ORDER BY subscriber_count DESC
		LIMIT ? OFFSET ?
	`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var feeds []*Feed
	for rows.Next() {
		f := &Feed{}
		if err := rows.Scan(&f.FeedURL, &f.Title, &f.SiteURL, &f.Description, &f.FeedType,
			&f.LastFetchedAt, &f.LastError, &f.SubscriberCount, &f.Etag, &f.LastModified,
			&f.FetchIntervalMinutes, &f.NextFetchAt, &f.ConsecutiveEmptyFetches, &f.ErrorCount, &f.FaviconURL); err != nil {
			return nil, err
		}
		feeds = append(feeds, f)
	}
	return feeds, rows.Err()
}

func (db *DB) ListUnsubscribedFeeds(ctx context.Context, userDID string, limit, offset int) ([]*Feed, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT feed_url, title, site_url, description, feed_type,
			last_fetched_at, last_error, subscriber_count, etag, last_modified,
			fetch_interval_minutes, next_fetch_at, consecutive_empty_fetches, error_count, favicon_url
		FROM feeds
		WHERE feed_url NOT IN (SELECT feed_url FROM subscriptions WHERE user_did = ?)
		ORDER BY subscriber_count DESC
		LIMIT ? OFFSET ?
	`, userDID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var feeds []*Feed
	for rows.Next() {
		f := &Feed{}
		if err := rows.Scan(&f.FeedURL, &f.Title, &f.SiteURL, &f.Description, &f.FeedType,
			&f.LastFetchedAt, &f.LastError, &f.SubscriberCount, &f.Etag, &f.LastModified,
			&f.FetchIntervalMinutes, &f.NextFetchAt, &f.ConsecutiveEmptyFetches, &f.ErrorCount, &f.FaviconURL); err != nil {
			return nil, err
		}
		feeds = append(feeds, f)
	}
	return feeds, rows.Err()
}
