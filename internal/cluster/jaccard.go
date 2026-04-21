package cluster

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"sync"
)

type Config struct {
	SimilarityThreshold float64
	FollowBoost         float64
	LikesWeight         float64
	TagsWeight          float64
	DescriptionWeight   float64
}

func DefaultConfig() Config {
	return Config{
		SimilarityThreshold: 0.2,
		FollowBoost:         0.5,
		LikesWeight:         0.3,
		TagsWeight:          0.2,
		DescriptionWeight:   0.15,
	}
}

type Engine struct {
	db     *sql.DB
	logger *slog.Logger
	mu     sync.Mutex
	config Config
}

func NewEngine(db *sql.DB, logger *slog.Logger) *Engine {
	return &Engine{db: db, logger: logger, config: DefaultConfig()}
}

func (e *Engine) ComputeArticleRecommendations(ctx context.Context) error {
	tx, err := e.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `DELETE FROM user_article_recommendations`); err != nil {
		return err
	}

	query := fmt.Sprintf(`
		INSERT INTO user_article_recommendations (user_did, feed_url, article_url, score)
		SELECT targets.target, l.feed_url, l.article_url, SUM(targets.jaccard) AS score
		FROM (
			SELECT us.user_a AS target, us.user_b AS peer, us.jaccard
			FROM user_similarity us
			WHERE us.jaccard > %g
			UNION ALL
			SELECT us.user_b AS target, us.user_a AS peer, us.jaccard
			FROM user_similarity us
			WHERE us.jaccard > %g
		) targets
		JOIN likes l ON l.author_did = targets.peer
		WHERE NOT EXISTS (
			SELECT 1 FROM subscriptions sub WHERE sub.user_did = targets.target AND sub.feed_url = l.feed_url
		)
		AND NOT EXISTS (
			SELECT 1 FROM likes ul WHERE ul.author_did = targets.target AND ul.feed_url = l.feed_url AND ul.article_url = l.article_url
		)
		GROUP BY targets.target, l.feed_url, l.article_url
		HAVING COUNT(*) > 0
		ORDER BY score DESC
	`, e.config.SimilarityThreshold, e.config.SimilarityThreshold)

	if _, err := tx.ExecContext(ctx, query); err != nil {
		return err
	}

	e.logger.Info("article recommendations computed")
	return tx.Commit()
}

func (e *Engine) ComputeForUser(ctx context.Context, userDID string) {
	if !e.mu.TryLock() {
		e.logger.Info("skipping ComputeForUser: already in progress", "did", userDID)
		return
	}
	defer e.mu.Unlock()

	if err := e.ComputeUserSimilarityForUser(ctx, userDID); err != nil {
		e.logger.Error("per-user similarity failed", "error", err, "did", userDID)
	}
	if err := e.ComputeRecommendationsForUser(ctx, userDID); err != nil {
		e.logger.Error("per-user recommendations failed", "error", err, "did", userDID)
	}
}

