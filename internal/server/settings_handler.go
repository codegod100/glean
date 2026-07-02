package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"pkg.rbrt.fr/glean/internal/ml"
)

func (s *Server) handleToggleLanguage(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)

	lang := chi.URLParam(r, "code")
	if lang == "" {
		writeAPIError(w, http.StatusBadRequest, "missing language code")
		return
	}

	valid := make(map[string]bool)
	for _, known := range ml.KnownLanguages() {
		valid[known.Code] = true
	}
	if !valid[lang] {
		writeAPIError(w, http.StatusBadRequest, "unknown language")
		return
	}

	current, err := s.dbs.Users.GetLanguages(r.Context(), user.DID)
	if err != nil {
		s.logger.Error("failed to get languages", "error", err)
		writeAPIError(w, http.StatusInternalServerError, err.Error())
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
		writeAPIError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, languagesResponse{Languages: nonNil(updated)})
}

func (s *Server) handleToggleExpandedView(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)

	if err := r.ParseForm(); err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}

	enabled := r.FormValue("expanded_view") == "1"

	if err := s.dbs.Users.SetExpandedView(r.Context(), user.DID, enabled); err != nil {
		s.logger.Error("failed to update expanded view", "error", err)
		writeAPIError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, expandedViewResponse{ExpandedView: enabled})
}

func (s *Server) handleToggleDigestEnabled(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)

	if err := r.ParseForm(); err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}

	enabled := r.FormValue("digest_enabled") == "1"

	if err := s.dbs.Users.SetDigestEnabled(r.Context(), user.DID, enabled); err != nil {
		s.logger.Error("failed to update digest enabled", "error", err)
		writeAPIError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, digestEnabledResponse{DigestEnabled: enabled})
}
