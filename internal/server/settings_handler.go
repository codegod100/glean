package server

import (
	"net/http"

	"pkg.rbrt.fr/glean/internal/ml"
)

func (s *Server) handleToggleLanguage(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)

	lang := r.PathValue("code")
	if lang == "" {
		http.Error(w, "missing language code", http.StatusBadRequest)
		return
	}

	valid := make(map[string]bool)
	for _, known := range ml.KnownLanguages() {
		valid[known.Code] = true
	}
	if !valid[lang] {
		http.Error(w, "unknown language", http.StatusBadRequest)
		return
	}

	current, err := s.dbs.Users.GetLanguages(r.Context(), user.DID)
	if err != nil {
		s.logger.Error("failed to get languages", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var updated []string
	found := false
	for _, l := range current {
		if l == lang {
			found = true
			continue
		}
		updated = append(updated, l)
	}
	if !found {
		updated = append(updated, lang)
	}

	if err := s.dbs.Users.UpdateLanguages(r.Context(), user.DID, updated); err != nil {
		s.logger.Error("failed to update languages", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set(HXRedirect, "/profile/"+user.DID)
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

func (s *Server) handleToggleDigestEnabled(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)

	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	enabled := r.FormValue("digest_enabled") == "1"

	if err := s.dbs.Users.SetDigestEnabled(r.Context(), user.DID, enabled); err != nil {
		s.logger.Error("failed to update digest enabled", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("HX-Redirect", "/profile/"+user.DID)
	w.WriteHeader(http.StatusOK)
}
