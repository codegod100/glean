package server

import (
	"net/http"
	"time"

	"pkg.rbrt.fr/glean/internal/db"
)

func (s *Server) handleTrending(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)

	scope := r.URL.Query().Get("scope")
	if scope != "for-me" {
		scope = "all"
	}

	page := pageFromRequest(r, 25)
	since := time.Now().AddDate(0, 0, -7).Format(time.RFC3339)

	var trending []*db.TrendingItem
	if scope == "for-me" {
		trending, _ = s.db.ListTrendingArticlesForUser(r.Context(), user.DID, since, page.Limit()+1, page.Offset())
	} else {
		trending, _ = s.db.ListTrendingArticles(r.Context(), since, page.Limit()+1, page.Offset())
	}

	totalFetched := len(trending)
	page = page.Paginate(totalFetched)
	if page.HasNext {
		trending = trending[:page.PageSize]
	}

	s.render(w, r, "trending.html", map[string]any{
		"User":        user,
		"Trending":    trending,
		"Scope":       scope,
		"Page":        page,
		"BaseURL":     "/trending",
		"QueryParams": buildQueryParams(map[string]string{"scope": scope}),
	})
}
