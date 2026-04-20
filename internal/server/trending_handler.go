package server

import (
	"net/http"
	"time"
)

func (s *Server) handleTrending(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	since := time.Now().AddDate(0, 0, -7).Format(time.RFC3339)
	articles, _ := s.db.ListTrendingArticles(r.Context(), since, 25, 0)
	s.render(w, r, "trending.html", map[string]any{
		"User":     user,
		"Trending": articles,
	})
}
