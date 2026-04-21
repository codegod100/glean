package server

import (
	"net/http"
	"time"
)

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)

	unreadCount, _ := s.db.GetUnreadCount(r.Context(), user.DID, "")
	subCount, _ := s.db.GetSubscriptionCount(r.Context(), user.DID)

	page := pageFromRequest(r, 25)
	articles, _ := s.db.ListUnreadArticles(r.Context(), user.DID, "", page.Limit()+1, page.Offset())
	totalFetched := len(articles)
	page = page.Paginate(totalFetched)
	if page.HasNext {
		articles = articles[:page.PageSize]
	}

	articleRecs, _ := s.engine.GetArticleRecommendations(r.Context(), user.DID, 5)
	peopleRecs, _ := s.engine.GetPeopleRecommendations(r.Context(), user.DID, 5)
	feedRecs, _ := s.engine.GetFeedRecommendations(r.Context(), user.DID, 5)

	since := time.Now().AddDate(0, 0, -7).Format(time.RFC3339)
	personalTrending, _ := s.db.ListTrendingArticlesForUser(r.Context(), user.DID, since, 5, 0)
	globalTrending, _ := s.db.ListTrendingArticles(r.Context(), user.DID, since, 10, 0)

	s.render(w, r, "dashboard.html", map[string]any{
		"User":                   user,
		"UnreadCount":            unreadCount,
		"SubscriptionCount":      subCount,
		"Articles":               articles,
		"ArticleRecommendations": articleRecs,
		"FeedRecommendations":    feedRecs,
		"PeopleRecommendations":  peopleRecs,
		"PersonalTrending":       personalTrending,
		"GlobalTrending":         globalTrending,
		"Page":                   page,
		"BaseURL":                "/dashboard",
		"QueryParams":            map[string]string{},
		"Now":                    time.Now(),
	})
}
