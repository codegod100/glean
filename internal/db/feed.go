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

func (s *ArticleStore) UpsertFeed(ctx context.Context, feed *Feed) error {
	return s.BatchUpsertFeeds(ctx, []*Feed{feed})
}

func (s *ArticleStore) GetFeed(ctx context.Context, feedURL string) (*Feed, error) {
	f := &Feed{}
	err := s.db.QueryRowContext(ctx, `
		SELECT feed_url, title, site_url, description, feed_type,
			last_fetched_at, last_error, subscriber_count, etag, last_modified,
			consecutive_empty_fetches, error_count, favicon_url
		FROM articles.feeds WHERE feed_url = ?
	`, feedURL).Scan(&f.FeedURL, &f.Title, &f.SiteURL, &f.Description, &f.FeedType,
		&f.LastFetchedAt, &f.LastError, &f.SubscriberCount, &f.Etag, &f.LastModified,
		&f.ConsecutiveEmptyFetches, &f.ErrorCount, &f.FaviconURL)
	if err != nil {
		return nil, err
	}
	return f, nil
}

func (s *ArticleStore) GetFeedsToFetch(ctx context.Context, olderThan time.Duration, limit int) ([]*Feed, error) {
	cutoff := time.Now().Add(-olderThan)
	rows, err := s.db.QueryContext(ctx, `
		SELECT feed_url, title, site_url, description, feed_type,
			last_fetched_at, last_error, subscriber_count, etag, last_modified,
			consecutive_empty_fetches, error_count, favicon_url
		FROM articles.feeds
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
			&f.ConsecutiveEmptyFetches, &f.ErrorCount, &f.FaviconURL); err != nil {
			return nil, err
		}
		feeds = append(feeds, f)
	}
	return feeds, rows.Err()
}

func (s *ArticleStore) MarkFeedFetched(ctx context.Context, feedURL, etag, lastModified string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE articles.feeds SET
			etag = ?,
			last_modified = ?,
			error_count = 0,
			last_error = '',
			last_fetched_at = CURRENT_TIMESTAMP
		WHERE feed_url = ?
	`, etag, lastModified, feedURL)
	return err
}

func (s *ArticleStore) MarkFeedFetchError(ctx context.Context, feedURL, lastError string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE articles.feeds SET
			error_count = error_count + 1,
			last_error = ?,
			last_fetched_at = CURRENT_TIMESTAMP
		WHERE feed_url = ?
	`, lastError, feedURL)
	return err
}

func (s *ArticleStore) decrementSubscriberCount(ctx context.Context, feedURL string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE articles.feeds SET subscriber_count = MAX(subscriber_count - 1, 0) WHERE feed_url = ?
	`, feedURL)
	return err
}

func (s *ArticleStore) CreateSubscription(ctx context.Context, userDID, feedURL, title, category, uri, cid string) error {
	existing, err := s.GetSubscription(ctx, userDID, feedURL)
	if err == nil && existing != nil {
		if !existing.URI.Valid || existing.URI.String == "" {
			return s.updateSubscriptionURI(ctx, userDID, feedURL, uri, cid)
		}
		return ErrDuplicateSubscription
	}
	return s.BatchReconcileSubscriptions(ctx, userDID, []SubData{{FeedURL: feedURL, Title: title, Category: category, URI: uri, CID: cid}})
}

