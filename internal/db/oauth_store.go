package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	oauth "github.com/bluesky-social/indigo/atproto/auth/oauth"
	"github.com/bluesky-social/indigo/atproto/syntax"
)

type OAuthStore struct {
	db *DB
}

func NewOAuthStore(db *DB) *OAuthStore {
	return &OAuthStore{db: db}
}

func (s *OAuthStore) Init(ctx context.Context) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS oauth_auth_requests (
			state TEXT PRIMARY KEY,
			data TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS oauth_sessions (
			account_did TEXT NOT NULL,
			session_id TEXT NOT NULL,
			data TEXT NOT NULL,
			PRIMARY KEY (account_did, session_id)
		)`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}

func (s *OAuthStore) GetSession(ctx context.Context, did syntax.DID, sessionID string) (*oauth.ClientSessionData, error) {
	var data []byte
	err := s.db.QueryRowContext(ctx, `
		SELECT data FROM oauth_sessions WHERE account_did = ? AND session_id = ?
	`, did.String(), sessionID).Scan(&data)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("session not found: %s/%s", did, sessionID)
	}
	if err != nil {
		return nil, err
	}
	var sess oauth.ClientSessionData
	if err := json.Unmarshal(data, &sess); err != nil {
		return nil, err
	}
	return &sess, nil
}

func (s *OAuthStore) SaveSession(ctx context.Context, sess oauth.ClientSessionData) error {
	data, err := json.Marshal(sess)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO oauth_sessions (account_did, session_id, data)
		VALUES (?, ?, ?)
		ON CONFLICT(account_did, session_id) DO UPDATE SET data = excluded.data
	`, sess.AccountDID.String(), sess.SessionID, data)
	return err
}

func (s *OAuthStore) DeleteSession(ctx context.Context, did syntax.DID, sessionID string) error {
	_, err := s.db.ExecContext(ctx, `
		DELETE FROM oauth_sessions WHERE account_did = ? AND session_id = ?
	`, did.String(), sessionID)
	return err
}

func (s *OAuthStore) GetAuthRequestInfo(ctx context.Context, state string) (*oauth.AuthRequestData, error) {
	var data []byte
	err := s.db.QueryRowContext(ctx, `
		SELECT data FROM oauth_auth_requests WHERE state = ?
	`, state).Scan(&data)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("auth request not found: %s", state)
	}
	if err != nil {
		return nil, err
	}
	var info oauth.AuthRequestData
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, err
	}
	return &info, nil
}

func (s *OAuthStore) SaveAuthRequestInfo(ctx context.Context, info oauth.AuthRequestData) error {
	data, err := json.Marshal(info)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO oauth_auth_requests (state, data) VALUES (?, ?)
	`, info.State, data)
	return err
}

func (s *OAuthStore) DeleteAuthRequestInfo(ctx context.Context, state string) error {
	_, err := s.db.ExecContext(ctx, `
		DELETE FROM oauth_auth_requests WHERE state = ?
	`, state)
	return err
}
