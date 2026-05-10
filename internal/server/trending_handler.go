package server

import (
	"net/http"

	"pkg.rbrt.fr/glean/internal/db"
)

const scopeForMe = "for-me"

func (s *Server) handleTrending(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	ctx := r.Context()

	scope := r.URL.Query().Get("scope")
	if scope == scopeForMe && user == nil {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}
	if scope != scopeForMe {
		scope = "all"
	}

	page := pageFromRequest(r, 25)

	var userDID string
	if user != nil {
		userDID = user.DID
	}

	var trending []*db.TrendingItem
	var err error

	if scope == scopeForMe {
		var userLangs []string
		if user != nil {
			userLangs, _ = s.dbs.Users.GetLanguages(ctx, user.DID)
		}
		trending, err = s.engine.GetPersonalTrending(ctx, userDID, userLangs, page.Limit()+1, page.Offset())
	} else {
		trending, err = s.engine.GetGlobalTrending(ctx, userDID, page.Limit()+1, page.Offset())
	}
	if err != nil {
		s.logger.Warn("failed to list trending articles", "error", err, "scope", scope)
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