func (s *ArticleStore) updateSubscriptionURI(ctx context.Context, userDID, feedURL, uri, cid string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE articles.subscriptions SET uri = ?, cid = ? WHERE user_did = ? AND feed_url = ?
	`, uri, cid, userDID, feedURL)
	return err
}

func uriOrNil(v string) any {
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

func (s *ArticleStore) DeleteSubscription(ctx context.Context, userDID, feedURL string) error {
	_, err := s.db.ExecContext(ctx, `
		DELETE FROM articles.subscriptions WHERE user_did = ? AND feed_url = ?
	`, userDID, feedURL)
	if err != nil {
		return err
	}
	return s.decrementSubscriberCount(ctx, feedURL)
}

func (s *ArticleStore) DeleteAllSubscriptions(ctx context.Context, userDID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	rows, err := tx.QueryContext(ctx, `SELECT feed_url FROM articles.subscriptions WHERE user_did = ?`, userDID)
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

	_, err = tx.ExecContext(ctx, `DELETE FROM articles.subscriptions WHERE user_did = ?`, userDID)
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
			UPDATE articles.feeds SET subscriber_count = MAX(subscriber_count - 1, 0)
			WHERE feed_url IN (`+strings.Join(ph, ",")+`)
		`, args...)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (s *ArticleStore) GetSubscriptionByURI(ctx context.Context, userDID, uri string) (*Subscription, error) {
	sub := &Subscription{}
	err := s.db.QueryRowContext(ctx, `
		SELECT s.id, s.user_did, s.feed_url, COALESCE(s.title, f.title, ''), s.category, s.added_at,
		s.uri, s.cid
		FROM articles.subscriptions s
		LEFT JOIN articles.feeds f ON s.feed_url = f.feed_url
		WHERE s.user_did = ? AND s.uri = ?
	`, userDID, uri).Scan(&sub.ID, &sub.UserDID, &sub.FeedURL, &sub.FeedTitle, &sub.Category, &sub.AddedAt, &sub.URI, &sub.CID)
	if err != nil {
		return nil, err
	}
	return sub, nil
}

func (s *ArticleStore) GetSubscription(ctx context.Context, userDID, feedURL string) (*Subscription, error) {
	sub := &Subscription{}
	err := s.db.QueryRowContext(ctx, `
		SELECT s.id, s.user_did, s.feed_url, COALESCE(s.title, f.title, ''), s.category, s.added_at,
		s.uri, s.cid
		FROM articles.subscriptions s
		LEFT JOIN articles.feeds f ON s.feed_url = f.feed_url
		WHERE s.user_did = ? AND s.feed_url = ?
	`, userDID, feedURL).Scan(&sub.ID, &sub.UserDID, &sub.FeedURL, &sub.FeedTitle, &sub.Category, &sub.AddedAt, &sub.URI, &sub.CID)
	if err != nil {
		return nil, err
	}
	return sub, nil
}

func (s *ArticleStore) ListSubscriptions(ctx context.Context, userDID, category string, limit, offset int) ([]*Subscription, error) {
	query := `SELECT s.id, s.user_did, s.feed_url, COALESCE(s.title, f.title, ''), s.category, s.added_at,
		s.uri, s.cid, f.favicon_url
		FROM articles.subscriptions s
		LEFT JOIN articles.feeds f ON s.feed_url = f.feed_url
		WHERE s.user_did = ?`
	args := []any{userDID}

	if category != "" {
		query += ` AND s.category = ?`
		args = append(args, category)
	}

	query += ` ORDER BY s.added_at DESC LIMIT ? OFFSET ?`
	args = append(args, limit, offset)

	rows, err := s.db.QueryContext(ctx, query, args...)
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

func (s *ArticleStore) GetSubscriptionCount(ctx context.Context, userDID string) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM articles.subscriptions WHERE user_did = ?
	`, userDID).Scan(&count)
	return count, err
}

func (s *ArticleStore) GetCategories(ctx context.Context, userDID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT DISTINCT category FROM articles.subscriptions
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

func (s *ArticleStore) UpdateFeedFavicon(ctx context.Context, feedURL, faviconURL string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE articles.feeds SET favicon_url = ? WHERE feed_url = ?`, faviconURL, feedURL)
	return err
}

func (s *ArticleStore) ListDeadFeeds(ctx context.Context, userDID string, threshold int) ([]*Feed, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT f.feed_url, f.title, f.site_url, f.description, f.feed_type,
			f.last_fetched_at, f.last_error, f.subscriber_count, f.etag, f.last_modified,
			f.consecutive_empty_fetches, f.error_count, f.favicon_url
		FROM articles.feeds f
		JOIN articles.subscriptions s ON s.feed_url = f.feed_url AND s.user_did = ?
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
			&f.ConsecutiveEmptyFetches, &f.ErrorCount, &f.FaviconURL); err != nil {
			return nil, err
		}
		feeds = append(feeds, f)
	}
	return feeds, rows.Err()
}