func (e *Engine) ComputeFeedSimilarity(ctx context.Context) error {
	tx, err := e.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `DELETE FROM feed_similarity`); err != nil {
		return err
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO feed_similarity (feed_a, feed_b, jaccard)
		SELECT
			s1.feed_url,
			s2.feed_url,
			CAST(COUNT(*) AS REAL) / (f1.subscriber_count + f2.subscriber_count - CAST(COUNT(*) AS REAL))
		FROM subscriptions s1
		JOIN subscriptions s2 ON s1.user_did = s2.user_did AND s1.feed_url < s2.feed_url
		JOIN feeds f1 ON f1.feed_url = s1.feed_url
		JOIN feeds f2 ON f2.feed_url = s2.feed_url
		GROUP BY s1.feed_url, s2.feed_url
		HAVING COUNT(*) > 0
	`)
	if err != nil {
		return err
	}

	if err := e.computeDescriptionSimilarity(ctx, tx); err != nil {
		e.logger.Warn("description similarity failed", "error", err)
	}

	e.logger.Info("feed similarity computed")
	return tx.Commit()
}

func (e *Engine) computeDescriptionSimilarity(ctx context.Context, tx *sql.Tx) error {
	if _, err := tx.ExecContext(ctx, `CREATE TEMP TABLE IF NOT EXISTS _feed_words (feed_url TEXT, word TEXT)`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM _feed_words`); err != nil {
		return err
	}

	_, err := tx.ExecContext(ctx, `
		INSERT INTO _feed_words (feed_url, word)
		WITH feed_tokens AS (
			SELECT feed_url, LOWER(TRIM(value)) AS word
			FROM feeds,
			json_each('["' || REPLACE(LOWER(COALESCE(description, '')), ' ', '","') || '"]')
			WHERE description IS NOT NULL AND description != ''
		)
		SELECT feed_url, word FROM feed_tokens
		WHERE LENGTH(word) > 3
		AND word NOT IN ('about','also','been','being','both','could','every','from','have','here',
			'into','just','like','more','much','must','other','over','some','such','than','that',
			'their','them','then','there','these','they','this','through','very','what','when',
			'where','which','while','will','with','your','most','updated','latest','posts',
			'news','blog','feed','reading','read','articles','article','weekly','daily',
			'monthly','personal','thoughts','views','opinions','writing','write','written')
	`)
	if err != nil {
		return err
	}

	descUpdate := fmt.Sprintf(`
		UPDATE feed_similarity SET
			jaccard = jaccard + %g * CAST(word_overlap.common AS REAL) / NULLIF(
				(SELECT COUNT(DISTINCT word) FROM _feed_words WHERE feed_url = feed_similarity.feed_a) +
				(SELECT COUNT(DISTINCT word) FROM _feed_words WHERE feed_url = feed_similarity.feed_b) -
				CAST(word_overlap.common AS REAL),
				0
			)
		FROM (
			SELECT w1.feed_url AS feed_a, w2.feed_url AS feed_b, COUNT(DISTINCT w1.word) AS common
			FROM _feed_words w1
			JOIN _feed_words w2 ON w1.word = w2.word AND w1.feed_url < w2.feed_url
			GROUP BY w1.feed_url, w2.feed_url
			HAVING common > 1
		) word_overlap
		WHERE feed_similarity.feed_a = word_overlap.feed_a
		  AND feed_similarity.feed_b = word_overlap.feed_b
	`, e.config.DescriptionWeight)

	if _, err := tx.ExecContext(ctx, descUpdate); err != nil {
		return err
	}

	descInsert := fmt.Sprintf(`
		INSERT OR IGNORE INTO feed_similarity (feed_a, feed_b, jaccard)
		SELECT feed_a, feed_b, 0 FROM (
			SELECT w1.feed_url AS feed_a, w2.feed_url AS feed_b, COUNT(DISTINCT w1.word) AS common
			FROM _feed_words w1
			JOIN _feed_words w2 ON w1.word = w2.word AND w1.feed_url < w2.feed_url
			GROUP BY w1.feed_url, w2.feed_url
			HAVING common > 1
		)
	`)
	if _, err := tx.ExecContext(ctx, descInsert); err != nil {
		return err
	}

	descBoost := fmt.Sprintf(`
		UPDATE feed_similarity SET
			jaccard = jaccard + %g * CAST(word_overlap.common AS REAL) / NULLIF(
				(SELECT COUNT(DISTINCT word) FROM _feed_words WHERE feed_url = feed_similarity.feed_a) +
				(SELECT COUNT(DISTINCT word) FROM _feed_words WHERE feed_url = feed_similarity.feed_b) -
				CAST(word_overlap.common AS REAL),
				0
			)
		FROM (
			SELECT w1.feed_url AS feed_a, w2.feed_url AS feed_b, COUNT(DISTINCT w1.word) AS common
			FROM _feed_words w1
			JOIN _feed_words w2 ON w1.word = w2.word AND w1.feed_url < w2.feed_url
			GROUP BY w1.feed_url, w2.feed_url
			HAVING common > 1
		) word_overlap
		WHERE feed_similarity.feed_a = word_overlap.feed_a
		  AND feed_similarity.feed_b = word_overlap.feed_b
		  AND feed_similarity.jaccard = 0
	`, e.config.DescriptionWeight)

	_, err = tx.ExecContext(ctx, descBoost)
	return err
}

