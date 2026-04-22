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

func (e *Engine) GetFeedRecommendations(ctx context.Context, userDID string, limit int) ([]*FeedRecommendation, error) {
	subCount := 0
	_ = e.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM subscriptions WHERE user_did = ?`, userDID).Scan(&subCount)

	if subCount < 5 {
		recs, err := e.ColdStartRecommendations(ctx, userDID, limit*2)
		if err == nil && len(recs) > 0 {
			return ApplyDiversity(recs, limit), nil
		}
	}

	recs, err := e.ComputeFeedRecommendationsOnDemand(ctx, userDID, limit*2)
	if err != nil {
		return nil, err
	}

	return ApplyDiversity(recs, limit), nil
}

func (e *Engine) GetPeopleRecommendations(ctx context.Context, userDID string, limit int) ([]*PersonRecommendation, error) {
	return e.ComputePeopleRecommendationsOnDemand(ctx, userDID, limit)
}

func (e *Engine) GetArticleRecommendations(ctx context.Context, userDID string, limit int) ([]*ArticleRecommendation, error) {
	return e.ComputeArticleRecommendationsOnDemand(ctx, userDID, limit)
}

type SignalWeights struct {
	WSub      float64
	WLike     float64
	WTag      float64
	WSocial   float64
	WPop      float64
	WCategory float64
}

func defaultWeights() SignalWeights {
	return SignalWeights{
		WSub:      1.0,
		WLike:     0.5,
		WTag:      0.3,
		WSocial:   0.7,
		WPop:      0.2,
		WCategory: 0.4,
	}
}

func (e *Engine) GetWeights(ctx context.Context, userDID string) SignalWeights {
	w := defaultWeights()
	var dbW SignalWeights
	err := e.db.QueryRowContext(ctx, `
		SELECT w_sub, w_like, w_tag, w_social, w_pop, w_category
		FROM user_signal_weights WHERE user_did = ?
	`, userDID).Scan(&dbW.WSub, &dbW.WLike, &dbW.WTag, &dbW.WSocial, &dbW.WPop, &dbW.WCategory)
	if err == nil {
		return dbW
	}
	return w
}

func (e *Engine) ComputeFeedRecommendationsOnDemand(ctx context.Context, userDID string, limit int) ([]*FeedRecommendation, error) {
	w := e.GetWeights(ctx, userDID)

	rows, err := e.db.QueryContext(ctx, `
		WITH similar_users AS (
			SELECT user_b AS peer, jaccard FROM user_similarity WHERE user_a = ? AND jaccard > 0.15
			UNION ALL
			SELECT user_a AS peer, jaccard FROM user_similarity WHERE user_b = ? AND jaccard > 0.15
		),
		candidate_feeds AS (
			SELECT s.feed_url,
				SUM(su.jaccard) AS sub_signal
			FROM similar_users su
			JOIN subscriptions s ON s.user_did = su.peer
			WHERE s.feed_url NOT IN (SELECT feed_url FROM subscriptions WHERE user_did = ?)
			  AND s.feed_url NOT IN (SELECT target_id FROM dismissed_recommendations WHERE user_did = ? AND target_type = 'feed')
			GROUP BY s.feed_url
		),
		like_signals AS (
			SELECT s.feed_url,
				SUM(su.jaccard * EXP(-0.023 * CAST(julianday('now') - julianday(l.created_at) AS REAL))) AS like_signal
			FROM similar_users su
			JOIN likes l ON l.author_did = su.peer
			JOIN subscriptions s ON s.feed_url = l.feed_url
			WHERE s.feed_url NOT IN (SELECT feed_url FROM subscriptions WHERE user_did = ?)
			  AND s.feed_url NOT IN (SELECT target_id FROM dismissed_recommendations WHERE user_did = ? AND target_type = 'feed')
			GROUP BY s.feed_url
		),
		social_boost AS (
			SELECT s.feed_url,
				SUM(CASE WHEN fd.distance = 1 THEN 1.0 ELSE 0.3 END) AS social
			FROM follow_distances fd
			JOIN subscriptions s ON s.user_did = fd.user_b
			WHERE fd.user_a = ?
			  AND s.feed_url NOT IN (SELECT feed_url FROM subscriptions WHERE user_did = ?)
			  AND s.feed_url NOT IN (SELECT target_id FROM dismissed_recommendations WHERE user_did = ? AND target_type = 'feed')
			GROUP BY s.feed_url
		),
		category_counts AS (
			SELECT category, COUNT(*) AS cnt
			FROM subscriptions WHERE user_did = ? AND category IS NOT NULL AND category != ''
			GROUP BY category
		),
		max_subs AS (
			SELECT CAST(COALESCE(MAX(subscriber_count), 1) AS REAL) AS m FROM feeds
		)
		SELECT cf.feed_url, COALESCE(f.title, ''), COALESCE(f.site_url, ''),
		       COALESCE(f.description, ''), f.subscriber_count, COALESCE(f.favicon_url, ''),
		       COALESCE(cf.sub_signal, 0) * ?
		     + COALESCE(ls.like_signal, 0) * ?
		     + COALESCE(sb.social, 0) * ?
		     + COALESCE(LOG(1 + CAST(f.subscriber_count AS REAL)) / LOG(1 + ms.m), 0) * ?
		     + CASE WHEN f.description IS NOT NULL AND EXISTS (
		           SELECT 1 FROM category_counts cc
		           WHERE LOWER(f.description) LIKE '%' || LOWER(cc.category) || '%'
		       ) THEN ? ELSE 0 END
		       AS score
		FROM candidate_feeds cf
		JOIN feeds f ON f.feed_url = cf.feed_url
		LEFT JOIN like_signals ls ON ls.feed_url = cf.feed_url
		LEFT JOIN social_boost sb ON sb.feed_url = cf.feed_url
		CROSS JOIN max_subs ms
		ORDER BY score DESC
		LIMIT ?
	`, userDID, userDID, userDID, userDID, userDID, userDID, userDID, userDID, userDID, userDID,
		w.WSub, w.WLike, w.WSocial, w.WPop, w.WCategory, limit)
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

func (e *Engine) ComputeArticleRecommendationsOnDemand(ctx context.Context, userDID string, limit int) ([]*ArticleRecommendation, error) {
	w := e.GetWeights(ctx, userDID)

	rows, err := e.db.QueryContext(ctx, `
		WITH similar_users AS (
			SELECT user_b AS peer, jaccard FROM user_similarity WHERE user_a = ? AND jaccard > 0.15
			UNION ALL
			SELECT user_a AS peer, jaccard FROM user_similarity WHERE user_b = ? AND jaccard > 0.15
		),
		liked_articles AS (
			SELECT l.feed_url, l.article_url,
				SUM(su.jaccard * EXP(-0.023 * CAST(julianday('now') - julianday(l.created_at) AS REAL))) AS like_signal
			FROM similar_users su
			JOIN likes l ON l.author_did = su.peer
			WHERE NOT EXISTS (
				SELECT 1 FROM likes ul WHERE ul.author_did = ? AND ul.feed_url = l.feed_url AND ul.article_url = l.article_url
			)
			AND NOT EXISTS (
				SELECT 1 FROM dismissed_recommendations d WHERE d.user_did = ? AND d.target_type = 'article' AND d.target_id = l.article_url
			)
			GROUP BY l.feed_url, l.article_url
		),
		social_likes AS (
			SELECT l.feed_url, l.article_url,
				SUM(CASE WHEN fd.distance = 1 THEN 1.0 ELSE 0.3 END) AS social
			FROM follow_distances fd
			JOIN likes l ON l.author_did = fd.user_b
			WHERE fd.user_a = ?
			  AND NOT EXISTS (
				SELECT 1 FROM likes ul WHERE ul.author_did = ? AND ul.feed_url = l.feed_url AND ul.article_url = l.article_url
			  )
			GROUP BY l.feed_url, l.article_url
		)
		SELECT a.id, a.title, COALESCE(a.url, ''), la.feed_url, COALESCE(f.title, ''),
		       COALESCE(a.author, ''), COALESCE(a.summary, ''), a.published,
		       COALESCE(la.like_signal, 0) * ?
		     + COALESCE(sl.social, 0) * ?
		     + EXP(-0.023 * CAST(julianday('now') - julianday(a.published) AS REAL)) * 0.2
		       AS score
		FROM liked_articles la
		JOIN articles a ON a.feed_url = la.feed_url AND a.url = la.article_url
		LEFT JOIN feeds f ON f.feed_url = la.feed_url
		LEFT JOIN social_likes sl ON sl.feed_url = la.feed_url AND sl.article_url = la.article_url
		ORDER BY score DESC
		LIMIT ?
	`, userDID, userDID, userDID, userDID, userDID, userDID, w.WLike, w.WSocial, limit)
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

func (e *Engine) ComputePeopleRecommendationsOnDemand(ctx context.Context, userDID string, limit int) ([]*PersonRecommendation, error) {
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

func (e *Engine) ComputeSignalProfiles(ctx context.Context) error {
	tx, err := e.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `DELETE FROM user_signal_profiles`); err != nil {
		return err
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO user_signal_profiles (user_did, total_likes, total_tags, top_categories)
		SELECT
			u.did,
			(SELECT COUNT(*) FROM likes WHERE author_did = u.did),
			COALESCE((SELECT COUNT(DISTINCT TRIM(value))
				FROM annotations, json_each('["' || REPLACE(tags, ',', '","') || '"]')
				WHERE author_did = u.did AND tags IS NOT NULL AND tags != ''
			), 0),
			(SELECT '[' || GROUP_CONCAT('{"c":"' || category || '","n":"' || CAST(cnt AS TEXT) || '}') || ']'
			 FROM (
				SELECT category, COUNT(*) AS cnt
				FROM subscriptions WHERE user_did = u.did AND category IS NOT NULL AND category != ''
				GROUP BY category ORDER BY cnt DESC LIMIT 5
			 )
			)
		FROM users u
	`)
	if err != nil {
		return err
	}

	e.logger.Info("signal profiles computed")
	return tx.Commit()
}

