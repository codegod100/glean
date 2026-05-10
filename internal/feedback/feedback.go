package feedback

import (
	"context"
	"database/sql"
	"time"
)

type Impression struct {
	TargetType string
	TargetID   string
}

type Service struct {
	db *sql.DB
}

func NewService(db *sql.DB) *Service {
	return &Service{db: db}
}

func (s *Service) Dismiss(ctx context.Context, userDID, targetType, targetID, reason string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO main.dismissed_recommendations (user_did, target_type, target_id, reason)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(user_did, target_type, target_id) DO UPDATE SET reason = excluded.reason, dismissed_at = CURRENT_TIMESTAMP
	`, userDID, targetType, targetID, reason)
	return err
}

func (s *Service) DismissFeed(ctx context.Context, userDID, feedURL, reason string) error {
	return s.Dismiss(ctx, userDID, "feed", feedURL, reason)
}

func (s *Service) DismissArticle(ctx context.Context, userDID, articleURL, reason string) error {
	return s.Dismiss(ctx, userDID, "article", articleURL, reason)
}

func (s *Service) DismissPerson(ctx context.Context, userDID, targetDID, reason string) error {
	return s.Dismiss(ctx, userDID, "person", targetDID, reason)
}

func (s *Service) RecordImpressions(ctx context.Context, userDID string, impressions []Impression) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	for _, imp := range impressions {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO main.recommendation_impressions (user_did, target_type, target_id, first_shown_at, last_shown_at, shown_count)
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

func (s *Service) MarkImpressionActed(ctx context.Context, userDID, targetType, targetID string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE main.recommendation_impressions SET acted = 1
		WHERE user_did = ? AND target_type = ? AND target_id = ?
	`, userDID, targetType, targetID)
	return err
}

func (s *Service) AutoDismissStale(ctx context.Context, minShownCount int, maxAgeDays int) error {
	cutoff := time.Now().AddDate(0, 0, -maxAgeDays).Format(time.RFC3339)

	_, err := s.db.ExecContext(ctx, `
		INSERT OR IGNORE INTO main.dismissed_recommendations (user_did, target_type, target_id, reason, dismissed_at)
		SELECT user_did, target_type, target_id, 'auto_stale', CURRENT_TIMESTAMP
		FROM main.recommendation_impressions
		WHERE acted = 0
		  AND shown_count >= ?
		  AND first_shown_at < ?
	`, minShownCount, cutoff)
	return err
}

func (s *Service) IsFeedDismissed(ctx context.Context, userDID, feedURL string) (bool, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(1) FROM main.dismissed_recommendations
		WHERE user_did = ? AND target_type = 'feed' AND target_id = ?
	`, userDID, feedURL).Scan(&count)
	return count > 0, err
}