func (e *Engine) ComputeUserSimilarity(ctx context.Context) error {
	tx, err := e.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `DELETE FROM user_similarity`); err != nil {
		return err
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO user_similarity (user_a, user_b, jaccard, common_feeds)
		SELECT
			s1.user_did,
			s2.user_did,
			CAST(COUNT(*) AS REAL) / (
				(SELECT COUNT(*) FROM subscriptions WHERE user_did = s1.user_did) +
				(SELECT COUNT(*) FROM subscriptions WHERE user_did = s2.user_did) -
				CAST(COUNT(*) AS REAL)
			),
			COUNT(*)
		FROM subscriptions s1
		JOIN subscriptions s2 ON s1.feed_url = s2.feed_url AND s1.user_did < s2.user_did
		GROUP BY s1.user_did, s2.user_did
		HAVING COUNT(*) > 0
	`)
	if err != nil {
		return err
	}

	likesUpdate := fmt.Sprintf(`
		UPDATE user_similarity SET
			jaccard = jaccard + %g * CAST(likes_overlap.common AS REAL) / NULLIF(
				(SELECT COUNT(*) FROM likes WHERE author_did = user_similarity.user_a) +
				(SELECT COUNT(*) FROM likes WHERE author_did = user_similarity.user_b) -
				CAST(likes_overlap.common AS REAL),
				0
			),
			common_likes = likes_overlap.common
		FROM (
			SELECT l1.author_did AS user_a, l2.author_did AS user_b, COUNT(*) AS common
			FROM likes l1
			JOIN likes l2 ON l1.feed_url = l2.feed_url AND l1.article_url = l2.article_url
				AND l1.author_did < l2.author_did
			GROUP BY l1.author_did, l2.author_did
			HAVING common > 0
		) likes_overlap
		WHERE user_similarity.user_a = likes_overlap.user_a
		  AND user_similarity.user_b = likes_overlap.user_b
	`, e.config.LikesWeight)

	if _, err := tx.ExecContext(ctx, likesUpdate); err != nil {
		return err
	}

	likesInsert := fmt.Sprintf(`
		INSERT INTO user_similarity (user_a, user_b, jaccard, common_feeds, common_likes)
		SELECT
			l1.author_did,
			l2.author_did,
			%g * CAST(COUNT(*) AS REAL) / NULLIF(
				(SELECT COUNT(*) FROM likes WHERE author_did = l1.author_did) +
				(SELECT COUNT(*) FROM likes WHERE author_did = l2.author_did) -
				CAST(COUNT(*) AS REAL),
				0
			),
			0,
			COUNT(*)
		FROM likes l1
		JOIN likes l2 ON l1.feed_url = l2.feed_url AND l1.article_url = l2.article_url
			AND l1.author_did < l2.author_did
		GROUP BY l1.author_did, l2.author_did
		HAVING COUNT(*) > 0
		ON CONFLICT(user_a, user_b) DO UPDATE SET
			jaccard = jaccard + excluded.jaccard,
			common_likes = excluded.common_likes
	`, e.config.LikesWeight)

	if _, err := tx.ExecContext(ctx, likesInsert); err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, `CREATE TEMP TABLE IF NOT EXISTS _tag_overlap (user_a TEXT, user_b TEXT, common INT)`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM _tag_overlap`); err != nil {
		return err
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO _tag_overlap (user_a, user_b, common)
		WITH user_tags AS (
			SELECT author_did, TRIM(value) AS tag FROM annotations, json_each('["' || REPLACE(tags, ',', '","') || '"]')
			WHERE tags IS NOT NULL AND tags != ''
		)
		SELECT t1.author_did, t2.author_did, COUNT(DISTINCT t1.tag)
		FROM user_tags t1
		JOIN user_tags t2 ON t1.tag = t2.tag AND t1.author_did < t2.author_did
		GROUP BY t1.author_did, t2.author_did
		HAVING COUNT(DISTINCT t1.tag) > 0
	`)
	if err != nil {
		return err
	}

	_, err = tx.ExecContext(ctx, `
		INSERT OR IGNORE INTO user_similarity (user_a, user_b, jaccard, common_feeds, common_tags)
		SELECT user_a, user_b, 0, 0, 0 FROM _tag_overlap
	`)
	if err != nil {
		return err
	}

	tagsUpdate := fmt.Sprintf(`
		UPDATE user_similarity SET
			jaccard = jaccard + %g * CAST(_tag_overlap.common AS REAL) / NULLIF(
				(SELECT COUNT(DISTINCT TRIM(value)) FROM annotations a, json_each('["' || REPLACE(a.tags, ',', '","') || '"]') WHERE a.author_did = user_similarity.user_a AND a.tags IS NOT NULL AND a.tags != '') +
				(SELECT COUNT(DISTINCT TRIM(value)) FROM annotations a, json_each('["' || REPLACE(a.tags, ',', '","') || '"]') WHERE a.author_did = user_similarity.user_b AND a.tags IS NOT NULL AND a.tags != '') -
				CAST(_tag_overlap.common AS REAL),
				0
			),
			common_tags = _tag_overlap.common
		FROM _tag_overlap
		WHERE user_similarity.user_a = _tag_overlap.user_a
		  AND user_similarity.user_b = _tag_overlap.user_b
	`, e.config.TagsWeight)

	if _, err := tx.ExecContext(ctx, tagsUpdate); err != nil {
		return err
	}

	followQuery := fmt.Sprintf(`
		INSERT INTO user_similarity (user_a, user_b, jaccard, common_feeds, common_likes, common_tags)
		SELECT
			MIN(f.user_did, f.target_did),
			MAX(f.user_did, f.target_did),
			%g,
			0, 0, 0
		FROM follows f
		WHERE f.user_did != f.target_did
		GROUP BY MIN(f.user_did, f.target_did), MAX(f.user_did, f.target_did)
		ON CONFLICT(user_a, user_b) DO UPDATE SET
			jaccard = jaccard + %g
	`, e.config.FollowBoost, e.config.FollowBoost)

	if _, err := tx.ExecContext(ctx, followQuery); err != nil {
		return err
	}

	e.logger.Info("user similarity computed")
	return tx.Commit()
}

func (e *Engine) ComputeUserSimilarityForUser(ctx context.Context, userDID string) error {
	tx, err := e.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `DELETE FROM user_similarity WHERE user_a = ? OR user_b = ?`, userDID, userDID); err != nil {
		return err
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO user_similarity (user_a, user_b, jaccard, common_feeds)
		SELECT
			MIN(?, s2.user_did),
			MAX(?, s2.user_did),
			CAST(COUNT(*) AS REAL) / (
				(SELECT COUNT(*) FROM subscriptions WHERE user_did = ?) +
				(SELECT COUNT(*) FROM subscriptions WHERE user_did = s2.user_did) -
				CAST(COUNT(*) AS REAL)
			),
			COUNT(*)
		FROM subscriptions s1
		JOIN subscriptions s2 ON s1.feed_url = s2.feed_url AND s2.user_did != ?
		WHERE s1.user_did = ?
		GROUP BY s2.user_did
		HAVING COUNT(*) > 0
	`, userDID, userDID, userDID, userDID, userDID)
	if err != nil {
		return err
	}

	likesPerUser := fmt.Sprintf(`
		UPDATE user_similarity SET
			jaccard = jaccard + %g * CAST(likes_overlap.common AS REAL) / NULLIF(
				(SELECT COUNT(*) FROM likes WHERE author_did = ?) +
				(SELECT COUNT(*) FROM likes WHERE author_did =
					CASE WHEN user_similarity.user_a = ? THEN user_similarity.user_b ELSE user_similarity.user_a END
				) - CAST(likes_overlap.common AS REAL),
				0
			),
			common_likes = likes_overlap.common
		FROM (
			SELECT l2.author_did AS peer, COUNT(*) AS common
			FROM likes l1
			JOIN likes l2 ON l1.feed_url = l2.feed_url AND l1.article_url = l2.article_url
				AND l2.author_did != ?
			WHERE l1.author_did = ?
			GROUP BY l2.author_did
			HAVING common > 0
		) likes_overlap
		WHERE user_similarity.user_a = likes_overlap.peer
		   OR user_similarity.user_b = likes_overlap.peer
	`, e.config.LikesWeight)

	if _, err := tx.ExecContext(ctx, likesPerUser, userDID, userDID, userDID, userDID); err != nil {
		return err
	}

	likesInsert := fmt.Sprintf(`
		INSERT INTO user_similarity (user_a, user_b, jaccard, common_feeds, common_likes)
		SELECT
			MIN(?, l2.author_did),
			MAX(?, l2.author_did),
			%g * CAST(COUNT(*) AS REAL) / NULLIF(
				(SELECT COUNT(*) FROM likes WHERE author_did = ?) +
				(SELECT COUNT(*) FROM likes WHERE author_did = l2.author_did) -
				CAST(COUNT(*) AS REAL),
				0
			),
			0,
			COUNT(*)
		FROM likes l1
		JOIN likes l2 ON l1.feed_url = l2.feed_url AND l1.article_url = l2.article_url
			AND l2.author_did != ?
		WHERE l1.author_did = ?
		GROUP BY l2.author_did
		HAVING COUNT(*) > 0
		ON CONFLICT(user_a, user_b) DO UPDATE SET
			jaccard = jaccard + excluded.jaccard,
			common_likes = excluded.common_likes
	`, e.config.LikesWeight)

	if _, err := tx.ExecContext(ctx, likesInsert, userDID, userDID, userDID, userDID, userDID); err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, `CREATE TEMP TABLE IF NOT EXISTS _user_tag_overlap (peer TEXT, common INT)`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM _user_tag_overlap`); err != nil {
		return err
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO _user_tag_overlap (peer, common)
		WITH user_tags AS (
			SELECT author_did, TRIM(value) AS tag FROM annotations, json_each('["' || REPLACE(tags, ',', '","') || '"]')
			WHERE tags IS NOT NULL AND tags != ''
		)
		SELECT t2.author_did, COUNT(DISTINCT t1.tag)
		FROM user_tags t1
		JOIN user_tags t2 ON t1.tag = t2.tag AND t2.author_did != ?
		WHERE t1.author_did = ?
		GROUP BY t2.author_did
		HAVING COUNT(DISTINCT t1.tag) > 0
	`, userDID, userDID)
	if err != nil {
		return err
	}

	_, err = tx.ExecContext(ctx, `
		INSERT OR IGNORE INTO user_similarity (user_a, user_b, jaccard, common_feeds, common_tags)
		SELECT MIN(?, peer), MAX(?, peer), 0, 0, 0 FROM _user_tag_overlap
	`, userDID, userDID)
	if err != nil {
		return err
	}

	tagsPerUser := fmt.Sprintf(`
		UPDATE user_similarity SET
			jaccard = jaccard + %g * CAST(_user_tag_overlap.common AS REAL) / NULLIF(
				(SELECT COUNT(DISTINCT TRIM(value)) FROM annotations a, json_each('["' || REPLACE(a.tags, ',', '","') || '"]') WHERE a.author_did = ? AND a.tags IS NOT NULL AND a.tags != '') +
				(SELECT COUNT(DISTINCT TRIM(value)) FROM annotations a, json_each('["' || REPLACE(a.tags, ',', '","') || '"]') WHERE a.author_did =
					CASE WHEN user_similarity.user_a = ? THEN user_similarity.user_b ELSE user_similarity.user_a END
				AND a.tags IS NOT NULL AND a.tags != '') -
				CAST(_user_tag_overlap.common AS REAL),
				0
			),
			common_tags = _user_tag_overlap.common
		FROM _user_tag_overlap
		WHERE user_similarity.user_a = _user_tag_overlap.peer
		   OR user_similarity.user_b = _user_tag_overlap.peer
	`, e.config.TagsWeight)

	if _, err := tx.ExecContext(ctx, tagsPerUser, userDID, userDID); err != nil {
		return err
	}

	followQuery := fmt.Sprintf(`
		INSERT INTO user_similarity (user_a, user_b, jaccard, common_feeds, common_likes, common_tags)
		SELECT
			MIN(?, f.target_did),
			MAX(?, f.target_did),
			%g,
			0, 0, 0
		FROM follows f
		WHERE f.user_did = ? AND f.target_did != ?
		GROUP BY MIN(?, f.target_did), MAX(?, f.target_did)
		ON CONFLICT(user_a, user_b) DO UPDATE SET
			jaccard = jaccard + %g
	`, e.config.FollowBoost, e.config.FollowBoost)

	if _, err := tx.ExecContext(ctx, followQuery, userDID, userDID, userDID, userDID, userDID, userDID); err != nil {
		return err
	}

	e.logger.Info("per-user similarity computed", "did", userDID)
	return tx.Commit()
}

