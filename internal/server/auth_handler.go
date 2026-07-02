package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/bluesky-social/indigo/atproto/syntax"

	oauth "github.com/bluesky-social/indigo/atproto/auth/oauth"

	"pkg.rbrt.fr/glean/internal/atproto"
)

func (s *Server) handleAuthLoginMeta(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, oauthEnabledResponse{OAuthEnabled: s.clientID != ""})
}

// handleAuthRegister kicks off registration via the Eurosky PDS.
func (s *Server) handleAuthRegister(w http.ResponseWriter, r *http.Request) {
	authURL, err := s.oauth.StartAuthFlow(r.Context(), "https://eurosky.social")
	if err != nil {
		s.logger.Error("failed to start register OAuth flow", "error", err)
		writeAPIError(w, http.StatusInternalServerError, "Could not connect to Eurosky.")
		return
	}
	writeJSON(w, http.StatusOK, redirectResponse{Redirect: authURL})
}

func (s *Server) handleAuthResolve(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimPrefix(r.URL.Query().Get("q"), "@")
	if q == "" {
		writeJSON(w, http.StatusOK, actorsResponse{})
		return
	}

	actors, err := atproto.SearchActorsTypeahead(r.Context(), q, 5)
	if err != nil {
		s.logger.Warn("actor typeahead failed", "error", err)
		writeJSON(w, http.StatusOK, actorsResponse{})
		return
	}

	writeJSON(w, http.StatusOK, actorsResponse{Actors: actors})
}

func (s *Server) handleAuthStart(w http.ResponseWriter, r *http.Request) {
	handle := strings.TrimPrefix(r.FormValue("handle"), "@")
	if handle == "" {
		writeAPIError(w, http.StatusBadRequest, "Please enter your handle.")
		return
	}

	authURL, err := s.oauth.StartAuthFlow(r.Context(), handle)
	if err != nil {
		s.logger.Error("failed to start OAuth flow", "error", err)

		did, resolveErr := atproto.ResolveHandle(r.Context(), handle)
		if resolveErr != nil {
			writeAPIError(w, http.StatusBadRequest, "Could not resolve that handle. Please check and try again.")
			return
		}
		user, createErr := s.dbs.Users.CreateUser(r.Context(), did)
		if createErr != nil {
			writeAPIError(w, http.StatusInternalServerError, "Could not create your account. Please try again.")
			return
		}
		s.setUserSession(w, user)
		writeJSON(w, http.StatusOK, redirectResponse{Redirect: "/dashboard"})
		return
	}

	writeJSON(w, http.StatusOK, redirectResponse{Redirect: authURL})
}

// handleAuthCallback is hit by the OAuth provider after authorization. It is a
// browser navigation (not an XHR), so it issues HTTP redirects rather than JSON.
func (s *Server) handleAuthCallback(w http.ResponseWriter, r *http.Request) {
	params := r.URL.Query()

	if params.Get("code") != "" && params.Get("state") != "" {
		s.handleOAuthCallback(w, r)
		return
	}

	handle := params.Get("handle")
	if handle == "" {
		http.Redirect(w, r, "/auth/login?error=missing_handle", http.StatusSeeOther)
		return
	}

	did, err := atproto.ResolveHandle(r.Context(), handle)
	if err != nil {
		s.logger.Error("failed to resolve handle", "error", err)
		http.Redirect(w, r, "/auth/login?error=handle_not_found", http.StatusSeeOther)
		return
	}

	user, err := s.dbs.Users.CreateUser(r.Context(), did)
	if err != nil {
		s.logger.Error("failed to create user", "error", err)
		http.Redirect(w, r, "/auth/login?error=create_failed", http.StatusSeeOther)
		return
	}

	s.setUserSession(w, user)
	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

func (s *Server) handleOAuthCallback(w http.ResponseWriter, r *http.Request) {
	sessData, err := s.oauth.ProcessCallback(r.Context(), r.URL.Query())
	if err != nil {
		s.logger.Error("OAuth callback failed", "error", err)
		http.Redirect(w, r, "/auth/login?error=auth_failed", http.StatusSeeOther)
		return
	}

	did := sessData.AccountDID.String()

	client := s.pdsClientFromSession(sessData)

	user, err := s.dbs.Users.CreateUser(r.Context(), did)
	if err != nil {
		s.logger.Error("failed to create user", "error", err)
		http.Redirect(w, r, "/auth/login?error=create_failed", http.StatusSeeOther)
		return
	}

	sessionData := sessionData{
		DID:       user.DID,
		PDSURL:    sessData.HostURL,
		SessionID: sessData.SessionID,
	}
	encoded, err := encodeSession(s.sessionKey, sessionData)
	if err != nil {
		s.logger.Error("failed to encode session", "error", err)
		http.Redirect(w, r, "/auth/login?error=session_error", http.StatusSeeOther)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "glean_session",
		Value:    encoded,
		Path:     "/",
		MaxAge:   86400 * 30,
		HttpOnly: true,
		Secure:   s.secureCookies,
		SameSite: http.SameSiteLaxMode,
	})

	s.syncUserInBackground(user.DID, client)

	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

func (s *Server) pdsClientFromSession(sessData *oauth.ClientSessionData) *atproto.Client {
	session, err := s.oauth.ResumeSession(context.Background(), sessData.AccountDID, sessData.SessionID)
	if err != nil {
		s.logger.Warn("failed to resume session for sync", "error", err)
		return nil
	}
	return atproto.NewClient(session.APIClient())
}

func (s *Server) handleOAuthClientMetadata(w http.ResponseWriter, r *http.Request) {
	if s.clientID == "" {
		writeAPIError(w, http.StatusNotFound, "localhost client")
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
	writeJSON(w, http.StatusOK, redirectResponse{Redirect: "/"})
}