func (s *ArticleStore) ListAllFeeds(ctx context.Context, limit, offset int) ([]*Feed, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT feed_url, title, site_url, description, feed_type,
			last_fetched_at, last_error, subscriber_count, etag, last_modified,
			consecutive_empty_fetches, error_count, favicon_url
		FROM articles.feeds
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
			&f.ConsecutiveEmptyFetches, &f.ErrorCount, &f.FaviconURL); err != nil {
			return nil, err
		}
		feeds = append(feeds, f)
	}
	return feeds, rows.Err()
}

type SubData struct {
	FeedURL  string
	Title    string
	Category string
	URI      string
	CID      string
}

func (s *ArticleStore) BatchUpsertFeeds(ctx context.Context, feeds []*Feed) error {
	if len(feeds) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO articles.feeds (feed_url, title, site_url, description, feed_type)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(feed_url) DO UPDATE SET
			title = excluded.title,
			site_url = excluded.site_url,
			description = excluded.description,
			feed_type = excluded.feed_type
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, f := range feeds {
		if _, err := stmt.ExecContext(ctx, f.FeedURL, f.Title, f.SiteURL, f.Description, f.FeedType); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *ArticleStore) BatchReconcileSubscriptions(ctx context.Context, userDID string, subs []SubData) error {
	if len(subs) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	rows, err := tx.QueryContext(ctx, `SELECT feed_url, COALESCE(uri, '') FROM articles.subscriptions WHERE user_did = ?`, userDID)
	if err != nil {
		return err
	}
	existing := make(map[string]string, len(subs))
	for rows.Next() {
		var feedURL, uri string
		if err := rows.Scan(&feedURL, &uri); err != nil {
			rows.Close()
			return err
		}
		existing[feedURL] = uri
	}
	rows.Close()

	insertStmt, err := tx.PrepareContext(ctx, `
		INSERT OR IGNORE INTO articles.subscriptions (user_did, feed_url, title, category, uri, cid)
		VALUES (?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer insertStmt.Close()

	updateStmt, err := tx.PrepareContext(ctx, `
		UPDATE articles.subscriptions SET uri = ?, cid = ? WHERE user_did = ? AND feed_url = ?
	`)
	if err != nil {
		return err
	}
	defer updateStmt.Close()

	incrStmt, err := tx.PrepareContext(ctx, `UPDATE articles.feeds SET subscriber_count = subscriber_count + 1 WHERE feed_url = ?`)
	if err != nil {
		return err
	}
	defer incrStmt.Close()

	for _, sub := range subs {
		if existingURI, ok := existing[sub.FeedURL]; ok {
			if existingURI == "" && sub.URI != "" {
				if _, err := updateStmt.ExecContext(ctx, sub.URI, sub.CID, userDID, sub.FeedURL); err != nil {
					return err
				}
			}
			continue
		}
		result, err := insertStmt.ExecContext(ctx, userDID, sub.FeedURL, nilIfEmpty(sub.Title), sub.Category, uriOrNil(sub.URI), uriOrNil(sub.CID))
		if err != nil {
			return err
		}
		n, _ := result.RowsAffected()
		if n > 0 {
			if _, err := incrStmt.ExecContext(ctx, sub.FeedURL); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

func (s *ArticleStore) ListUnsubscribedFeeds(ctx context.Context, userDID string, limit, offset int) ([]*Feed, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT feed_url, title, site_url, description, feed_type,
			last_fetched_at, last_error, subscriber_count, etag, last_modified,
			consecutive_empty_fetches, error_count, favicon_url
		FROM articles.feeds
		WHERE feed_url NOT IN (SELECT feed_url FROM articles.subscriptions WHERE user_did = ?)
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
			&f.ConsecutiveEmptyFetches, &f.ErrorCount, &f.FaviconURL); err != nil {
			return nil, err
		}
		feeds = append(feeds, f)
	}
	return feeds, rows.Err()
}
