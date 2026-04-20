package server

import (
	"net/http"
	"time"
)

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)

	unreadCount, _ := s.db.GetUnreadCount(r.Context(), user.DID, "")
	subCount, _ := s.db.GetSubscriptionCount(r.Context(), user.DID)
	articles, _ := s.db.ListUnreadArticles(r.Context(), user.DID, "", 25, 0)
	feedRecs, _ := s.db.GetFeedRecommendations(r.Context(), user.DID, 5)
	peopleRecs, _ := s.db.GetPeopleRecommendations(r.Context(), user.DID, 5)
	since := time.Now().AddDate(0, 0, -7).Format(time.RFC3339)
	trending, _ := s.db.ListTrendingArticles(r.Context(), since, 5, 0)

	s.render(w, r, "dashboard.html", map[string]any{
		"User":                  user,
		"UnreadCount":           unreadCount,
		"SubscriptionCount":     subCount,
		"Articles":              articles,
		"FeedRecommendations":   feedRecs,
		"PeopleRecommendations": peopleRecs,
		"Trending":              trending,
	})
}
