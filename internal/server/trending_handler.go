package server

import (
	"net/http"
	"strconv"
	"time"

	"pkg.rbrt.fr/glean/internal/db"
)

func (s *Server) handleTrending(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)

	scope := r.URL.Query().Get("scope")
	if scope != "all" {
		scope = "for-me"
	}

	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if offset < 0 {
		offset = 0
	}
	pageLimit := 25

	since := time.Now().AddDate(0, 0, -7).Format(time.RFC3339)

	var trending []*db.TrendingItem
	if scope == "for-me" {
		trending, _ = s.db.ListTrendingArticlesForUser(r.Context(), user.DID, since, pageLimit+1, offset)
	} else {
		trending, _ = s.db.ListTrendingArticles(r.Context(), since, pageLimit+1, offset)
	}

	hasMore := len(trending) > pageLimit
	if hasMore {
		trending = trending[:pageLimit]
	}

	s.render(w, r, "trending.html", map[string]any{
		"User":       user,
		"Trending":   trending,
		"Scope":      scope,
		"HasMore":    hasMore,
		"NextOffset": offset + pageLimit,
	})
}
