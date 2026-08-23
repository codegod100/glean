package db

import (
	"context"
	"database/sql"
)

type User struct {
	DID          string
	Handle       string
	DisplayName  string
	AvatarURL    string
	IndexedAt    sql.NullTime
	UpdatedAt    sql.NullTime
	FollowsDirty bool
}

type UserStore struct {
	db *DB
}

func NewUserStore(db *DB) *UserStore {
	return &UserStore{db: db}
}

func (s *UserStore) BatchCreateUsers(ctx context.Context, dids []string) error {
	if len(dids) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT OR IGNORE INTO users (did, updated_at)
		VALUES (?, CURRENT_TIMESTAMP)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, did := range dids {
		if _, err := stmt.ExecContext(ctx, did); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *UserStore) CreateUser(ctx context.Context, did string) (*User, error) {
	err := s.BatchCreateUsers(ctx, []string{did})
	if err != nil {
		return nil, err
	}
	return s.GetUser(ctx, did)
}

func (s *UserStore) GetUser(ctx context.Context, did string) (*User, error) {
	u := &User{}
	err := s.db.QueryRowContext(ctx, `
		SELECT did, indexed_at, updated_at, follows_dirty FROM users WHERE did = ?
	`, did).Scan(&u.DID, &u.IndexedAt, &u.UpdatedAt, &u.FollowsDirty)
	if err != nil {
		return nil, err
	}
	return u, nil
}

func (s *UserStore) UserDIDs(ctx context.Context) (map[string]bool, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT did FROM users`)
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

// UserDIDList returns the DIDs of all known users.
func (s *UserStore) UserDIDList(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT did FROM users`)
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

func (s *UserStore) ListUsers(ctx context.Context) ([]*User, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT did, indexed_at, updated_at, follows_dirty
		FROM users ORDER BY updated_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []*User
	for rows.Next() {
		u := &User{}
		if err := rows.Scan(&u.DID, &u.IndexedAt, &u.UpdatedAt, &u.FollowsDirty); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}
