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

func (s *Server) handleAuthStart(w http.ResponseWriter, r *http.Request) {
	handle := strings.TrimPrefix(r.FormValue("handle"), "@")
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
		user, createErr := s.dbs.Users.CreateUser(r.Context(), did, handle, "", "")
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

	user, err := s.dbs.Users.CreateUser(r.Context(), did, handle, "", "")
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

	client := s.pdsClientFromSession(sessData)

	var displayName, avatarURL string
	if client != nil {
		if dn, avatar, err := client.GetProfile(r.Context(), did); err == nil {
			displayName = dn
			avatarURL = avatar
		}
	}

	user, err := s.dbs.Users.CreateUser(r.Context(), did, handle, displayName, avatarURL)
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
