package server

import (
	"net/http"

	"pkg.rbrt.fr/glean/internal/atproto"
)

func (s *Server) handleAuthLogin(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "login.html", map[string]any{})
}

func (s *Server) handleAuthCallback(w http.ResponseWriter, r *http.Request) {
	handle := r.URL.Query().Get("handle")
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

func (s *Server) handleAuthLogout(w http.ResponseWriter, r *http.Request) {
	s.clearUserSession(w)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