func (e *Engine) ColdStartRecommendations(ctx context.Context, userDID string, limit int) ([]*FeedRecommendation, error) {
	subCount := 0
	_ = e.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM subscriptions WHERE user_did = ?`, userDID).Scan(&subCount)
	if subCount >= 5 {
		return nil, nil
	}

	rows, err := e.db.QueryContext(ctx, `
		WITH followed_feeds AS (
			SELECT s.feed_url, 1.0 AS weight
			FROM follow_distances fd
			JOIN subscriptions s ON s.user_did = fd.user_b
			WHERE fd.user_a = ? AND fd.distance = 1
			AND s.feed_url NOT IN (SELECT feed_url FROM subscriptions WHERE user_did = ?)
			AND s.feed_url NOT IN (SELECT target_id FROM dismissed_recommendations WHERE user_did = ? AND target_type = 'feed')
		),
		popular_feeds AS (
			SELECT feed_url, subscriber_count,
				LOG(1 + CAST(subscriber_count AS REAL)) / LOG(1 + CAST((SELECT COALESCE(MAX(subscriber_count), 1) FROM feeds) AS REAL)) AS pop_score
			FROM feeds
			WHERE subscriber_count > 0
			AND feed_url NOT IN (SELECT feed_url FROM subscriptions WHERE user_did = ?)
			AND feed_url NOT IN (SELECT target_id FROM dismissed_recommendations WHERE user_did = ? AND target_type = 'feed')
			ORDER BY subscriber_count DESC
			LIMIT 50
		),
		all_candidates AS (
			SELECT feed_url, MAX(weight) AS weight FROM (
				SELECT feed_url, weight FROM followed_feeds
				UNION ALL
				SELECT feed_url, pop_score AS weight FROM popular_feeds
			)
			GROUP BY feed_url
		)
		SELECT ac.feed_url,
		       COALESCE(f.title, ''),
		       COALESCE(f.site_url, ''),
		       COALESCE(f.description, ''),
		       f.subscriber_count,
		       COALESCE(f.favicon_url, ''),
		       ac.weight AS score
		FROM all_candidates ac
		JOIN feeds f ON f.feed_url = ac.feed_url
		ORDER BY score DESC
		LIMIT ?
	`, userDID, userDID, userDID, userDID, userDID, limit)
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
