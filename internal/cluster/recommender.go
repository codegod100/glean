package cluster

import (
	"context"
	"database/sql"
)

type FeedRecommendation struct {
	FeedURL         string
	Title           string
	SiteURL         string
	Description     string
	SubscriberCount int
	FaviconURL      string
	Score           float64
}

type PersonRecommendation struct {
	DID         string
	Handle      string
	DisplayName string
	AvatarURL   string
	Jaccard     float64
	CommonFeeds int
	CommonLikes int
	CommonTags  int
}

type ArticleRecommendation struct {
	ArticleID int64
	Title     string
	URL       string
	FeedURL   string
	FeedTitle string
	Author    string
	Summary   string
	Published sql.NullTime
	Score     float64
}

type SimilarFeed struct {
	FeedURL     string
	Title       string
	SiteURL     string
	Description string
	FeedType    string
	Jaccard     float64
}

func (e *Engine) GetFeedRecommendations(ctx context.Context, userDID string, limit int) ([]*FeedRecommendation, error) {
	rows, err := e.db.QueryContext(ctx, `
		SELECT r.feed_url, COALESCE(f.title, ''), COALESCE(f.site_url, ''),
		       COALESCE(f.description, ''), f.subscriber_count, COALESCE(f.favicon_url, ''), r.score
		FROM user_feed_recommendations r
		JOIN feeds f ON f.feed_url = r.feed_url
		WHERE r.user_did = ?
		  AND r.feed_url NOT IN (SELECT feed_url FROM subscriptions WHERE user_did = ?)
		ORDER BY r.score DESC
		LIMIT ?
	`, userDID, userDID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*FeedRecommendation
	for rows.Next() {
		rec := &FeedRecommendation{}
		if err := rows.Scan(&rec.FeedURL, &rec.Title, &rec.SiteURL, &rec.Description,
			&rec.SubscriberCount, &rec.FaviconURL, &rec.Score); err != nil {
			return nil, err
		}
		results = append(results, rec)
	}
	return results, rows.Err()
}

func (e *Engine) GetPeopleRecommendations(ctx context.Context, userDID string, limit int) ([]*PersonRecommendation, error) {
	rows, err := e.db.QueryContext(ctx, `
		SELECT u.did, u.handle, COALESCE(u.display_name, ''), COALESCE(u.avatar_url, ''),
		       sim.jaccard, sim.common_feeds, COALESCE(sim.common_likes, 0), COALESCE(sim.common_tags, 0)
		FROM (
			SELECT user_b AS peer_did, jaccard, common_feeds, common_likes, common_tags FROM user_similarity WHERE user_a = ?
			UNION ALL
			SELECT user_a AS peer_did, jaccard, common_feeds, common_likes, common_tags FROM user_similarity WHERE user_b = ?
		) sim
		JOIN users u ON u.did = sim.peer_did
		WHERE u.handle IS NOT NULL AND u.handle != ''
		ORDER BY sim.jaccard DESC
		LIMIT ?
	`, userDID, userDID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*PersonRecommendation
	for rows.Next() {
		rec := &PersonRecommendation{}
		if err := rows.Scan(&rec.DID, &rec.Handle, &rec.DisplayName, &rec.AvatarURL,
			&rec.Jaccard, &rec.CommonFeeds, &rec.CommonLikes, &rec.CommonTags); err != nil {
			return nil, err
		}
		results = append(results, rec)
	}
	return results, rows.Err()
}

func (e *Engine) GetArticleRecommendations(ctx context.Context, userDID string, limit int) ([]*ArticleRecommendation, error) {
	rows, err := e.db.QueryContext(ctx, `
		SELECT a.id, a.title, COALESCE(a.url, ''), r.feed_url, COALESCE(f.title, ''),
			COALESCE(a.author, ''), COALESCE(a.summary, ''), a.published, r.score
		FROM user_article_recommendations r
		JOIN articles a ON a.feed_url = r.feed_url AND a.url = r.article_url
		LEFT JOIN feeds f ON f.feed_url = r.feed_url
		WHERE r.user_did = ?
		ORDER BY r.score DESC
		LIMIT ?
	`, userDID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var recs []*ArticleRecommendation
	for rows.Next() {
		rec := &ArticleRecommendation{}
		if err := rows.Scan(&rec.ArticleID, &rec.Title, &rec.URL, &rec.FeedURL, &rec.FeedTitle,
			&rec.Author, &rec.Summary, &rec.Published, &rec.Score); err != nil {
			return nil, err
		}
		recs = append(recs, rec)
	}
	return recs, rows.Err()
}

func (e *Engine) GetSimilarFeeds(ctx context.Context, feedURL string, limit int) ([]*SimilarFeed, error) {
	rows, err := e.db.QueryContext(ctx, `
		SELECT f.feed_url, COALESCE(f.title, ''), COALESCE(f.site_url, ''),
		       COALESCE(f.description, ''), COALESCE(f.feed_type, ''), sim.jaccard
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

	var results []*SimilarFeed
	for rows.Next() {
		rec := &SimilarFeed{}
		if err := rows.Scan(&rec.FeedURL, &rec.Title, &rec.SiteURL, &rec.Description,
			&rec.FeedType, &rec.Jaccard); err != nil {
			return nil, err
		}
		results = append(results, rec)
	}
	return results, rows.Err()
}
