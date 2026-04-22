package server

import (
	"net/http"
	"time"

	"pkg.rbrt.fr/glean/internal/cluster"
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

	var impressions []cluster.Impression
	for _, rec := range feedRecs {
		impressions = append(impressions, cluster.Impression{TargetType: "feed", TargetID: rec.FeedURL})
	}
	for _, rec := range articleRecs {
		impressions = append(impressions, cluster.Impression{TargetType: "article", TargetID: rec.URL})
	}
	if len(impressions) > 0 {
		_ = s.engine.RecordImpressions(r.Context(), user.DID, impressions)
	}

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

func (s *Server) handleDismissArticleRecommendation(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	articleURL := r.FormValue("article_url")
	if articleURL == "" {
		http.Error(w, "article_url required", http.StatusBadRequest)
		return
	}

	reason := r.FormValue("reason")
	if reason == "" {
		reason = "not_interested"
	}

	if err := s.engine.DismissArticle(r.Context(), user.DID, articleURL, reason); err != nil {
		s.logger.Error("failed to dismiss article recommendation", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}
