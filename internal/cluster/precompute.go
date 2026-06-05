package cluster

import (
	"context"
	"encoding/json"
	"time"
)

func (e *Engine) PrecomputeAllRecommendations(ctx context.Context) error {
	e.logger.Info("starting recommendation precomputation")
	start := time.Now()

	users, err := e.listActiveUsers(ctx)
	if err != nil {
		return err
	}

	e.logger.Info("precomputing recommendations for users", "count", len(users))

	computed := 0
	for _, did := range users {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if err := e.precomputeForUser(ctx, did); err != nil {
			e.logger.Warn("precompute failed for user", "did", did, "error", err)
			continue
		}
		computed++
	}

	e.logger.Info("recommendation precomputation complete",
		"users", computed,
		"duration", time.Since(start),
	)
	return nil
}

func (e *Engine) precomputeForUser(ctx context.Context, userDID string) error {
	start := time.Now()

	feedRecs, err := e.ComputeFeedRecommendationsOnDemand(ctx, userDID, 10)
	if err != nil {
		return err
	}
	if len(feedRecs) > 0 {
		normalizeFeedScores(feedRecs)
		feedRecs = ApplyDiversity(feedRecs, 5)
	}

	articleRecs, err := e.ComputeArticleRecommendationsOnDemand(ctx, userDID, nil, 10)
	if err != nil {
		return err
	}
	if len(articleRecs) > 0 {
		normalizeArticleScores(articleRecs)
		if len(articleRecs) > 5 {
			articleRecs = articleRecs[:5]
		}
	}

	half := 3
	inNet, err := e.computePeopleByFollowStatus(ctx, userDID, true, half)
	if err != nil {
		return err
	}
	outNet, err := e.computePeopleByFollowStatus(ctx, userDID, false, half)
	if err != nil {
		return err
	}
	peopleRecs := append(inNet, outNet...)
	if len(peopleRecs) > 0 {
		normalizePersonScores(peopleRecs)
		if len(peopleRecs) > 6 {
			peopleRecs = peopleRecs[:6]
		}
	}

	e.storePrecomputed(ctx, userDID, "feed", mustJSON(feedRecs))
	e.storePrecomputed(ctx, userDID, "article", mustJSON(articleRecs))
	e.storePrecomputed(ctx, userDID, "person", mustJSON(peopleRecs))

	e.logger.Debug("precomputed recommendations for user",
		"did", userDID,
		"feeds", len(feedRecs),
		"articles", len(articleRecs),
		"people", len(peopleRecs),
		"duration", time.Since(start),
	)

	return nil
}

func (e *Engine) listActiveUsers(ctx context.Context) ([]string, error) {
	rows, err := e.db.QueryContext(ctx, `
		SELECT DISTINCT s.user_did FROM articles.subscriptions s
		UNION
		SELECT DISTINCT author_did FROM articles.likes
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var dids []string
	for rows.Next() {
		var did string
		if err := rows.Scan(&did); err != nil {
			return nil, err
		}
		dids = append(dids, did)
	}
	return dids, rows.Err()
}

func (e *Engine) getPrecomputed(ctx context.Context, userDID, recType string) (string, bool) {
	var data string
	err := e.db.QueryRowContext(ctx, `
		SELECT data FROM recs.precomputed_recommendations
		WHERE user_did = ? AND rec_type = ?
	`, userDID, recType).Scan(&data)
	if err != nil {
		return "", false
	}
	return data, true
}

func (e *Engine) storePrecomputed(ctx context.Context, userDID, recType string, data string) {
	if data == "null" {
		return
	}
	_, _ = e.db.ExecContext(ctx, `
		INSERT INTO recs.precomputed_recommendations (user_did, rec_type, data, computed_at)
		VALUES (?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(user_did, rec_type) DO UPDATE SET data = excluded.data, computed_at = excluded.computed_at
	`, userDID, recType, data)
}

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "null"
	}
	return string(b)
}
