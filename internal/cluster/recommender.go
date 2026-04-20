package cluster

import (
	"context"
)

func (e *Engine) GetFeedRecommendations(ctx context.Context, userDID string, limit int) ([]map[string]any, error) {
	rows, err := e.db.QueryContext(ctx, `
		SELECT r.feed_url, f.title, f.site_url, f.description, f.feed_type, r.score
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
		var feedURL, title, siteURL, description, feedType string
		var score float64
		if err := rows.Scan(&feedURL, &title, &siteURL, &description, &feedType, &score); err != nil {
			return nil, err
		}
		results = append(results, map[string]any{
			"feed_url":    feedURL,
			"title":       title,
			"site_url":    siteURL,
			"description": description,
			"feed_type":   feedType,
			"score":       score,
		})
	}
	return results, rows.Err()
}

func (e *Engine) GetPeopleRecommendations(ctx context.Context, userDID string, limit int) ([]map[string]any, error) {
	rows, err := e.db.QueryContext(ctx, `
		SELECT u.did, u.handle, u.display_name, u.avatar_url, sim.jaccard, sim.common_feeds
		FROM (
			SELECT user_b AS peer_did, jaccard, common_feeds FROM user_similarity WHERE user_a = ?
			UNION ALL
			SELECT user_a AS peer_did, jaccard, common_feeds FROM user_similarity WHERE user_b = ?
		) sim
		JOIN users u ON u.did = sim.peer_did
		ORDER BY sim.jaccard DESC
		LIMIT ?
	`, userDID, userDID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]any
	for rows.Next() {
		var did, handle, displayName, avatarURL string
		var jaccard float64
		var commonFeeds int
		if err := rows.Scan(&did, &handle, &displayName, &avatarURL, &jaccard, &commonFeeds); err != nil {
			return nil, err
		}
		results = append(results, map[string]any{
			"did":          did,
			"handle":       handle,
			"display_name": displayName,
			"avatar_url":   avatarURL,
			"jaccard":      jaccard,
			"common_feeds": commonFeeds,
		})
	}
	return results, rows.Err()
}

func (e *Engine) GetSimilarFeeds(ctx context.Context, feedURL string, limit int) ([]map[string]any, error) {
	rows, err := e.db.QueryContext(ctx, `
		SELECT f.feed_url, f.title, f.site_url, f.description, f.feed_type, sim.jaccard
		FROM (
			SELECT feed_b AS peer_url, jaccard FROM feed_similarity WHERE feed_a = ?
			UNION ALL
			SELECT feed_a AS peer_url, jaccard FROM feed_similarity WHERE feed_b = ?
		) sim
		JOIN feeds f ON f.feed_url = sim.peer_url
		ORDER BY sim.jaccard DESC
		LIMIT ?
	`, feedURL, feedURL, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]any
	for rows.Next() {
		var peerURL, title, siteURL, description, feedType string
		var jaccard float64
		if err := rows.Scan(&peerURL, &title, &siteURL, &description, &feedType, &jaccard); err != nil {
			return nil, err
		}
		results = append(results, map[string]any{
			"feed_url":    peerURL,
			"title":       title,
			"site_url":    siteURL,
			"description": description,
			"feed_type":   feedType,
			"jaccard":     jaccard,
		})
	}
	return results, rows.Err()
}
