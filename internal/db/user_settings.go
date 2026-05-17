package db

import (
	"context"
	"database/sql"
	"strings"
)

type UserSettings struct {
	DID           string
	Languages     []string
	ExpandedView  bool
	DigestEnabled bool
}

func (s *UserStore) GetSettings(ctx context.Context, did string) (*UserSettings, error) {
	var langs sql.NullString
	var expandedView bool
	var digestEnabled bool
	err := s.db.QueryRowContext(ctx, `SELECT languages, COALESCE(expanded_view, 0), COALESCE(digest_enabled, 1) FROM user_settings WHERE did = ?`, did).Scan(&langs, &expandedView, &digestEnabled)
	if err != nil {
		if err == sql.ErrNoRows {
			return &UserSettings{DID: did, DigestEnabled: false}, nil
		}
		return nil, err
	}
	us := &UserSettings{DID: did, ExpandedView: expandedView, DigestEnabled: digestEnabled}
	if langs.Valid && langs.String != "" {
		us.Languages = strings.Split(langs.String, ",")
	}
	return us, nil
}

func (s *UserStore) UpdateLanguages(ctx context.Context, did string, languages []string) error {
	langStr := strings.Join(languages, ",")
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO user_settings (did, languages, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(did) DO UPDATE SET languages = excluded.languages, updated_at = CURRENT_TIMESTAMP
	`, did, langStr)
	return err
}

func (s *UserStore) GetLanguages(ctx context.Context, did string) ([]string, error) {
	us, err := s.GetSettings(ctx, did)
	if err != nil {
		return nil, err
	}
	if len(us.Languages) == 0 {
		return nil, nil
	}
	return us.Languages, nil
}

func (s *UserStore) SetExpandedView(ctx context.Context, did string, enabled bool) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO user_settings (did, expanded_view, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(did) DO UPDATE SET expanded_view = excluded.expanded_view, updated_at = CURRENT_TIMESTAMP
	`, did, enabled)
	return err
}

func (s *UserStore) SetDigestEnabled(ctx context.Context, did string, enabled bool) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO user_settings (did, digest_enabled, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(did) DO UPDATE SET digest_enabled = excluded.digest_enabled, updated_at = CURRENT_TIMESTAMP
	`, did, enabled)
	return err
}
