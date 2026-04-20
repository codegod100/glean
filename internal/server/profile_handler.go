package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

func (s *Server) handleProfile(w http.ResponseWriter, r *http.Request) {
	did := chi.URLParam(r, "did")
	profileUser, err := s.db.GetUser(r.Context(), did)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	subs, _ := s.db.ListSubscriptions(r.Context(), did, "", 50, 0)
	annotations, _ := s.db.ListAnnotations(r.Context(), "", "", did, 50, 0)
	subCount, _ := s.db.GetSubscriptionCount(r.Context(), did)

	s.render(w, r, "profile.html", map[string]any{
		"User":            s.getUserFromSession(r),
		"ProfileUser":     profileUser,
		"Subscriptions":   subs,
		"Annotations":     annotations,
		"SubscriptionCount": subCount,
		"AnnotationCount": len(annotations),
	})
}
