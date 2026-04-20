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
	articles, _ := s.db.ListUnreadArticles(r.Context(), user.DID, "", page.FetchLimit(), page.Offset)
	page = page.Paginate(len(articles))
	if page.HasMore {
		articles = articles[:page.Limit]
	}

	articleRecs, _ := s.db.GetArticleRecommendations(r.Context(), user.DID, 5)
	peopleRecs, _ := s.db.GetPeopleRecommendations(r.Context(), user.DID, 5)
	feedRecs, _ := s.db.GetFeedRecommendations(r.Context(), user.DID, 5)

	since := time.Now().AddDate(0, 0, -7).Format(time.RFC3339)
	personalTrending, _ := s.db.ListTrendingArticlesForUser(r.Context(), user.DID, since, 5, 0)

	s.render(w, r, "dashboard.html", map[string]any{
		"User":                   user,
		"UnreadCount":            unreadCount,
		"SubscriptionCount":      subCount,
		"Articles":               articles,
		"ArticleRecommendations": articleRecs,
		"FeedRecommendations":    feedRecs,
		"PeopleRecommendations":  peopleRecs,
		"PersonalTrending":       personalTrending,
		"Page":                   page,
	})
}
