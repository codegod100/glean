package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	oauth "github.com/bluesky-social/indigo/atproto/auth/oauth"
	"github.com/bluesky-social/indigo/atproto/syntax"
	"gotest.tools/v3/assert"

	"pkg.rbrt.fr/glean/internal/db"
)

func setupTestServer(t *testing.T) (*Server, *db.Store) {
	t.Helper()
	f, err := os.CreateTemp("", "glean-test-*.db")
	assert.NilError(t, err)
	assert.NilError(t, f.Close())
	path := f.Name()
	t.Cleanup(func() {
		for _, suffix := range []string{"", "_users", "_users-shm", "_users-wal", "_articles", "_articles-shm", "_articles-wal", "_recs", "_recs-shm", "_recs-wal"} {
			_ = os.Remove(path + suffix)
		}
	})

	dbs, err := db.Open(path)
	assert.NilError(t, err)
	t.Cleanup(func() { _ = dbs.Close() })

	s := &Server{
		dbs:        dbs,
		oauthStore: db.NewOAuthStore(dbs),
		sessionKey: []byte("test-session-key-32-bytes-long!"),
	}
	return s, dbs
}

func encodeTestSession(t *testing.T, s *Server, data sessionData) string {
	t.Helper()
	encoded, err := encodeSession(s.sessionKey, data)
	assert.NilError(t, err)
	return encoded
}

func seedOAuthSession(t *testing.T, s *Server, did, sessionID string) {
	t.Helper()
	parsedDID, err := syntax.ParseDID(did)
	assert.NilError(t, err)
	sessData := oauth.ClientSessionData{
		AccountDID:   parsedDID,
		SessionID:    sessionID,
		HostURL:      "https://example.com",
		AccessToken:  "test-access-token",
		RefreshToken: "test-refresh-token",
	}
	err = s.oauthStore.SaveSession(context.Background(), sessData)
	assert.NilError(t, err)
}

func TestSessionMiddleware_ValidOAuthSession(t *testing.T) {
	s, dbs := setupTestServer(t)
	ctx := context.Background()

	_, err := dbs.Users.CreateUser(ctx, "did:test:user1")
	assert.NilError(t, err)

	seedOAuthSession(t, s, "did:test:user1", "session-1")

	cookieVal := encodeTestSession(t, s, sessionData{
		DID:       "did:test:user1",
		SessionID: "session-1",
	})

	called := false
	handler := s.sessionMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		u := currentUser(r)
		assert.Assert(t, u != nil)
		assert.Equal(t, u.DID, "did:test:user1")
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	req.AddCookie(&http.Cookie{Name: "glean_session", Value: cookieVal})
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	assert.Assert(t, called)
	assert.Equal(t, rec.Code, http.StatusOK)
}

func TestSessionMiddleware_InvalidOAuthSession_ClearsCookie(t *testing.T) {
	s, dbs := setupTestServer(t)
	ctx := context.Background()

	_, err := dbs.Users.CreateUser(ctx, "did:test:user2")
	assert.NilError(t, err)

	cookieVal := encodeTestSession(t, s, sessionData{
		DID:       "did:test:user2",
		SessionID: "session-gone",
	})

	called := false
	handler := s.sessionMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		u := currentUser(r)
		assert.Assert(t, u == nil)
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	req.AddCookie(&http.Cookie{Name: "glean_session", Value: cookieVal})
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	assert.Assert(t, called)
	assert.Equal(t, rec.Code, http.StatusOK)

	var cleared bool
	for _, c := range rec.Result().Cookies() {
		if c.Name == "glean_session" && c.MaxAge == -1 {
			cleared = true
		}
	}
	assert.Assert(t, cleared, "expected glean_session cookie to be cleared")
}

func TestSessionMiddleware_NonOAuthSession_PassesThrough(t *testing.T) {
	s, dbs := setupTestServer(t)
	ctx := context.Background()

	_, err := dbs.Users.CreateUser(ctx, "did:test:user3")
	assert.NilError(t, err)

	cookieVal := encodeTestSession(t, s, sessionData{
		DID: "did:test:user3",
	})

	called := false
	handler := s.sessionMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		u := currentUser(r)
		assert.Assert(t, u != nil)
		assert.Equal(t, u.DID, "did:test:user3")
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	req.AddCookie(&http.Cookie{Name: "glean_session", Value: cookieVal})
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	assert.Assert(t, called)
	assert.Equal(t, rec.Code, http.StatusOK)
}

func TestSessionMiddleware_OAuthSessionDeletedAfterLogin_ClearsCookie(t *testing.T) {
	s, dbs := setupTestServer(t)
	ctx := context.Background()

	_, err := dbs.Users.CreateUser(ctx, "did:test:user4")
	assert.NilError(t, err)

	seedOAuthSession(t, s, "did:test:user4", "session-4")

	cookieVal := encodeTestSession(t, s, sessionData{
		DID:       "did:test:user4",
		SessionID: "session-4",
	})

	handler := s.sessionMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := currentUser(r)
		if u == nil {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	req.AddCookie(&http.Cookie{Name: "glean_session", Value: cookieVal})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, rec.Code, http.StatusOK)

	parsedDID, err := syntax.ParseDID("did:test:user4")
	assert.NilError(t, err)
	err = s.oauthStore.DeleteSession(ctx, parsedDID, "session-4")
	assert.NilError(t, err)

	req2 := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	req2.AddCookie(&http.Cookie{Name: "glean_session", Value: cookieVal})
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)
	assert.Equal(t, rec2.Code, http.StatusUnauthorized)
}

func TestIsOAuthSessionValid(t *testing.T) {
	s, _ := setupTestServer(t)
	ctx := context.Background()

	seedOAuthSession(t, s, "did:test:valid", "sess-1")

	assert.Assert(t, s.isOAuthSessionValid(ctx, &sessionData{
		DID:       "did:test:valid",
		SessionID: "sess-1",
	}))

	assert.Assert(t, !s.isOAuthSessionValid(ctx, &sessionData{
		DID:       "did:test:valid",
		SessionID: "sess-nonexistent",
	}))

	assert.Assert(t, !s.isOAuthSessionValid(ctx, &sessionData{
		DID:       "did:test:nonexistent",
		SessionID: "sess-1",
	}))
}
