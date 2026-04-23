package server

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"

	"pkg.rbrt.fr/glean/internal/atproto"
	"pkg.rbrt.fr/glean/internal/db"
)

type ctxKey struct{}

func contextWithUser(ctx context.Context, user *db.User) context.Context {
	return context.WithValue(ctx, ctxKey{}, user)
}

func currentUser(r *http.Request) *db.User {
	user, _ := r.Context().Value(ctxKey{}).(*db.User)
	return user
}

func (s *Server) getUserFromSession(r *http.Request) *db.User {
	cookie, err := r.Cookie("glean_session")
	if err != nil {
		return nil
	}

	data, err := decodeSession(s.sessionKey, cookie.Value)
	if err != nil {
		return nil
	}

	user, err := s.dbs.Users.GetUser(r.Context(), data.DID)
	if err != nil {
		return nil
	}

	p := atproto.ResolveProfile(r.Context(), user.DID)
	user.Handle = p.Handle
	user.DisplayName = p.DisplayName
	user.AvatarURL = p.AvatarURL
	return user
}

func (s *Server) setUserSession(w http.ResponseWriter, user *db.User) {
	data := sessionData{DID: user.DID}
	encoded, err := encodeSession(s.sessionKey, data)
	if err != nil {
		s.logger.Error("failed to encode session", "error", err)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "glean_session",
		Value:    encoded,
		Path:     "/",
		MaxAge:   86400 * 30,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func (s *Server) clearUserSession(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     "glean_session",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

type sessionData struct {
	DID          string `json:"did"`
	PDSURL       string `json:"pds_url,omitempty"`
	AccessToken  string `json:"access_token,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
	SessionID    string `json:"session_id,omitempty"`
}

func (s *Server) getSessionData(r *http.Request) *sessionData {
	cookie, err := r.Cookie("glean_session")
	if err != nil {
		return nil
	}
	data, err := decodeSession(s.sessionKey, cookie.Value)
	if err != nil {
		return nil
	}
	return data
}

func encodeSession(key []byte, data sessionData) (string, error) {
	payload, err := json.Marshal(data)
	if err != nil {
		return "", err
	}

	mac := hmac.New(sha256.New, key)
	mac.Write(payload)
	sig := mac.Sum(nil)

	raw := append(payload, sig...)
	return base64.URLEncoding.EncodeToString(raw), nil
}

func decodeSession(key []byte, encoded string) (*sessionData, error) {
	raw, err := base64.URLEncoding.DecodeString(encoded)
	if err != nil {
		return nil, errInvalidSession
	}

	if len(raw) < sha256.Size {
		return nil, errInvalidSession
	}

	payload := raw[:len(raw)-sha256.Size]
	sig := raw[len(raw)-sha256.Size:]

	mac := hmac.New(sha256.New, key)
	mac.Write(payload)
	expectedSig := mac.Sum(nil)

	if !hmac.Equal(sig, expectedSig) {
		return nil, errInvalidSession
	}

	var data sessionData
	if err := json.Unmarshal(payload, &data); err != nil {
		return nil, errInvalidSession
	}

	return &data, nil
}

var errInvalidSession = &invalidSessionError{}

type invalidSessionError struct{}

func (e *invalidSessionError) Error() string { return "invalid session" }
