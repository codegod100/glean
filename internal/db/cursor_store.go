package db

import (
	"context"
	"database/sql"
)

type DBCursorStore struct {
	db *DB
}

func NewCursorStore(db *DB) *DBCursorStore {
	return &DBCursorStore{db: db}
}

func (s *DBCursorStore) LoadCursor(ctx context.Context) (*int64, error) {
	var cursor int64
	err := s.db.QueryRowContext(ctx, "SELECT cursor_us FROM jetstream_cursor WHERE id = 1").Scan(&cursor)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &cursor, nil
}

func (s *DBCursorStore) SaveCursor(ctx context.Context, cursor int64) error {
	_, err := s.db.ExecContext(ctx,
		"INSERT INTO jetstream_cursor (id, cursor_us) VALUES (1, ?) ON CONFLICT(id) DO UPDATE SET cursor_us = excluded.cursor_us",
		cursor,
	)
	return err
}
