package server

import (
	"net/http"
	"strings"

	"pkg.rbrt.fr/glean/internal/ml"
)

func (s *Server) handleUpdateLanguages(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)

	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	valid := make(map[string]bool)
	for _, known := range ml.KnownLanguages() {
		valid[known.Code] = true
	}

	var filtered []string
	for _, l := range r.Form["languages"] {
		l = strings.TrimSpace(l)
		if valid[l] {
			filtered = append(filtered, l)
		}
	}

	if err := s.dbs.Users.UpdateLanguages(r.Context(), user.DID, filtered); err != nil {
		s.logger.Error("failed to update languages", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("HX-Redirect", "/profile/"+user.DID)
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleToggleExpandedView(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)

	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	enabled := r.FormValue("expanded_view") == "1"

	if err := s.dbs.Users.SetExpandedView(r.Context(), user.DID, enabled); err != nil {
		s.logger.Error("failed to update expanded view", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("HX-Redirect", "/profile/"+user.DID)
	w.WriteHeader(http.StatusOK)
}
