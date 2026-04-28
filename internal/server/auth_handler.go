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

func (s *Server) handleAuthLogin(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "login.html", map[string]any{})
}

func (s *Server) handleAuthResolve(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimPrefix(r.URL.Query().Get("q"), "@")
	if q == "" {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"actors":[]}`))
		return
	}

	actors, err := atproto.SearchActorsTypeahead(r.Context(), q, 5)
	if err != nil {
		s.logger.Warn("actor typeahead failed", "error", err)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"actors":[]}`))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"actors": actors})
}

func (s *Server) handleAuthStart(w http.ResponseWriter, r *http.Request) {
	handle := strings.TrimPrefix(r.FormValue("handle"), "@")
	if handle == "" {
		s.renderError(w, r, http.StatusBadRequest, "Missing handle", "Please enter your handle.")
		return
	}

	authURL, err := s.oauth.StartAuthFlow(r.Context(), handle)
	if err != nil {
		s.logger.Error("failed to start OAuth flow", "error", err)

		did, resolveErr := atproto.ResolveHandle(r.Context(), handle)
		if resolveErr != nil {
			s.renderError(w, r, http.StatusBadGateway, "Handle not found", "Could not resolve that handle. Please check and try again.")
			return
		}
		user, createErr := s.dbs.Users.CreateUser(r.Context(), did)
		if createErr != nil {
			s.renderError(w, r, http.StatusInternalServerError, "Sign in failed", "Could not create your account. Please try again.")
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
		s.renderError(w, r, http.StatusBadRequest, "Missing handle", "Please enter your handle.")
		return
	}

	did, err := atproto.ResolveHandle(r.Context(), handle)
	if err != nil {
		s.logger.Error("failed to resolve handle", "error", err)
		s.renderError(w, r, http.StatusBadGateway, "Handle not found", "Could not resolve that handle. Please check and try again.")
		return
	}

	user, err := s.dbs.Users.CreateUser(r.Context(), did)
	if err != nil {
		s.logger.Error("failed to create user", "error", err)
		s.renderError(w, r, http.StatusInternalServerError, "Sign in failed", "Could not create your account. Please try again.")
		return
	}

	s.setUserSession(w, user)
	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

func (s *Server) handleOAuthCallback(w http.ResponseWriter, r *http.Request) {
	sessData, err := s.oauth.ProcessCallback(r.Context(), r.URL.Query())
	if err != nil {
		s.logger.Error("OAuth callback failed", "error", err)
		s.renderError(w, r, http.StatusInternalServerError, "Authentication failed", "Something went wrong during sign in. Please try again.")
		return
	}

	did := sessData.AccountDID.String()

	client := s.pdsClientFromSession(sessData)

	user, err := s.dbs.Users.CreateUser(r.Context(), did)
	if err != nil {
		s.logger.Error("failed to create user", "error", err)
		s.renderError(w, r, http.StatusInternalServerError, "Sign in failed", "Could not create your account. Please try again.")
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
		s.renderError(w, r, http.StatusInternalServerError, "Session error", "Could not create your session. Please try again.")
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
