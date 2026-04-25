package cluster

import (
	"context"
	"fmt"
)

const maxFollowDepth = 3

type followDistance struct {
	userA    string
	userB    string
	distance int
}

func (e *Engine) ComputeFollowDistancesData(ctx context.Context, sources []string) ([]followDistance, error) {
	if len(sources) == 0 {
		return nil, nil
	}

	rows, err := e.db.QueryContext(ctx, `SELECT user_did, target_did FROM main.follows WHERE user_did != target_did`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	adj := make(map[string][]string)
	for rows.Next() {
		var src, dst string
		if err := rows.Scan(&src, &dst); err != nil {
			return nil, err
		}
		adj[src] = append(adj[src], dst)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	var result []followDistance
	for _, src := range sources {
		dist := map[string]int{src: 0}
		queue := []string{src}
		for len(queue) > 0 {
			cur := queue[0]
			queue = queue[1:]
			d := dist[cur]
			if d >= maxFollowDepth {
				continue
			}
			for _, next := range adj[cur] {
				if _, ok := dist[next]; !ok {
					dist[next] = d + 1
					queue = append(queue, next)
				}
			}
		}
		for other, d := range dist {
			if d > 0 {
				result = append(result, followDistance{userA: src, userB: other, distance: d})
			}
		}
	}

	return result, nil
}

func (e *Engine) WriteFollowDistances(ctx context.Context, distances []followDistance) error {
	tx, err := e.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `DELETE FROM recs.follow_distances`); err != nil {
		return err
	}

	stmt, err := tx.PrepareContext(ctx, `INSERT INTO recs.follow_distances (user_a, user_b, distance) VALUES (?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, d := range distances {
		if _, err := stmt.ExecContext(ctx, d.userA, d.userB, d.distance); err != nil {
			return err
		}
	}

	e.logger.Info("follow distances computed", "pairs", len(distances))
	return tx.Commit()
}

// ComputeFollowDistances incrementally recomputes follow distances for users
// whose follows changed since the last run, as tracked by the follows_dirty column.
func (e *Engine) ComputeFollowDistances(ctx context.Context) error {
	rows, err := e.db.QueryContext(ctx, `SELECT did FROM main.users WHERE follows_dirty = 1`)
	if err != nil {
		return err
	}

	var dirtyUsers []string
	for rows.Next() {
		var did string
		if err := rows.Scan(&did); err != nil {
			rows.Close()
			return err
		}
		dirtyUsers = append(dirtyUsers, did)
	}
	rows.Close()

	if len(dirtyUsers) == 0 {
		return nil
	}

	distances, err := e.ComputeFollowDistancesData(ctx, dirtyUsers)
	if err != nil {
		return err
	}

	tx, err := e.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	ph := make([]string, len(dirtyUsers))
	args := make([]any, len(dirtyUsers))
	for i, did := range dirtyUsers {
		ph[i] = "?"
		args[i] = did
	}
	if _, err := tx.ExecContext(ctx,
		fmt.Sprintf("DELETE FROM recs.follow_distances WHERE user_a IN (%s)", joinPh(ph)),
		args...,
	); err != nil {
		return err
	}

	stmt, err := tx.PrepareContext(ctx, `INSERT INTO recs.follow_distances (user_a, user_b, distance) VALUES (?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, d := range distances {
		if _, err := stmt.ExecContext(ctx, d.userA, d.userB, d.distance); err != nil {
			return err
		}
	}

	if _, err := tx.ExecContext(ctx,
		fmt.Sprintf("UPDATE main.users SET follows_dirty = 0 WHERE did IN (%s)", joinPh(ph)),
		args...,
	); err != nil {
		return err
	}

	e.logger.Info("follow distances computed", "users", len(dirtyUsers), "pairs", len(distances))
	return tx.Commit()
}