func (e *Engine) ComputeRecommendationsForUser(ctx context.Context, userDID string) error {
	tx, err := e.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `DELETE FROM user_feed_recommendations WHERE user_did = ?`, userDID); err != nil {
		return err
	}

	recQuery := fmt.Sprintf(`
		INSERT INTO user_feed_recommendations (user_did, feed_url, score)
		SELECT ?, s.feed_url, SUM(us.jaccard) AS score
		FROM user_similarity us
		JOIN subscriptions s ON s.user_did = CASE
			WHEN us.user_a = ? THEN us.user_b
			ELSE us.user_a
		END
		WHERE (us.user_a = ? OR us.user_b = ?)
		AND us.jaccard > %g
		AND s.feed_url NOT IN (SELECT feed_url FROM subscriptions WHERE user_did = ?)
		GROUP BY s.feed_url
		ORDER BY score DESC
	`, e.config.SimilarityThreshold)

	if _, err := tx.ExecContext(ctx, recQuery, userDID, userDID, userDID, userDID, userDID); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	if err := e.computeArticleRecommendationsForUser(ctx, userDID); err != nil {
		return err
	}

	e.logger.Info("per-user recommendations computed", "did", userDID)
	return nil
}

func (e *Engine) computeArticleRecommendationsForUser(ctx context.Context, userDID string) error {
	tx, err := e.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `DELETE FROM user_article_recommendations WHERE user_did = ?`, userDID); err != nil {
		return err
	}

	artQuery := fmt.Sprintf(`
		INSERT INTO user_article_recommendations (user_did, feed_url, article_url, score)
		SELECT ?, l.feed_url, l.article_url, SUM(us.jaccard) AS score
		FROM (
			SELECT us.user_b AS peer, us.jaccard
			FROM user_similarity us WHERE us.user_a = ? AND us.jaccard > %g
			UNION ALL
			SELECT us.user_a AS peer, us.jaccard
			FROM user_similarity us WHERE us.user_b = ? AND us.jaccard > %g
		) us
		JOIN likes l ON l.author_did = us.peer
		WHERE NOT EXISTS (
			SELECT 1 FROM subscriptions sub WHERE sub.user_did = ? AND sub.feed_url = l.feed_url
		)
		AND NOT EXISTS (
			SELECT 1 FROM likes ul WHERE ul.author_did = ? AND ul.feed_url = l.feed_url AND ul.article_url = l.article_url
		)
		GROUP BY l.feed_url, l.article_url
		HAVING COUNT(*) > 0
		ORDER BY score DESC
	`, e.config.SimilarityThreshold, e.config.SimilarityThreshold)

	if _, err := tx.ExecContext(ctx, artQuery, userDID, userDID, userDID, userDID, userDID); err != nil {
		return err
	}

	return tx.Commit()
}

