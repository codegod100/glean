package db

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"pkg.rbrt.fr/glean/internal/feed"
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
	ConsecutiveEmptyFetches int
	ErrorCount              int
	FaviconURL              sql.NullString
}

func (f *Feed) ToFeed() *feed.Feed {
	return &feed.Feed{
		URL:         f.FeedURL,
		Title:       f.Title.String,
		SiteURL:     f.SiteURL.String,
		Description: f.Description.String,
		Type:        f.FeedType.String,
		FaviconURL:  f.FaviconURL.String,
	}
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

func scanFeed(scanner interface{ Scan(...any) error }) (*Feed, error) {
	f := &Feed{}
	if err := scanner.Scan(&f.FeedURL, &f.Title, &f.SiteURL, &f.Description, &f.FeedType,
		&f.LastFetchedAt, &f.LastError, &f.SubscriberCount,
		&f.ConsecutiveEmptyFetches, &f.ErrorCount, &f.FaviconURL); err != nil {
		return nil, err
	}
	return f, nil
}

func (s *ArticleStore) UpsertFeed(ctx context.Context, feed *Feed) error {
	return s.BatchUpsertFeeds(ctx, []*Feed{feed})
}

func (s *ArticleStore) GetFeed(ctx context.Context, feedURL string) (*Feed, error) {
	row := s.db.QueryRowContext(ctx, `SELECT * FROM articles.feeds WHERE feed_url = ?`, feedURL)
	return scanFeed(row)
}

func (s *ArticleStore) GetFeedsToFetch(ctx context.Context, olderThan time.Duration, limit int) ([]*Feed, error) {
	cutoff := time.Now().Add(-olderThan)
	rows, err := s.db.QueryContext(ctx, `SELECT * FROM articles.feeds
		WHERE subscriber_count > 0 AND error_count < 25 AND (last_fetched_at IS NULL OR last_fetched_at <= ?)
		ORDER BY last_fetched_at ASC NULLS FIRST LIMIT ?`, cutoff, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var feeds []*Feed
	for rows.Next() {
		f, err := scanFeed(rows)
		if err != nil {
			return nil, err
		}
		feeds = append(feeds, f)
	}
	return feeds, rows.Err()
}

func (s *ArticleStore) MarkFeedFetched(ctx context.Context, feedURL string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE articles.feeds SET
			error_count = 0,
			last_error = '',
			last_fetched_at = CURRENT_TIMESTAMP
		WHERE feed_url = ?
	`, feedURL)
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

func (s *ArticleStore) CreateSubscription(ctx context.Context, userDID, feedURL, title, category, uri, cid string) error {
	existing, err := s.GetSubscription(ctx, userDID, feedURL)
	if err != nil || existing == nil {
		_, err := s.db.ExecContext(ctx, `
			INSERT OR IGNORE INTO articles.subscriptions (user_did, feed_url, title, category, uri, cid)
			VALUES (?, ?, ?, ?, ?, ?)
		`, userDID, feedURL, nilIfEmpty(title), nilIfEmpty(category), nilIfEmpty(uri), nilIfEmpty(cid))
		return err
	}

	unchanged := existing.FeedTitle == title && existing.Category.String == category && existing.CID.String == cid
	if unchanged {
		return ErrDuplicateSubscription
	}

	if !existing.URI.Valid || existing.URI.String == "" {
		_, err := s.db.ExecContext(ctx, `
			UPDATE articles.subscriptions SET title = ?, category = ?, uri = ?, cid = ? WHERE user_did = ? AND feed_url = ?
		`, nilIfEmpty(title), nilIfEmpty(category), nilIfEmpty(uri), nilIfEmpty(cid), userDID, feedURL)
		return err
	}
	_, err = s.db.ExecContext(ctx, `
		UPDATE articles.subscriptions SET title = ?, category = ?, cid = ? WHERE user_did = ? AND feed_url = ?
	`, nilIfEmpty(title), nilIfEmpty(category), nilIfEmpty(cid), userDID, feedURL)
	return err
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
	return err
}

func (s *ArticleStore) DeleteAllSubscriptions(ctx context.Context, userDID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM articles.subscriptions WHERE user_did = ?`, userDID)
	return err
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
		s.uri, s.cid, f.favicon_url
		FROM articles.subscriptions s
		LEFT JOIN articles.feeds f ON s.feed_url = f.feed_url
		WHERE s.user_did = ? AND s.feed_url = ?
	`, userDID, feedURL).Scan(&sub.ID, &sub.UserDID, &sub.FeedURL, &sub.FeedTitle, &sub.Category, &sub.AddedAt, &sub.URI, &sub.CID, &sub.FaviconURL)
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

	if category == "__none__" {
		query += ` AND (s.category IS NULL OR s.category = '')`
	} else if category != "" {
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

func (s *ArticleStore) RecountSubscriberCounts(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE articles.feeds SET subscriber_count = (
			SELECT COUNT(*) FROM articles.subscriptions sub WHERE sub.feed_url = articles.feeds.feed_url
		)
	`)
	return err
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
	rows, err := s.db.QueryContext(ctx, `SELECT f.* FROM articles.feeds f
		JOIN articles.subscriptions s ON s.feed_url = f.feed_url AND s.user_did = ?
		WHERE f.error_count >= ? ORDER BY f.error_count DESC`, userDID, threshold)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var feeds []*Feed
	for rows.Next() {
		f, err := scanFeed(rows)
		if err != nil {
			return nil, err
		}
		feeds = append(feeds, f)
	}
	return feeds, rows.Err()
}

