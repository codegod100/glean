package db

import (
	"context"
	"database/sql"
)

type User struct {
	DID         string
	Handle      string
	DisplayName sql.NullString
	AvatarURL   sql.NullString
	IndexedAt   sql.NullTime
	UpdatedAt   sql.NullTime
}

type UserData struct {
	DID         string
	Handle      string
	DisplayName string
	AvatarURL   string
}

func (db *DB) BatchCreateUsers(ctx context.Context, users []UserData) error {
	if len(users) == 0 {
		return nil
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO users (did, handle, display_name, avatar_url, updated_at)
		VALUES (?, COALESCE(NULLIF(?, ''), ?), NULLIF(?, ''), NULLIF(?, ''), CURRENT_TIMESTAMP)
		ON CONFLICT(did) DO UPDATE SET
			handle = COALESCE(NULLIF(excluded.handle, ''), users.handle),
			display_name = COALESCE(NULLIF(excluded.display_name, ''), users.display_name),
			avatar_url = COALESCE(NULLIF(excluded.avatar_url, ''), users.avatar_url),
			updated_at = CURRENT_TIMESTAMP
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, u := range users {
		if _, err := stmt.ExecContext(ctx, u.DID, u.Handle, u.DID, u.DisplayName, u.AvatarURL); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (db *DB) CreateUser(ctx context.Context, did, handle, displayName, avatarURL string) (*User, error) {
	err := db.BatchCreateUsers(ctx, []UserData{{DID: did, Handle: handle, DisplayName: displayName, AvatarURL: avatarURL}})
	if err != nil {
		return nil, err
	}
	return db.GetUser(ctx, did)
}

func (db *DB) GetUser(ctx context.Context, did string) (*User, error) {
	u := &User{}
	err := db.QueryRowContext(ctx, `
		SELECT did, handle, display_name, avatar_url, indexed_at, updated_at
		FROM users WHERE did = ?
	`, did).Scan(&u.DID, &u.Handle, &u.DisplayName, &u.AvatarURL, &u.IndexedAt, &u.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return u, nil
}

func (db *DB) GetUserByHandle(ctx context.Context, handle string) (*User, error) {
	u := &User{}
	err := db.QueryRowContext(ctx, `
		SELECT did, handle, display_name, avatar_url, indexed_at, updated_at
		FROM users WHERE handle = ?
	`, handle).Scan(&u.DID, &u.Handle, &u.DisplayName, &u.AvatarURL, &u.IndexedAt, &u.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return u, nil
}

func (db *DB) ListUserDIDs(ctx context.Context) (map[string]bool, error) {
	rows, err := db.QueryContext(ctx, `SELECT did FROM users`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	dids := make(map[string]bool)
	for rows.Next() {
		var did string
		if err := rows.Scan(&did); err != nil {
			return nil, err
		}
		dids[did] = true
	}
	return dids, rows.Err()
}

func (db *DB) UpdateUserProfile(ctx context.Context, did, displayName, avatarURL string) error {
	_, err := db.ExecContext(ctx, `
		UPDATE users SET
			display_name = COALESCE(NULLIF(?, ''), display_name),
			avatar_url = COALESCE(NULLIF(?, ''), avatar_url),
			updated_at = CURRENT_TIMESTAMP
		WHERE did = ?
	`, displayName, avatarURL, did)
	return err
}

func (db *DB) ListUsers(ctx context.Context) ([]*User, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT did, handle, display_name, avatar_url, indexed_at, updated_at
		FROM users ORDER BY updated_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []*User
	for rows.Next() {
		u := &User{}
		if err := rows.Scan(&u.DID, &u.Handle, &u.DisplayName, &u.AvatarURL, &u.IndexedAt, &u.UpdatedAt); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}
