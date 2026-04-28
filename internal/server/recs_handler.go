package server

import (
	"net/http"
)

func (s *Server) handleDismissFeedRecommendation(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	feedURL := r.FormValue("feed_url")
	if feedURL == "" {
		http.Error(w, "feed_url required", http.StatusBadRequest)
		return
	}

	reason := r.FormValue("reason")
	if reason == "" {
		reason = "not_interested"
	}

	if err := s.engine.DismissFeed(r.Context(), user.DID, feedURL, reason); err != nil {
		s.logger.Error("failed to dismiss feed recommendation", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleDismissArticleRecommendation(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	articleURL := r.FormValue("article_url")
	if articleURL == "" {
		http.Error(w, "article_url required", http.StatusBadRequest)
		return
	}

	reason := r.FormValue("reason")
	if reason == "" {
		reason = "not_interested"
	}

	if err := s.engine.DismissArticle(r.Context(), user.DID, articleURL, reason); err != nil {
		s.logger.Error("failed to dismiss article recommendation", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleDismissPersonRecommendation(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	targetDID := r.FormValue("target_did")
	if targetDID == "" {
		http.Error(w, "target_did required", http.StatusBadRequest)
		return
	}

	reason := r.FormValue("reason")
	if reason == "" {
		reason = "not_interested"
	}

	if err := s.engine.DismissPerson(r.Context(), user.DID, targetDID, reason); err != nil {
		s.logger.Error("failed to dismiss person recommendation", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}
