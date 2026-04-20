package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"

	"github.com/bluesky-social/indigo/atproto/syntax"

	oauth "github.com/bluesky-social/indigo/atproto/auth/oauth"

	"pkg.rbrt.fr/glean/internal/atproto"
)

func (s *Server) handleAuthLogin(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "login.html", map[string]any{})
}

func (s *Server) handleAuthStart(w http.ResponseWriter, r *http.Request) {
	handle := r.FormValue("handle")
	if handle == "" {
		http.Error(w, "handle required", http.StatusBadRequest)
		return
	}

	authURL, err := s.oauth.StartAuthFlow(r.Context(), handle)
	if err != nil {
		s.logger.Error("failed to start OAuth flow", "error", err)

		did, resolveErr := atproto.ResolveHandle(r.Context(), handle)
		if resolveErr != nil {
			http.Error(w, "could not resolve handle", http.StatusInternalServerError)
			return
		}
		user, createErr := s.db.CreateUser(r.Context(), did, handle, "", "")
		if createErr != nil {
			http.Error(w, createErr.Error(), http.StatusInternalServerError)
			return
		}
		s.setUserSession(w, user)
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, authURL, http.StatusSeeOther)
}

func (s *Server) handleAuthCallback(w http.ResponseWriter, r *http.Request) {
	params := r.URL.Query()

	if params.Get("code") != "" && params.Get("state") != "" {
		s.handleOAuthCallback(w, r)
		return
	}

	handle := params.Get("handle")
	if handle == "" {
		http.Error(w, "handle required", http.StatusBadRequest)
		return
	}

	did, err := atproto.ResolveHandle(r.Context(), handle)
	if err != nil {
		s.logger.Error("failed to resolve handle", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	user, err := s.db.CreateUser(r.Context(), did, handle, "", "")
	if err != nil {
		s.logger.Error("failed to create user", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.setUserSession(w, user)
	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

func (s *Server) handleOAuthCallback(w http.ResponseWriter, r *http.Request) {
	sessData, err := s.oauth.ProcessCallback(r.Context(), r.URL.Query())
	if err != nil {
		s.logger.Error("OAuth callback failed", "error", err)
		http.Error(w, "authentication failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	did := sessData.AccountDID.String()
	handle := did
	if ident, err := s.oauth.Dir.LookupDID(r.Context(), sessData.AccountDID); err == nil {
		handle = ident.Handle.String()
	}

	displayName, avatarURL := s.fetchUserProfile(r.Context(), sessData)

	user, err := s.db.CreateUser(r.Context(), did, handle, displayName, avatarURL)
	if err != nil {
		s.logger.Error("failed to create user", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	sessionData := sessionData{
		DID:       user.DID,
		PDSURL:    sessData.HostURL,
		SessionID: sessData.SessionID,
	}
	encoded, err := encodeSession(sessionData)
	if err != nil {
		s.logger.Error("failed to encode session", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
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

	s.syncUserInBackground(user.DID, s.pdsClientFromSession(sessData))

	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

func (s *Server) fetchUserProfile(ctx context.Context, sessData *oauth.ClientSessionData) (string, string) {
	did := sessData.AccountDID.String()

	session, err := s.oauth.ResumeSession(ctx, sessData.AccountDID, sessData.SessionID)
	if err != nil {
		s.logger.Warn("failed to resume session for profile fetch", "error", err)
		return "", ""
	}

	var profile struct {
		DisplayName string `json:"displayName"`
		Avatar      string `json:"avatar"`
	}

	nsid, _ := syntax.ParseNSID("app.bsky.actor.getProfile")
	if err := session.APIClient().Get(ctx, nsid, map[string]any{"actor": did}, &profile); err != nil {
		s.logger.Warn("failed to fetch profile from PDS", "error", err, "did", did)
		return "", ""
	}
	return profile.DisplayName, profile.Avatar
}

func (s *Server) pdsClientFromSession(sessData *oauth.ClientSessionData) *atproto.Client {
	session, err := s.oauth.ResumeSession(context.Background(), sessData.AccountDID, sessData.SessionID)
	if err != nil {
		s.logger.Warn("failed to resume session for sync", "error", err)
		return nil
	}
	return &atproto.Client{APIClient: session.APIClient()}
}

func (s *Server) handleOAuthClientMetadata(w http.ResponseWriter, r *http.Request) {
	if s.clientID == "" {
		http.Error(w, "localhost client", http.StatusNotFound)
		return
	}

	meta := s.oauth.Config.ClientMetadata()
	name := "Glean"
	meta.ClientName = &name
	uri := s.clientID
	meta.ClientURI = &uri

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(meta)
}

func (s *Server) handleAuthLogout(w http.ResponseWriter, r *http.Request) {
	session := s.getSessionData(r)
	if session != nil && session.SessionID != "" {
		did, err := syntax.ParseDID(session.DID)
		if err == nil {
			_ = s.oauth.Logout(r.Context(), did, session.SessionID)
		}
	}
	s.clearUserSession(w)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func resolveCallbackURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s/auth/callback", scheme, r.Host)
}

func resolveClientID(r *http.Request) string {
	cid := os.Getenv("GLEAN_OAUTH_CLIENT_ID")
	if cid != "" {
		return cid
	}
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s/oauth/client-metadata", scheme, r.Host)
}

func resolveBaseURL(r *http.Request) *url.URL {
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return &url.URL{Scheme: scheme, Host: r.Host}
}
