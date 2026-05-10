package cluster

import (
	"context"
	"database/sql"
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

func (e *Engine) writeFollowDistancesForUser(ctx context.Context, stmt *sql.Stmt, src string) (int, error) {
	reachable, err := e.bfsReachable(ctx, src)
	if err != nil {
		return 0, err
	}

	written := 0
	for dst, d := range reachable {
		if d > 0 {
			if _, err := stmt.ExecContext(ctx, src, dst, d); err != nil {
				return written, err
			}
			written++
		}
	}
	return written, nil
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

	var totalPairs int
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO recs.follow_distances (user_a, user_b, distance) VALUES (?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, did := range dirtyUsers {
		n, err := e.writeFollowDistancesForUser(ctx, stmt, did)
		if err != nil {
			return err
		}
		totalPairs += n
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

	e.logger.Info("follow distances computed", "users", len(dirtyUsers), "pairs", totalPairs)
	return tx.Commit()
}