func (s *ArticleStore) ListAllFeeds(ctx context.Context, limit, offset int) ([]*Feed, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT * FROM articles.feeds
		ORDER BY subscriber_count DESC LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var feeds []*Feed
	for rows.Next() {
		f, err := scanFeed(rows)
		if err != nil {
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
			title = COALESCE(NULLIF(excluded.title, ''), articles.feeds.title),
			site_url = COALESCE(NULLIF(excluded.site_url, ''), articles.feeds.site_url),
			description = COALESCE(NULLIF(excluded.description, ''), articles.feeds.description),
			feed_type = COALESCE(NULLIF(excluded.feed_type, ''), articles.feeds.feed_type)
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

	backfillStmt, err := tx.PrepareContext(ctx, `
		UPDATE articles.subscriptions SET title = ?, category = ?, uri = ?, cid = ? WHERE user_did = ? AND feed_url = ?
	`)
	if err != nil {
		return err
	}
	defer backfillStmt.Close()

	updateStmt, err := tx.PrepareContext(ctx, `
		UPDATE articles.subscriptions SET title = ?, category = ?, cid = ? WHERE user_did = ? AND feed_url = ?
	`)
	if err != nil {
		return err
	}
	defer updateStmt.Close()

	for _, sub := range subs {
		if existingURI, ok := existing[sub.FeedURL]; ok {
			if existingURI == "" && sub.URI != "" {
				if _, err := backfillStmt.ExecContext(ctx, nilIfEmpty(sub.Title), nilIfEmpty(sub.Category), sub.URI, nilIfEmpty(sub.CID), userDID, sub.FeedURL); err != nil {
					return err
				}
			} else if existingURI == sub.URI {
				if _, err := updateStmt.ExecContext(ctx, nilIfEmpty(sub.Title), nilIfEmpty(sub.Category), nilIfEmpty(sub.CID), userDID, sub.FeedURL); err != nil {
					return err
				}
			}
			continue
		}
		if _, err := insertStmt.ExecContext(ctx, userDID, sub.FeedURL, nilIfEmpty(sub.Title), sub.Category, nilIfEmpty(sub.URI), nilIfEmpty(sub.CID)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *ArticleStore) ListSubscriptionsWithoutURI(ctx context.Context, userDID string) ([]SubData, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT feed_url, COALESCE(title, ''), COALESCE(category, '') FROM articles.subscriptions WHERE user_did = ? AND (uri IS NULL OR uri = '')`,
		userDID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var subs []SubData
	for rows.Next() {
		var sub SubData
		if err := rows.Scan(&sub.FeedURL, &sub.Title, &sub.Category); err != nil {
			return nil, err
		}
		subs = append(subs, sub)
	}
	return subs, rows.Err()
}

func (s *ArticleStore) UpdateSubscription(ctx context.Context, userDID, feedURL, title, category, uri, cid string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE articles.subscriptions SET title = ?, category = ?, uri = ?, cid = ? WHERE user_did = ? AND feed_url = ?`,
		nilIfEmpty(title), nilIfEmpty(category), nilIfEmpty(uri), nilIfEmpty(cid), userDID, feedURL)
	return err
}

func (s *ArticleStore) DeleteOrphanedSubscriptions(ctx context.Context, userDID string, activeFeedURLs map[string]bool) error {
	rows, err := s.db.QueryContext(ctx,
		`SELECT feed_url FROM articles.subscriptions WHERE user_did = ? AND uri IS NOT NULL AND uri != ''`,
		userDID)
	if err != nil {
		return err
	}
	var toDelete []string
	for rows.Next() {
		var feedURL string
		if err := rows.Scan(&feedURL); err != nil {
			rows.Close()
			return err
		}
		if !activeFeedURLs[feedURL] {
			toDelete = append(toDelete, feedURL)
		}
	}
	rows.Close()

	if len(toDelete) == 0 {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, feedURL := range toDelete {
		if _, err := tx.ExecContext(ctx, `DELETE FROM articles.subscriptions WHERE user_did = ? AND feed_url = ?`, userDID, feedURL); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (s *ArticleStore) ListUnsubscribedFeeds(ctx context.Context, userDID string, limit, offset int) ([]*Feed, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT * FROM articles.feeds
		WHERE feed_url NOT IN (SELECT feed_url FROM articles.subscriptions WHERE user_did = ?)
		ORDER BY subscriber_count DESC LIMIT ? OFFSET ?`, userDID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var feeds []*Feed
	for rows.Next() {
		f, err := scanFeed(rows)
		if err != nil {
			return nil, err
		}
		feeds = append(feeds, f)
	}
	return feeds, rows.Err()
}

type FeedListByDID struct {
	DID               string
	SubscriptionCount int
	Subscriptions     []SubData
}

func (s *ArticleStore) ListFeedListsByDIDs(ctx context.Context, dids []string, limit, offset int) ([]*FeedListByDID, error) {
	placeholders := make([]string, len(dids))
	args := make([]any, len(dids))
	for i, d := range dids {
		placeholders[i] = "?"
		args[i] = d
	}

	query := `
		SELECT u.did, COUNT(s.id) as subscription_count
		FROM users u
		LEFT JOIN articles.subscriptions s ON u.did = s.user_did
		WHERE u.did IN (` + strings.Join(placeholders, ",") + `)
		GROUP BY u.did ORDER BY u.did ASC LIMIT ? OFFSET ?`
	args = append(args, limit, offset)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type userRow struct {
		did      string
		subCount int
	}
	var users []userRow
	for rows.Next() {
		var did string
		var subCount int
		if err := rows.Scan(&did, &subCount); err != nil {
			return nil, err
		}
		users = append(users, userRow{did: did, subCount: subCount})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	subsByDID := make(map[string][]SubData)
	if len(users) > 0 {
		ph := make([]string, len(users))
		subArgs := make([]any, len(users))
		for i, u := range users {
			ph[i] = "?"
			subArgs[i] = u.did
		}
		subRows, err := s.db.QueryContext(ctx, `
			SELECT s.user_did, s.feed_url, COALESCE(s.title, f.title), COALESCE(s.category, '')
			FROM articles.subscriptions s
			JOIN articles.feeds f ON s.feed_url = f.feed_url
			WHERE s.user_did IN (`+strings.Join(ph, ",")+`)
			ORDER BY s.user_did, s.added_at DESC
		`, subArgs...)
		if err != nil {
			return nil, err
		}
		for subRows.Next() {
			var did, feedURL, title, cat string
			if err := subRows.Scan(&did, &feedURL, &title, &cat); err != nil {
				_ = subRows.Close()
				return nil, err
			}
			subsByDID[did] = append(subsByDID[did], SubData{
				FeedURL:  feedURL,
				Title:    title,
				Category: cat,
			})
		}
		_ = subRows.Close()
	}

	var result []*FeedListByDID
	for _, u := range users {
		result = append(result, &FeedListByDID{
			DID:               u.did,
			SubscriptionCount: u.subCount,
			Subscriptions:     subsByDID[u.did],
		})
	}
	return result, nil
}
