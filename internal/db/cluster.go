package db

import (
	"context"
	"database/sql"
	"fmt"
)

func (db *DB) ComputeFeedSimilarity(ctx context.Context) error {
	_, err := db.ExecContext(ctx, `DELETE FROM feed_similarity`)
	if err != nil {
		return err
	}

	rows, err := db.QueryContext(ctx, `
		SELECT s1.feed_url, s2.feed_url, COUNT(*) AS overlap
		FROM subscriptions s1
		JOIN subscriptions s2 ON s1.user_did = s2.user_did AND s1.feed_url < s2.feed_url
		GROUP BY s1.feed_url, s2.feed_url
		HAVING overlap > 0
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	type pair struct {
		feedA   string
		feedB   string
		overlap int
	}
	var pairs []pair
	for rows.Next() {
		var p pair
		if err := rows.Scan(&p.feedA, &p.feedB, &p.overlap); err != nil {
			return err
		}
		pairs = append(pairs, p)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	subCounts := make(map[string]int)
	for _, p := range pairs {
		subCounts[p.feedA] = 0
		subCounts[p.feedB] = 0
	}

	if len(subCounts) > 0 {
		countRows, err := db.QueryContext(ctx, `
			SELECT feed_url, COUNT(*) FROM subscriptions GROUP BY feed_url
		`)
		if err != nil {
			return err
		}
		for countRows.Next() {
			var feedURL string
			var count int
			if err := countRows.Scan(&feedURL, &count); err != nil {
				countRows.Close()
				return err
			}
			subCounts[feedURL] = count
		}
		countRows.Close()
	}

	for _, p := range pairs {
		total := subCounts[p.feedA] + subCounts[p.feedB] - p.overlap
		if total == 0 {
			continue
		}
		jaccard := float64(p.overlap) / float64(total)
		_, err := db.ExecContext(ctx, `
			INSERT INTO feed_similarity (feed_a, feed_b, jaccard, computed_at)
			VALUES (?, ?, ?, CURRENT_TIMESTAMP)
		`, p.feedA, p.feedB, jaccard)
		if err != nil {
			return err
		}
	}

	return nil
}

func (db *DB) ComputeUserSimilarity(ctx context.Context) error {
	_, err := db.ExecContext(ctx, `DELETE FROM user_similarity`)
	if err != nil {
		return err
	}

	rows, err := db.QueryContext(ctx, `
		SELECT s1.user_did, s2.user_did, COUNT(*) AS common
		FROM subscriptions s1
		JOIN subscriptions s2 ON s1.user_did < s2.user_did AND s1.feed_url = s2.feed_url
		GROUP BY s1.user_did, s2.user_did
		HAVING common > 0
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	type pair struct {
		userA  string
		userB  string
		common int
	}
	var pairs []pair
	for rows.Next() {
		var p pair
		if err := rows.Scan(&p.userA, &p.userB, &p.common); err != nil {
			return err
		}
		pairs = append(pairs, p)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	subCounts := make(map[string]int)
	for _, p := range pairs {
		subCounts[p.userA] = 0
		subCounts[p.userB] = 0
	}

	if len(subCounts) > 0 {
		countRows, err := db.QueryContext(ctx, `
			SELECT user_did, COUNT(*) FROM subscriptions GROUP BY user_did
		`)
		if err != nil {
			return err
		}
		for countRows.Next() {
			var userDID string
			var count int
			if err := countRows.Scan(&userDID, &count); err != nil {
				countRows.Close()
				return err
			}
			subCounts[userDID] = count
		}
		countRows.Close()
	}

	for _, p := range pairs {
		total := subCounts[p.userA] + subCounts[p.userB] - p.common
		if total == 0 {
			continue
		}
		jaccard := float64(p.common) / float64(total)
		_, err := db.ExecContext(ctx, `
			INSERT INTO user_similarity (user_a, user_b, jaccard, common_feeds, computed_at)
			VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)
		`, p.userA, p.userB, jaccard, p.common)
		if err != nil {
			return err
		}
	}

	return nil
}

func (db *DB) ComputeFeedRecommendations(ctx context.Context, userDID string) error {
	_, err := db.ExecContext(ctx, `
		DELETE FROM user_feed_recommendations WHERE user_did = ?
	`, userDID)
	if err != nil {
		return err
	}

	rows, err := db.QueryContext(ctx, `
		SELECT
			CASE WHEN fs.feed_a IN (SELECT feed_url FROM subscriptions WHERE user_did = ?) THEN fs.feed_b ELSE fs.feed_a END AS recommended_feed,
			SUM(fs.jaccard) AS score
		FROM feed_similarity fs
		WHERE fs.feed_a IN (SELECT feed_url FROM subscriptions WHERE user_did = ?)
			OR fs.feed_b IN (SELECT feed_url FROM subscriptions WHERE user_did = ?)
		GROUP BY recommended_feed
		HAVING recommended_feed NOT IN (SELECT feed_url FROM subscriptions WHERE user_did = ?)
		ORDER BY score DESC
	`, userDID, userDID, userDID, userDID)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var feedURL string
		var score float64
		if err := rows.Scan(&feedURL, &score); err != nil {
			return err
		}
		_, err := db.ExecContext(ctx, `
			INSERT INTO user_feed_recommendations (user_did, feed_url, score, computed_at)
			VALUES (?, ?, ?, CURRENT_TIMESTAMP)
		`, userDID, feedURL, score)
		if err != nil {
			return err
		}
	}
	return rows.Err()
}

func (db *DB) GetFeedRecommendations(ctx context.Context, userDID string, limit int) ([]map[string]any, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT r.feed_url, r.score, f.title, f.site_url, f.description, f.subscriber_count
		FROM user_feed_recommendations r
		JOIN feeds f ON f.feed_url = r.feed_url
		WHERE r.user_did = ?
		ORDER BY r.score DESC
		LIMIT ?
	`, userDID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]any
	for rows.Next() {
		var feedURL string
		var score float64
		var title, siteURL, description sql.NullString
		var subCount int
		if err := rows.Scan(&feedURL, &score, &title, &siteURL, &description, &subCount); err != nil {
			return nil, err
		}
		results = append(results, map[string]any{
			"feed_url":        feedURL,
			"score":           score,
			"title":           title,
			"site_url":        siteURL,
			"description":     description,
			"subscriber_count": subCount,
		})
	}
	return results, rows.Err()
}

func (db *DB) GetPeopleRecommendations(ctx context.Context, userDID string, limit int) ([]map[string]any, error) {
	rows, err := db.QueryContext(ctx, fmt.Sprintf(`
		SELECT
			CASE WHEN us.user_a = ? THEN us.user_b ELSE us.user_a END AS recommended_user,
			us.jaccard, us.common_feeds,
			u.handle, u.display_name, u.avatar_url
		FROM user_similarity us
		JOIN users u ON u.did = CASE WHEN us.user_a = ? THEN us.user_b ELSE us.user_a END
		WHERE us.user_a = ? OR us.user_b = ?
		ORDER BY us.jaccard DESC
		LIMIT %d
	`, limit), userDID, userDID, userDID, userDID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]any
	for rows.Next() {
		var recUser, handle string
		var jaccard float64
		var commonFeeds int
		var displayName, avatarURL sql.NullString
		if err := rows.Scan(&recUser, &jaccard, &commonFeeds, &handle, &displayName, &avatarURL); err != nil {
			return nil, err
		}
		results = append(results, map[string]any{
			"did":          recUser,
			"jaccard":      jaccard,
			"common_feeds": commonFeeds,
			"handle":       handle,
			"display_name": displayName,
			"avatar_url":   avatarURL,
		})
	}
	return results, rows.Err()
}

func (db *DB) GetSimilarFeeds(ctx context.Context, feedURL string, limit int) ([]*Feed, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT f.feed_url, f.title, f.site_url, f.description, f.feed_type,
			f.last_fetched_at, f.last_error, f.subscriber_count, f.etag, f.last_modified,
			f.fetch_interval_minutes, f.next_fetch_at, f.consecutive_empty_fetches, f.error_count
		FROM feed_similarity fs
		JOIN feeds f ON f.feed_url = CASE WHEN fs.feed_a = ? THEN fs.feed_b ELSE fs.feed_a END
		WHERE fs.feed_a = ? OR fs.feed_b = ?
		ORDER BY fs.jaccard DESC
		LIMIT ?
	`, feedURL, feedURL, feedURL, limit)
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
