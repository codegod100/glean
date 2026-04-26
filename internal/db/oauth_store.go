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

func NewOAuthStore(dbs *Store) *OAuthStore {
	return &OAuthStore{db: dbs.db}
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

func (s *OAuthStore) ListSessionsForDID(ctx context.Context, did string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT session_id FROM oauth_sessions WHERE account_did = ? ORDER BY ROWID DESC
	`, did)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *OAuthStore) CountActiveUsers(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(DISTINCT account_did) FROM oauth_sessions`).Scan(&count)
	return count, err
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
