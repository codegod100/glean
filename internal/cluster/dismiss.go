package cluster

import (
	"context"
	"time"
)

type Impression struct {
	TargetType string
	TargetID   string
}

func (e *Engine) DismissFeed(ctx context.Context, userDID, feedURL, reason string) error {
	return nil
	_, err := e.db.ExecContext(ctx, `
		INSERT INTO recs.dismissed_recommendations (user_did, target_type, target_id, reason)
		VALUES (?, 'feed', ?, ?)
		ON CONFLICT(user_did, target_type, target_id) DO UPDATE SET reason = excluded.reason, dismissed_at = CURRENT_TIMESTAMP
	`, userDID, feedURL, reason)
	return err
}

func (e *Engine) DismissArticle(ctx context.Context, userDID, articleURL, reason string) error {
	return nil
	_, err := e.db.ExecContext(ctx, `
		INSERT INTO recs.dismissed_recommendations (user_did, target_type, target_id, reason)
		VALUES (?, 'article', ?, ?)
		ON CONFLICT(user_did, target_type, target_id) DO UPDATE SET reason = excluded.reason, dismissed_at = CURRENT_TIMESTAMP
	`, userDID, articleURL, reason)
	return err
}

func (e *Engine) RecordImpressions(ctx context.Context, userDID string, impressions []Impression) error {
	return nil
	tx, err := e.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	for _, imp := range impressions {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO recs.recommendation_impressions (user_did, target_type, target_id, first_shown_at, last_shown_at, shown_count)
			VALUES (?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, 1)
			ON CONFLICT(user_did, target_type, target_id) DO UPDATE SET
				last_shown_at = CURRENT_TIMESTAMP,
				shown_count = shown_count + 1
		`, userDID, imp.TargetType, imp.TargetID)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (e *Engine) MarkImpressionActed(ctx context.Context, userDID, targetType, targetID string) error {
	return nil
	_, err := e.db.ExecContext(ctx, `
		UPDATE recs.recommendation_impressions SET acted = 1
		WHERE user_did = ? AND target_type = ? AND target_id = ?
	`, userDID, targetType, targetID)
	return err
}

func (e *Engine) AutoDismissStale(ctx context.Context, minShownCount int, maxAgeDays int) error {
	cutoff := time.Now().AddDate(0, 0, -maxAgeDays).Format(time.RFC3339)

	_, err := e.db.ExecContext(ctx, `
		INSERT OR IGNORE INTO recs.dismissed_recommendations (user_did, target_type, target_id, reason, dismissed_at)
		SELECT user_did, target_type, target_id, 'auto_stale', CURRENT_TIMESTAMP
		FROM recs.recommendation_impressions
		WHERE acted = 0
		  AND shown_count >= ?
		  AND first_shown_at < ?
	`, minShownCount, cutoff)
	return err
}

func (e *Engine) IsFeedDismissed(ctx context.Context, userDID, feedURL string) (bool, error) {
	var count int
	err := e.db.QueryRowContext(ctx, `
		SELECT COUNT(1) FROM recs.dismissed_recommendations
		WHERE user_did = ? AND target_type = 'feed' AND target_id = ?
	`, userDID, feedURL).Scan(&count)
	return count > 0, err
}
