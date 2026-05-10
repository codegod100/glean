package server

import (
	"net/http"
)

func (s *Server) handleDismissFeedRecommendation(w http.ResponseWriter, r *http.Request) {
	s.handleDismiss(w, r, "feed_url", "feed")
}

func (s *Server) handleDismissArticleRecommendation(w http.ResponseWriter, r *http.Request) {
	s.handleDismiss(w, r, "article_url", "article")
}

func (s *Server) handleDismissPersonRecommendation(w http.ResponseWriter, r *http.Request) {
	s.handleDismiss(w, r, "target_did", "person")
}

func (s *Server) handleDismiss(w http.ResponseWriter, r *http.Request, field, targetType string) {
	user := currentUser(r)
	targetID := r.FormValue(field)
	if targetID == "" {
		http.Error(w, field+" required", http.StatusBadRequest)
		return
	}

	reason := r.FormValue("reason")
	if reason == "" {
		reason = "not_interested"
	}

	if err := s.feedback.Dismiss(r.Context(), user.DID, targetType, targetID, reason); err != nil {
		s.logger.Error("failed to dismiss recommendation", "error", err, "type", targetType)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}
