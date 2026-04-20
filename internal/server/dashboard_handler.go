package server

import (
	"net/http"
	"strconv"
	"time"
)

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)

	unreadCount, _ := s.db.GetUnreadCount(r.Context(), user.DID, "")
	subCount, _ := s.db.GetSubscriptionCount(r.Context(), user.DID)

	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if offset < 0 {
		offset = 0
	}
	pageLimit := 25

	articles, _ := s.db.ListUnreadArticles(r.Context(), user.DID, "", pageLimit+1, offset)
	hasMore := len(articles) > pageLimit
	if hasMore {
		articles = articles[:pageLimit]
	}

	articleRecs, _ := s.db.GetArticleRecommendations(r.Context(), user.DID, 5)
	peopleRecs, _ := s.db.GetPeopleRecommendations(r.Context(), user.DID, 5)
	feedRecs, _ := s.db.GetFeedRecommendations(r.Context(), user.DID, 5)

	since := time.Now().AddDate(0, 0, -7).Format(time.RFC3339)
	personalTrending, _ := s.db.ListTrendingArticlesForUser(r.Context(), user.DID, since, 5, 0)

	s.render(w, r, "dashboard.html", map[string]any{
		"User":                  user,
		"UnreadCount":           unreadCount,
		"SubscriptionCount":     subCount,
		"Articles":              articles,
		"ArticleRecommendations": articleRecs,
		"FeedRecommendations":   feedRecs,
		"PeopleRecommendations": peopleRecs,
		"PersonalTrending":      personalTrending,
		"HasMore":               hasMore,
		"NextOffset":            offset + pageLimit,
	})
}