func (e *Engine) ComputeRecommendations(ctx context.Context) error {
	tx, err := e.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `DELETE FROM user_feed_recommendations`); err != nil {
		return err
	}

	recQuery := fmt.Sprintf(`
		INSERT INTO user_feed_recommendations (user_did, feed_url, score)
		SELECT target, feed_url, SUM(jaccard) AS score
		FROM (
			SELECT us.user_a AS target, s.feed_url, us.jaccard
			FROM user_similarity us
			JOIN subscriptions s ON s.user_did = us.user_b
			WHERE us.jaccard > %g
			AND s.feed_url NOT IN (SELECT feed_url FROM subscriptions WHERE user_did = us.user_a)

			UNION ALL

			SELECT us.user_b AS target, s.feed_url, us.jaccard
			FROM user_similarity us
			JOIN subscriptions s ON s.user_did = us.user_a
			WHERE us.jaccard > %g
			AND s.feed_url NOT IN (SELECT feed_url FROM subscriptions WHERE user_did = us.user_b)
		)
		GROUP BY target, feed_url
		ORDER BY score DESC
	`, e.config.SimilarityThreshold, e.config.SimilarityThreshold)

	if _, err := tx.ExecContext(ctx, recQuery); err != nil {
		return err
	}

	e.logger.Info("feed recommendations computed")

	if err := tx.Commit(); err != nil {
		return err
	}

	return e.ComputeArticleRecommendations(ctx)
}
