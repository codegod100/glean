package server

import (
	"net/http"
	"time"

	"pkg.rbrt.fr/glean/internal/db"
)

func (s *Server) handleTrending(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)

	scope := r.URL.Query().Get("scope")
	if scope != "all" {
		scope = "for-me"
	}

	page := pageFromRequest(r, 25)
	since := time.Now().AddDate(0, 0, -7).Format(time.RFC3339)

	var trending []*db.TrendingItem
	if scope == "for-me" {
		trending, _ = s.db.ListTrendingArticlesForUser(r.Context(), user.DID, since, page.FetchLimit(), page.Offset)
	} else {
		trending, _ = s.db.ListTrendingArticles(r.Context(), since, page.FetchLimit(), page.Offset)
	}

	page = page.Paginate(len(trending))
	if page.HasMore {
		trending = trending[:page.Limit]
	}

	s.render(w, r, "trending.html", map[string]any{
		"User":     user,
		"Trending": trending,
		"Scope":    scope,
		"Page":     page,
	})
}
