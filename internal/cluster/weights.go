package cluster

import (
	"context"
)

const (
	learningRate   = 0.1
	weightMin      = 0.1
	weightMax      = 3.0
	minActionsTune = 5
)

func (e *Engine) RewardSignal(ctx context.Context, userDID string, signal string) {
	e.adjustWeight(ctx, userDID, signal, 1.0)
}

func (e *Engine) PenalizeSignal(ctx context.Context, userDID string, signal string) {
	e.adjustWeight(ctx, userDID, signal, -1.0)
}

func (e *Engine) adjustWeight(ctx context.Context, userDID string, signal string, delta float64) {
	var actedCount int
	_ = e.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM recs.recommendation_impressions WHERE user_did = ? AND acted = 1
	`, userDID).Scan(&actedCount)
	if actedCount < minActionsTune {
		return
	}

	var exists int
	_ = e.db.QueryRowContext(ctx, `SELECT 1 FROM recs.user_signal_weights WHERE user_did = ?`, userDID).Scan(&exists)

	if exists == 0 {
		_, _ = e.db.ExecContext(ctx, `
			INSERT INTO recs.user_signal_weights (user_did, w_sub, w_like, w_tag, w_social, w_pop, w_category)
			VALUES (?, 1.0, 0.5, 0.3, 0.7, 0.2, 0.4)
		`, userDID)
	}

	column := signalToColumn(signal)
	if column == "" {
		return
	}

	adj := learningRate * delta
	_, _ = e.db.ExecContext(ctx, `
		UPDATE recs.user_signal_weights SET
			`+column+` = MAX(?, MIN(?, `+column+` * (1 + ?))),
			updated_at = CURRENT_TIMESTAMP
		WHERE user_did = ?
	`, weightMin, weightMax, adj, userDID)
}

func signalToColumn(signal string) string {
	switch signal {
	case "sub":
		return "w_sub"
	case "like":
		return "w_like"
	case "tag":
		return "w_tag"
	case "social":
		return "w_social"
	case "pop":
		return "w_pop"
	case "category":
		return "w_category"
	default:
		return ""
	}
}

func (e *Engine) GetDominantSignal(w SignalWeights) string {
	signals := map[string]float64{
		"sub":      w.WSub,
		"like":     w.WLike,
		"tag":      w.WTag,
		"social":   w.WSocial,
		"pop":      w.WPop,
		"category": w.WCategory,
	}

	var best string
	bestVal := -1.0
	for s, v := range signals {
		if v > bestVal {
			bestVal = v
			best = s
		}
	}
	return best
}