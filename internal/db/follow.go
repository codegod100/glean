package db

import (
	"context"
	"database/sql"
	"time"
)

type Follow struct {
	UserDID    string
	TargetDID  string
	URI        sql.NullString
	CID        sql.NullString
	FollowedAt sql.NullTime
}

func (db *DB) UpsertFollow(ctx context.Context, userDID, targetDID, uri, cid string) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO follows (user_did, target_did, uri, cid, followed_at)
		VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(user_did, target_did) DO UPDATE SET
			uri = excluded.uri,
			cid = excluded.cid
	`, userDID, targetDID, uriOrNil(uri), uriOrNil(cid))
	return err
}

func (db *DB) DeleteFollow(ctx context.Context, userDID, targetDID string) error {
	_, err := db.ExecContext(ctx, `DELETE FROM follows WHERE user_did = ? AND target_did = ?`, userDID, targetDID)
	return err
}

func (db *DB) DeleteFollowByURI(ctx context.Context, uri string) error {
	_, err := db.ExecContext(ctx, `DELETE FROM follows WHERE uri = ?`, uri)
	return err
}

func (db *DB) ListFollows(ctx context.Context, userDID string, limit, offset int) ([]*Follow, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT user_did, target_did, uri, cid, followed_at
		FROM follows WHERE user_did = ?
		ORDER BY followed_at DESC
		LIMIT ? OFFSET ?
	`, userDID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var follows []*Follow
	for rows.Next() {
		f := &Follow{}
		if err := rows.Scan(&f.UserDID, &f.TargetDID, &f.URI, &f.CID, &f.FollowedAt); err != nil {
			return nil, err
		}
		follows = append(follows, f)
	}
	return follows, rows.Err()
}

func (db *DB) ListFollowers(ctx context.Context, targetDID string, limit, offset int) ([]*Follow, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT user_did, target_did, uri, cid, followed_at
		FROM follows WHERE target_did = ?
		ORDER BY followed_at DESC
		LIMIT ? OFFSET ?
	`, targetDID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var follows []*Follow
	for rows.Next() {
		f := &Follow{}
		if err := rows.Scan(&f.UserDID, &f.TargetDID, &f.URI, &f.CID, &f.FollowedAt); err != nil {
			return nil, err
		}
		follows = append(follows, f)
	}
	return follows, rows.Err()
}

func (db *DB) IsFollowing(ctx context.Context, userDID, targetDID string) (bool, error) {
	var exists int
	err := db.QueryRowContext(ctx, `
		SELECT 1 FROM follows WHERE user_did = ? AND target_did = ?
	`, userDID, targetDID).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (db *DB) GetFollowDIDs(ctx context.Context, userDID string) ([]string, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT target_did FROM follows WHERE user_did = ?
	`, userDID)
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

func (db *DB) SyncFollows(ctx context.Context, userDID string, activeFollows map[string]Follow) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	rows, err := tx.QueryContext(ctx, `SELECT target_did, uri, cid, followed_at FROM follows WHERE user_did = ?`, userDID)
	if err != nil {
		return err
	}

	existing := make(map[string]bool)
	for rows.Next() {
		var targetDID string
		var uri, cid sql.NullString
		var followedAt sql.NullTime
		if err := rows.Scan(&targetDID, &uri, &cid, &followedAt); err != nil {
			rows.Close()
			return err
		}
		existing[targetDID] = true
	}
	rows.Close()

	for targetDID := range existing {
		if _, ok := activeFollows[targetDID]; !ok {
			if _, err := tx.ExecContext(ctx, `DELETE FROM follows WHERE user_did = ? AND target_did = ?`, userDID, targetDID); err != nil {
				return err
			}
		}
	}

	for targetDID, f := range activeFollows {
		if !existing[targetDID] {
			var followedAt any
			if f.FollowedAt.Valid {
				followedAt = f.FollowedAt.Time
			} else {
				followedAt = time.Now()
			}
			_, err := tx.ExecContext(ctx, `
				INSERT INTO follows (user_did, target_did, uri, cid, followed_at)
				VALUES (?, ?, ?, ?, ?)
				ON CONFLICT(user_did, target_did) DO UPDATE SET uri = excluded.uri, cid = excluded.cid
			`, userDID, targetDID, f.URI, f.CID, followedAt)
			if err != nil {
				return err
			}
		}
	}

	return tx.Commit()
}
