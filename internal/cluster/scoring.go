package cluster

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"pkg.rbrt.fr/glean/internal/db"
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
	IsFollowed  bool
}

type ArticleRecommendation struct {
	ArticleID  int64
	Title      string
	URL        string
	FeedURL    string
	FeedTitle  string
	FaviconURL string
	Author     string
	Summary    string
	Published  sql.NullTime
	IsRead     bool
	Score      float64
}

// GetFeedRecommendations returns feed recommendations for a user. Users with
// fewer than 5 subscriptions get cold-start recommendations (embedding-based
// KNN or graph+popular fallback). Results are min-max normalized and
// diversity-filtered before returning.
func (e *Engine) GetFeedRecommendations(ctx context.Context, userDID string, limit int) ([]*FeedRecommendation, error) {
	if entry, ok := e.feedCache.Get(userDID); ok {
		return entry, nil
	}

	subCount := 0
	_ = e.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM articles.subscriptions WHERE user_did = ?`, userDID).Scan(&subCount)

	if subCount < 5 {
		recs, err := e.ColdStartRecommendations(ctx, userDID, limit*2)
		if err == nil && len(recs) > 0 {
			normalizeFeedScores(recs)
			return ApplyDiversity(recs, limit), nil
		}
	}

	recs, err := e.ComputeFeedRecommendationsOnDemand(ctx, userDID, limit*2)
	if err != nil {
		return nil, err
	}

	normalizeFeedScores(recs)
	result := ApplyDiversity(recs, limit)
	e.feedCache.Add(userDID, result)
	return result, nil
}

// GetPeopleRecommendations returns similar users based on subscription overlap,
// like co-occurrence, tag overlap, and follow relationships. Up to half the
// limit come from the user's network (followed) and half from outside. When
// outside-network candidates are scarce, in-network fills the remaining slots.
func (e *Engine) GetPeopleRecommendations(ctx context.Context, userDID string, limit int) ([]*PersonRecommendation, error) {
	if entry, ok := e.peopleCache.Get(userDID); ok {
		return entry, nil
	}

	recs, err := e.ComputePeopleRecommendationsOnDemand(ctx, userDID, limit*2)
	if err != nil {
		return nil, err
	}

	half := max(limit/2, 1)

	var inNet, outNet []*PersonRecommendation
	for _, r := range recs {
		if r.IsFollowed {
			inNet = append(inNet, r)
		} else {
			outNet = append(outNet, r)
		}
	}

	outTake := min(half, len(outNet))
	inTake := min(limit-outTake, len(inNet))
	recs = recs[:0]
	recs = append(recs, inNet[:inTake]...)
	recs = append(recs, outNet[:outTake]...)

	normalizePersonScores(recs)
	e.peopleCache.Add(userDID, recs)
	return recs, nil
}

// GetArticleRecommendations returns article recommendations combining social
// signals (liked by similar users, followed users' feeds), content similarity
// (embedding KNN against user's liked articles), and recency. Scores are
// min-max normalized.
func (e *Engine) GetArticleRecommendations(ctx context.Context, userDID string, languages []string, limit int) ([]*ArticleRecommendation, error) {
	artKey := userDID + "|" + strings.Join(languages, ",")
	if entry, ok := e.articleCache.Get(artKey); ok {
		return entry, nil
	}

	recs, err := e.ComputeArticleRecommendationsOnDemand(ctx, userDID, languages, limit)
	if err != nil {
		return nil, err
	}
	normalizeArticleScores(recs)
	e.articleCache.Add(artKey, recs)
	return recs, nil
}

func (e *Engine) GetGlobalTrending(ctx context.Context, userDID string, limit, offset int) ([]*db.TrendingItem, error) {
	if offset == 0 {
		if entry, ok := e.globalTrendingCache.Get(userDID); ok {
			return entry, nil
		}
	}

	since := time.Now().AddDate(0, 0, -7).Format(time.RFC3339)
	items, err := e.articles.ListTrendingArticles(ctx, userDID, since, limit, offset)
	if err != nil {
		return nil, err
	}

	if offset == 0 {
		e.globalTrendingCache.Add(userDID, items)
	}
	return items, nil
}

func (e *Engine) GetPersonalTrending(ctx context.Context, userDID string, languages []string, limit, offset int) ([]*db.TrendingItem, error) {
	key := userDID + "|" + strings.Join(languages, ",")
	if offset == 0 {
		if entry, ok := e.personalTrendingCache.Get(key); ok {
			return entry, nil
		}
	}

	since := time.Now().AddDate(0, 0, -7).Format(time.RFC3339)
	items, err := e.articles.ListTrendingArticlesForUser(ctx, userDID, since, languages, limit, offset)
	if err != nil {
		return nil, err
	}

	if offset == 0 {
		e.personalTrendingCache.Add(key, items)
	}
	return items, nil
}

// SignalWeights holds per-signal multipliers used in the recommendation scoring
// formula. Weights are auto-tuned per user via a bandit-style reward/penalty
// system (see weights.go). Each field maps to a column in
// recs.user_signal_weights.
type SignalWeights struct {
	WSub      float64
	WLike     float64
	WTag      float64
	WSocial   float64
	WPop      float64
	WCategory float64
	WContent  float64
}

func defaultWeights() SignalWeights {
	return SignalWeights{
		WSub:      1.0,
		WLike:     0.5,
		WTag:      0.3,
		WSocial:   0.7,
		WPop:      0.2,
		WCategory: 0.4,
		WContent:  0.4,
	}
}

func (e *Engine) GetWeights(ctx context.Context, userDID string) SignalWeights {
	w := defaultWeights()
	var dbW SignalWeights
	err := e.db.QueryRowContext(ctx, `
		SELECT w_sub, w_like, w_tag, w_social, w_pop, w_category, w_content
		FROM recs.user_signal_weights WHERE user_did = ?
	`, userDID).Scan(&dbW.WSub, &dbW.WLike, &dbW.WTag, &dbW.WSocial, &dbW.WPop, &dbW.WCategory, &dbW.WContent)
	if err == nil {
		return dbW
	}
	return w
}

func (e *Engine) ComputeFeedRecommendationsOnDemand(ctx context.Context, userDID string, limit int) ([]*FeedRecommendation, error) {
	w := e.GetWeights(ctx, userDID)

	rows, err := e.db.QueryContext(ctx, `
		WITH similar_users AS (
			SELECT user_b AS peer, jaccard FROM recs.user_similarity WHERE user_a = ? AND jaccard > 0.15
			UNION ALL
			SELECT user_a AS peer, jaccard FROM recs.user_similarity WHERE user_b = ? AND jaccard > 0.15
		),
		candidate_feeds AS (
			SELECT s.feed_url,
				SUM(su.jaccard) AS sub_signal
			FROM similar_users su
			JOIN articles.subscriptions s ON s.user_did = su.peer
			WHERE s.feed_url NOT IN (SELECT feed_url FROM articles.subscriptions WHERE user_did = ?)
			  AND s.feed_url NOT IN (SELECT target_id FROM main.dismissed_recommendations WHERE user_did = ? AND target_type = 'feed')
			GROUP BY s.feed_url
		),
		like_signals AS (
			SELECT s.feed_url,
				SUM(su.jaccard * EXP(-0.023 * CAST(julianday('now') - julianday(l.created_at) AS REAL))) AS like_signal
			FROM similar_users su
			JOIN articles.likes l ON l.author_did = su.peer
			JOIN articles.subscriptions s ON s.feed_url = l.feed_url
			WHERE s.feed_url NOT IN (SELECT feed_url FROM articles.subscriptions WHERE user_did = ?)
			  AND s.feed_url NOT IN (SELECT target_id FROM main.dismissed_recommendations WHERE user_did = ? AND target_type = 'feed')
			GROUP BY s.feed_url
		),
		social_boost AS (
			SELECT s.feed_url,
				SUM(CASE fd.distance WHEN 1 THEN 1.0 WHEN 2 THEN 0.3 WHEN 3 THEN 0.1 ELSE 0 END) AS social
			FROM recs.follow_distances fd
			JOIN articles.subscriptions s ON s.user_did = fd.user_b
			WHERE fd.user_a = ?
			  AND s.feed_url NOT IN (SELECT feed_url FROM articles.subscriptions WHERE user_did = ?)
			  AND s.feed_url NOT IN (SELECT target_id FROM main.dismissed_recommendations WHERE user_did = ? AND target_type = 'feed')
			GROUP BY s.feed_url
		),
		category_counts AS (
			SELECT category, COUNT(*) AS cnt
			FROM articles.subscriptions WHERE user_did = ? AND category IS NOT NULL AND category != ''
			GROUP BY category
		),
		max_subs AS (
			SELECT CAST(COALESCE(MAX(subscriber_count), 1) AS REAL) AS m FROM articles.feeds
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
		JOIN articles.feeds f ON f.feed_url = cf.feed_url
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

func normalizeScores[T any](items []T, getScore func(T) float64, setScore func(T, float64)) {
	if len(items) < 2 {
		return
	}
	smin, smax := getScore(items[0]), getScore(items[0])
	for i := range items[1:] {
		s := getScore(items[i+1])
		if s < smin {
			smin = s
		}
		if s > smax {
			smax = s
		}
	}
	if smax == smin {
		return
	}
	span := smax - smin
	for i := range items {
		setScore(items[i], (getScore(items[i])-smin)/span)
	}
}

func normalizeFeedScores(recs []*FeedRecommendation) {
	normalizeScores(recs,
		func(r *FeedRecommendation) float64 { return r.Score },
		func(r *FeedRecommendation, v float64) { r.Score = v },
	)
}

func normalizeArticleScores(recs []*ArticleRecommendation) {
	normalizeScores(recs,
		func(r *ArticleRecommendation) float64 { return r.Score },
		func(r *ArticleRecommendation, v float64) { r.Score = v },
	)
}

func normalizePersonScores(recs []*PersonRecommendation) {
	normalizeScores(recs,
		func(r *PersonRecommendation) float64 { return r.Jaccard },
		func(r *PersonRecommendation, v float64) { r.Jaccard = v },
	)
}

func (e *Engine) coldStartFromEmbeddings(ctx context.Context, userDID string, limit int) ([]*FeedRecommendation, error) {
	if e.embedder == nil {
		return nil, nil
	}

	conn, err := e.db.Conn(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	dim := e.embedder.Dimension()
	subRows, err := conn.QueryContext(ctx, `
		SELECT fe.feed_url, fe.embedding FROM articles.subscriptions s
		JOIN recs.feed_embeddings fe ON fe.feed_url = s.feed_url
		WHERE s.user_did = ?
	`, userDID)
	if err != nil {
		return nil, err
	}

	var subFeedURLs []string
	var blobs [][]byte
	for subRows.Next() {
		var url string
		var blob []byte
		if err := subRows.Scan(&url, &blob); err != nil {
			subRows.Close()
			return nil, err
		}
		if len(blob) != dim*4 {
			continue
		}
		subFeedURLs = append(subFeedURLs, url)
		blobs = append(blobs, blob)
	}
	subRows.Close()

	if len(blobs) == 0 {
		return nil, nil
	}

	subSet := make(map[string]bool, len(subFeedURLs))
	for _, u := range subFeedURLs {
		subSet[u] = true
	}

	queryBlob, err := avgEmbeddings(blobs, dim)
	if err != nil {
		return nil, fmt.Errorf("serialize query vector: %w", err)
	}

	knnRows, err := conn.QueryContext(ctx, `
		SELECT fe.feed_url, fe.distance, COALESCE(f.title, ''), COALESCE(f.site_url, ''),
			COALESCE(f.description, ''), f.subscriber_count, COALESCE(f.favicon_url, '')
		FROM recs.feed_embeddings fe
		JOIN articles.feeds f ON f.feed_url = fe.feed_url
		WHERE fe.embedding MATCH ? AND fe.k = ?
		ORDER BY fe.distance
	`, queryBlob, limit+len(subSet))
	if err != nil {
		return nil, err
	}

	var results []*FeedRecommendation
	for knnRows.Next() {
		var r FeedRecommendation
		var dist float64
		if err := knnRows.Scan(&r.FeedURL, &dist, &r.Title, &r.SiteURL,
			&r.Description, &r.SubscriberCount, &r.FaviconURL); err != nil {
			knnRows.Close()
			return nil, err
		}
		if subSet[r.FeedURL] {
			continue
		}
		r.Score = 1.0 - dist
		if r.Score <= 0 {
			continue
		}
		results = append(results, &r)
		if len(results) >= limit {
			break
		}
	}
	knnRows.Close()

	return results, nil
}

func (e *Engine) ComputeArticleRecommendationsOnDemand(ctx context.Context, userDID string, languages []string, limit int) ([]*ArticleRecommendation, error) {
	w := e.GetWeights(ctx, userDID)

	conn, err := e.db.Conn(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	if err := e.ensureContentBoostTable(ctx, conn); err != nil {
		return nil, err
	}

	if e.embedder != nil {
		if err := e.populateContentBoost(ctx, conn, userDID); err != nil {
			e.logger.Warn("content boost failed", "error", err)
		}
	}

	langFilter, langArgs := db.BuildLangFilter(languages, "a.")

	query := fmt.Sprintf(`
		WITH similar_users AS (
			SELECT user_b AS peer, jaccard FROM recs.user_similarity WHERE user_a = ? AND jaccard > 0.15
			UNION ALL
			SELECT user_a AS peer, jaccard FROM recs.user_similarity WHERE user_b = ? AND jaccard > 0.15
		),
		liked_articles AS (
			SELECT l.feed_url, l.article_url,
				SUM(su.jaccard * EXP(-0.023 * CAST(julianday('now') - julianday(l.created_at) AS REAL))) AS like_signal
			FROM similar_users su
			JOIN articles.likes l ON l.author_did = su.peer
			WHERE NOT EXISTS (
				SELECT 1 FROM articles.likes ul WHERE ul.author_did = ? AND ul.feed_url = l.feed_url AND ul.article_url = l.article_url
			)
			AND NOT EXISTS (
				SELECT 1 FROM main.dismissed_recommendations d WHERE d.user_did = ? AND d.target_type = 'article' AND d.target_id = l.article_url
			)
			GROUP BY l.feed_url, l.article_url
		),
		social_likes AS (
			SELECT l.feed_url, l.article_url,
				SUM(CASE fd.distance WHEN 1 THEN 1.0 WHEN 2 THEN 0.3 WHEN 3 THEN 0.1 ELSE 0 END) AS social
			FROM recs.follow_distances fd
			JOIN articles.likes l ON l.author_did = fd.user_b
			WHERE fd.user_a = ?
			  AND NOT EXISTS (
				SELECT 1 FROM articles.likes ul WHERE ul.author_did = ? AND ul.feed_url = l.feed_url AND ul.article_url = l.article_url
			  )
			GROUP BY l.feed_url, l.article_url
		)
		SELECT a.id, a.title, COALESCE(a.url, ''), la.feed_url, COALESCE(f.title, ''),
		       COALESCE(f.favicon_url, ''),
		       COALESCE(a.author, ''), COALESCE(a.summary, ''), a.published,
		       COALESCE(rs.is_read, 0),
		       COALESCE(la.like_signal, 0) * ?
		     + COALESCE(sl.social, 0) * ?
		     + COALESCE(cb.score, 0) * ?
		     + EXP(-0.023 * CAST(julianday('now') - julianday(a.published) AS REAL)) * 0.2
		       AS score
		FROM liked_articles la
		JOIN articles.articles a ON a.feed_url = la.feed_url AND a.url = la.article_url
		LEFT JOIN articles.feeds f ON f.feed_url = la.feed_url
		LEFT JOIN social_likes sl ON sl.feed_url = la.feed_url AND sl.article_url = la.article_url
		LEFT JOIN _content_boost cb ON cb.article_id = a.id
		LEFT JOIN articles.read_state rs ON rs.article_id = a.id AND rs.user_did = ?
		WHERE COALESCE(rs.is_read, 0) = 0 %s
		ORDER BY score DESC, (CASE WHEN a.published > 'now' THEN 1 ELSE 0 END), a.published DESC
		LIMIT ?
	`, langFilter)

	args := []any{userDID, userDID, userDID, userDID, userDID, userDID,
		w.WLike, w.WSocial, w.WContent, userDID}
	args = append(args, langArgs...)
	args = append(args, limit)

	rows, err := conn.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var recs []*ArticleRecommendation
	for rows.Next() {
		rec := &ArticleRecommendation{}
		if err := rows.Scan(&rec.ArticleID, &rec.Title, &rec.URL, &rec.FeedURL, &rec.FeedTitle,
			&rec.FaviconURL, &rec.Author, &rec.Summary, &rec.Published, &rec.IsRead, &rec.Score); err != nil {
			return nil, err
		}
		recs = append(recs, rec)
	}
	return recs, rows.Err()
}

func (e *Engine) ComputePeopleRecommendationsOnDemand(ctx context.Context, userDID string, limit int) ([]*PersonRecommendation, error) {
	rows, err := e.db.QueryContext(ctx, `
		SELECT u.did,
		       sim.jaccard, sim.common_feeds, COALESCE(sim.common_likes, 0), COALESCE(sim.common_tags, 0),
		       CASE WHEN f.target_did IS NOT NULL THEN 1 ELSE 0 END
		FROM (
			SELECT user_b AS peer_did, jaccard, common_feeds, common_likes, common_tags FROM recs.user_similarity WHERE user_a = ?
			UNION ALL
			SELECT user_a AS peer_did, jaccard, common_feeds, common_likes, common_tags FROM recs.user_similarity WHERE user_b = ?
		) sim
		JOIN main.users u ON u.did = sim.peer_did
		LEFT JOIN main.follows f ON f.user_did = ? AND f.target_did = u.did
		WHERE EXISTS (SELECT 1 FROM articles.subscriptions s JOIN articles.feeds f ON s.feed_url = f.feed_url WHERE s.user_did = u.did AND f.subscriber_count > 0)
		  AND NOT EXISTS (SELECT 1 FROM main.dismissed_recommendations d WHERE d.user_did = ? AND d.target_type = 'person' AND d.target_id = u.did)
		ORDER BY sim.jaccard DESC
		LIMIT ?
	`, userDID, userDID, userDID, userDID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*PersonRecommendation
	for rows.Next() {
		rec := &PersonRecommendation{}
		var isFollowed int
		if err := rows.Scan(&rec.DID,
			&rec.Jaccard, &rec.CommonFeeds, &rec.CommonLikes, &rec.CommonTags,
			&isFollowed); err != nil {
			return nil, err
		}
		rec.IsFollowed = isFollowed == 1
		results = append(results, rec)
	}
	return results, rows.Err()
}

func (e *Engine) ComputeSignalProfiles(ctx context.Context) error {
	conn, err := e.db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()

	{
		tx, err := conn.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer func() { _ = tx.Rollback() }()

		if _, err := tx.ExecContext(ctx, `
			CREATE TEMP TABLE IF NOT EXISTS _user_like_counts (user_did TEXT PRIMARY KEY, cnt INT)
		`); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM _user_like_counts`); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO _user_like_counts SELECT author_did, COUNT(*) FROM articles.likes GROUP BY author_did
		`); err != nil {
			return err
		}

		if _, err := tx.ExecContext(ctx, `
			CREATE TEMP TABLE IF NOT EXISTS _user_tag_counts (user_did TEXT PRIMARY KEY, cnt INT)
		`); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM _user_tag_counts`); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO _user_tag_counts
			WITH user_tags AS (
				SELECT author_did, TRIM(value) AS tag
				FROM articles.annotations, json_each('["' || REPLACE(tags, ',', '","') || '"]')
				WHERE tags IS NOT NULL AND tags != ''
			)
			SELECT author_did, COUNT(DISTINCT tag) FROM user_tags GROUP BY author_did
		`); err != nil {
			return err
		}

		if _, err := tx.ExecContext(ctx, `
			CREATE TEMP TABLE IF NOT EXISTS _user_top_categories (user_did TEXT PRIMARY KEY, categories TEXT)
		`); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM _user_top_categories`); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO _user_top_categories
			SELECT user_did, '[' || GROUP_CONCAT('{"c":"' || category || '","n":"' || CAST(cnt AS TEXT) || '}') || ']'
			FROM (
				SELECT user_did, category, COUNT(*) AS cnt
				FROM articles.subscriptions
				WHERE category IS NOT NULL AND category != ''
				GROUP BY user_did, category
				ORDER BY COUNT(*) DESC
				LIMIT 5
			)
			GROUP BY user_did
		`); err != nil {
			return err
		}

		if _, err := tx.ExecContext(ctx, `
			CREATE TEMP TABLE IF NOT EXISTS _signal_profiles_staging (
				user_did TEXT PRIMARY KEY, total_likes INT, total_tags INT, top_categories TEXT
			)
		`); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM _signal_profiles_staging`); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO _signal_profiles_staging (user_did, total_likes, total_tags, top_categories)
			SELECT
				u.did,
				COALESCE(lc.cnt, 0),
				COALESCE(tc.cnt, 0),
				COALESCE(cc.categories, '[]')
			FROM main.users u
			LEFT JOIN _user_like_counts lc ON lc.user_did = u.did
			LEFT JOIN _user_tag_counts tc ON tc.user_did = u.did
			LEFT JOIN _user_top_categories cc ON cc.user_did = u.did
		`); err != nil {
			return err
		}

		if err := tx.Commit(); err != nil {
			return err
		}
	}

	{
		tx, err := conn.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer func() { _ = tx.Rollback() }()

		if _, err := tx.ExecContext(ctx, `DELETE FROM recs.user_signal_profiles`); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO recs.user_signal_profiles (user_did, total_likes, total_tags, top_categories) SELECT user_did, total_likes, total_tags, top_categories FROM _signal_profiles_staging`); err != nil {
			return err
		}

		e.logger.Info("signal profiles computed")
		return tx.Commit()
	}
}

func (e *Engine) ColdStartRecommendations(ctx context.Context, userDID string, limit int) ([]*FeedRecommendation, error) {
	subCount := 0
	_ = e.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM articles.subscriptions WHERE user_did = ?`, userDID).Scan(&subCount)
	if subCount >= 5 {
		return nil, nil
	}

	recs, err := e.coldStartFromEmbeddings(ctx, userDID, limit)
	if err != nil {
		e.logger.Warn("embedding cold start failed", "error", err)
	}
	if len(recs) > 0 {
		return recs, nil
	}

	return e.coldStartFromGraphAndPopular(ctx, userDID, limit)
}

func (e *Engine) coldStartFromGraphAndPopular(ctx context.Context, userDID string, limit int) ([]*FeedRecommendation, error) {
	rows, err := e.db.QueryContext(ctx, `
		WITH followed_feeds AS (
			SELECT s.feed_url, 1.0 AS weight
			FROM recs.follow_distances fd
			JOIN articles.subscriptions s ON s.user_did = fd.user_b
			WHERE fd.user_a = ? AND fd.distance = 1
			AND s.feed_url NOT IN (SELECT feed_url FROM articles.subscriptions WHERE user_did = ?)
			AND s.feed_url NOT IN (SELECT target_id FROM main.dismissed_recommendations WHERE user_did = ? AND target_type = 'feed')
		),
		popular_feeds AS (
			SELECT feed_url, subscriber_count,
				LOG(1 + CAST(subscriber_count AS REAL)) / LOG(1 + CAST((SELECT COALESCE(MAX(subscriber_count), 1) FROM articles.feeds) AS REAL)) AS pop_score
			FROM articles.feeds
			WHERE subscriber_count > 0
			AND feed_url NOT IN (SELECT feed_url FROM articles.subscriptions WHERE user_did = ?)
			AND feed_url NOT IN (SELECT target_id FROM main.dismissed_recommendations WHERE user_did = ? AND target_type = 'feed')
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
		JOIN articles.feeds f ON f.feed_url = ac.feed_url
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
