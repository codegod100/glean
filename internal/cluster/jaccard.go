package cluster

import (
	"context"
	"database/sql"
	"log/slog"
	"sync"
)

type Engine struct {
	db     *sql.DB
	logger *slog.Logger
	mu     sync.Mutex
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

	_, err = tx.ExecContext(ctx, `
		INSERT INTO user_article_recommendations (user_did, feed_url, article_url, score)
		SELECT targets.target, l.feed_url, l.article_url, SUM(targets.jaccard) AS score
		FROM (
			SELECT us.user_a AS target, us.user_b AS peer, us.jaccard
			FROM user_similarity us
			WHERE us.jaccard > 0.2
			UNION ALL
			SELECT us.user_b AS target, us.user_a AS peer, us.jaccard
			FROM user_similarity us
			WHERE us.jaccard > 0.2
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
	`)
	if err != nil {
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

func NewEngine(db *sql.DB, logger *slog.Logger) *Engine {
	return &Engine{db: db, logger: logger}
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

	e.logger.Info("feed similarity computed")
	return tx.Commit()
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

	_, err = tx.ExecContext(ctx, `
		INSERT INTO user_similarity (user_a, user_b, jaccard, common_feeds)
		SELECT
			MIN(f.user_did, f.target_did),
			MAX(f.user_did, f.target_did),
			0.5,
			0
		FROM follows f
		WHERE f.user_did != f.target_did
		GROUP BY MIN(f.user_did, f.target_did), MAX(f.user_did, f.target_did)
		ON CONFLICT(user_a, user_b) DO UPDATE SET
			jaccard = jaccard + 0.5
	`)
	if err != nil {
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

	_, err = tx.ExecContext(ctx, `
		INSERT INTO user_similarity (user_a, user_b, jaccard, common_feeds)
		SELECT
			MIN(?, f.target_did),
			MAX(?, f.target_did),
			0.5,
			0
		FROM follows f
		WHERE f.user_did = ? AND f.target_did != ?
		GROUP BY MIN(?, f.target_did), MAX(?, f.target_did)
		ON CONFLICT(user_a, user_b) DO UPDATE SET
			jaccard = jaccard + 0.5
	`, userDID, userDID, userDID, userDID, userDID, userDID)
	if err != nil {
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

	_, err = tx.ExecContext(ctx, `
		INSERT INTO user_feed_recommendations (user_did, feed_url, score)
		SELECT ?, s.feed_url, SUM(us.jaccard) AS score
		FROM user_similarity us
		JOIN subscriptions s ON s.user_did = CASE
			WHEN us.user_a = ? THEN us.user_b
			ELSE us.user_a
		END
		WHERE (us.user_a = ? OR us.user_b = ?)
		AND us.jaccard > 0.2
		AND s.feed_url NOT IN (SELECT feed_url FROM subscriptions WHERE user_did = ?)
		GROUP BY s.feed_url
		ORDER BY score DESC
	`, userDID, userDID, userDID, userDID, userDID)
	if err != nil {
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

	_, err = tx.ExecContext(ctx, `
		INSERT INTO user_article_recommendations (user_did, feed_url, article_url, score)
		SELECT ?, l.feed_url, l.article_url, SUM(us.jaccard) AS score
		FROM (
			SELECT us.user_b AS peer, us.jaccard
			FROM user_similarity us WHERE us.user_a = ? AND us.jaccard > 0.2
			UNION ALL
			SELECT us.user_a AS peer, us.jaccard
			FROM user_similarity us WHERE us.user_b = ? AND us.jaccard > 0.2
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
	`, userDID, userDID, userDID, userDID, userDID)
	if err != nil {
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

	_, err = tx.ExecContext(ctx, `
		INSERT INTO user_feed_recommendations (user_did, feed_url, score)
		SELECT target, feed_url, SUM(jaccard) AS score
		FROM (
			SELECT us.user_a AS target, s.feed_url, us.jaccard
			FROM user_similarity us
			JOIN subscriptions s ON s.user_did = us.user_b
			WHERE us.jaccard > 0.2
			AND s.feed_url NOT IN (SELECT feed_url FROM subscriptions WHERE user_did = us.user_a)

			UNION ALL

			SELECT us.user_b AS target, s.feed_url, us.jaccard
			FROM user_similarity us
			JOIN subscriptions s ON s.user_did = us.user_a
			WHERE us.jaccard > 0.2
			AND s.feed_url NOT IN (SELECT feed_url FROM subscriptions WHERE user_did = us.user_b)
		)
		GROUP BY target, feed_url
		ORDER BY score DESC
	`)
	if err != nil {
		return err
	}

	e.logger.Info("feed recommendations computed")

	if err := tx.Commit(); err != nil {
		return err
	}

	return e.ComputeArticleRecommendations(ctx)
}
