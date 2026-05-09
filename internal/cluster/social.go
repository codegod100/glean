package cluster

import (
	"context"
	"fmt"
)

const maxFollowDepth = 3
const maxReachablePerUser = 10000

func chunk[T any](s []T, size int) [][]T {
	var chunks [][]T
	for i := 0; i < len(s); i += size {
		end := min(i+size, len(s))
		chunks = append(chunks, s[i:end])
	}
	return chunks
}

type followDistance struct {
	userA    string
	userB    string
	distance int
}

func (e *Engine) ComputeFollowDistancesData(ctx context.Context, sources []string) ([]followDistance, error) {
	if len(sources) == 0 {
		return nil, nil
	}

	type pair struct {
		src, dst string
	}
	distances := make(map[pair]int)

	for _, src := range sources {
		reachable, err := e.bfsReachable(ctx, src)
		if err != nil {
			return nil, err
		}
		for other, d := range reachable {
			if d > 0 {
				distances[pair{src, other}] = d
			}
		}
	}

	var result []followDistance
	for k, d := range distances {
		result = append(result, followDistance{userA: k.src, userB: k.dst, distance: d})
	}
	return result, nil
}

func (e *Engine) bfsReachable(ctx context.Context, src string) (map[string]int, error) {
	reachable := map[string]int{src: 0}
	frontier := []string{src}

	for depth := 0; depth < maxFollowDepth && len(frontier) > 0; depth++ {
		var nextLevel []string
		for _, batch := range chunk(frontier, 500) {
			ph := make([]string, len(batch))
			args := make([]any, len(batch))
			for i, did := range batch {
				ph[i] = "?"
				args[i] = did
			}

			rows, err := e.db.QueryContext(ctx,
				fmt.Sprintf(`SELECT target_did FROM main.follows WHERE user_did IN (%s) AND user_did != target_did`, joinPh(ph)),
				args...,
			)
			if err != nil {
				return reachable, err
			}

			for rows.Next() {
				var dst string
				if err := rows.Scan(&dst); err != nil {
					rows.Close()
					return reachable, err
				}
				if _, ok := reachable[dst]; !ok {
					if len(reachable) >= maxReachablePerUser {
						rows.Close()
						return reachable, nil
					}
					reachable[dst] = depth + 1
					nextLevel = append(nextLevel, dst)
				}
			}
			rows.Close()
		}
		frontier = nextLevel
	}
	return reachable, nil
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

	const sqliteMaxVars = 500
	for _, chunk := range chunk(dirtyUsers, sqliteMaxVars) {
		ph := make([]string, len(chunk))
		args := make([]any, len(chunk))
		for i, did := range chunk {
			ph[i] = "?"
			args[i] = did
		}
		if _, err := tx.ExecContext(ctx,
			fmt.Sprintf("DELETE FROM recs.follow_distances WHERE user_a IN (%s)", joinPh(ph)),
			args...,
		); err != nil {
			return err
		}
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

	for _, chunk := range chunk(dirtyUsers, sqliteMaxVars) {
		ph := make([]string, len(chunk))
		args := make([]any, len(chunk))
		for i, did := range chunk {
			ph[i] = "?"
			args[i] = did
		}
		if _, err := tx.ExecContext(ctx,
			fmt.Sprintf("UPDATE main.users SET follows_dirty = 0 WHERE did IN (%s)", joinPh(ph)),
			args...,
		); err != nil {
			return err
		}
	}

	e.logger.Info("follow distances computed", "users", len(dirtyUsers), "pairs", len(distances))
	return tx.Commit()
}
