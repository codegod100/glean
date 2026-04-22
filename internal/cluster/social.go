package cluster

import (
	"context"
)

func (e *Engine) ComputeFollowDistances(ctx context.Context) error {
	tx, err := e.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `DELETE FROM follow_distances`); err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO follow_distances (user_a, user_b, distance)
		SELECT user_did, target_did, 1
		FROM follows
		WHERE user_did != target_did
	`); err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT OR IGNORE INTO follow_distances (user_a, user_b, distance)
		SELECT f1.user_did, f2.target_did, 2
		FROM follows f1
		JOIN follows f2 ON f1.target_did = f2.user_did
		WHERE f1.user_did != f2.target_did
	`); err != nil {
		return err
	}

	e.logger.Info("follow distances computed")
	return tx.Commit()
}

func (e *Engine) ComputeFollowDistancesIncremental(ctx context.Context) error {
	var maxFollowed string
	err := e.db.QueryRowContext(ctx, `
		SELECT COALESCE(MAX(followed_at), '1970-01-01') FROM follows
	`).Scan(&maxFollowed)
	if err != nil {
		return err
	}

	var lastComputed string
	err = e.db.QueryRowContext(ctx, `
		SELECT COALESCE(MAX(updated_at), '1970-01-01') FROM user_similarity
	`).Scan(&lastComputed)
	if err != nil {
		return err
	}

	if maxFollowed <= lastComputed {
		return nil
	}

	return e.ComputeFollowDistances(ctx)
}
