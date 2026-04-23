package server

import (
	"net/http"
	"time"

	"pkg.rbrt.fr/glean/internal/cluster"
)

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	ctx := r.Context()

	unreadCount, err := s.dbs.Articles.GetUnreadCount(ctx, user.DID, "")
	if err != nil {
		s.logger.Warn("failed to get unread count", "error", err, "did", user.DID)
	}

	subCount, err := s.dbs.Articles.GetSubscriptionCount(ctx, user.DID)
	if err != nil {
		s.logger.Warn("failed to get subscription count", "error", err, "did", user.DID)
	}

	page := pageFromRequest(r, 25)
	articles, err := s.dbs.Articles.ListUnreadArticles(ctx, user.DID, "", page.Limit()+1, page.Offset())
	if err != nil {
		s.logger.Warn("failed to list unread articles", "error", err, "did", user.DID)
	}
	totalFetched := len(articles)
	page = page.Paginate(totalFetched)
	if page.HasNext {
		articles = articles[:page.PageSize]
	}

	articleRecs, err := s.engine.GetArticleRecommendations(ctx, user.DID, 5)
	if err != nil {
		s.logger.Warn("failed to get article recommendations", "error", err, "did", user.DID)
	}

	peopleRecs, err := s.engine.GetPeopleRecommendations(ctx, user.DID, 5)
	if err != nil {
		s.logger.Warn("failed to get people recommendations", "error", err, "did", user.DID)
	}

	feedRecs, err := s.engine.GetFeedRecommendations(ctx, user.DID, 5)
	if err != nil {
		s.logger.Warn("failed to get feed recommendations", "error", err, "did", user.DID)
	}

	var impressions []cluster.Impression
	for _, rec := range feedRecs {
		impressions = append(impressions, cluster.Impression{TargetType: "feed", TargetID: rec.FeedURL})
	}
	for _, rec := range articleRecs {
		impressions = append(impressions, cluster.Impression{TargetType: "article", TargetID: rec.URL})
	}
	if len(impressions) > 0 {
		if err := s.engine.RecordImpressions(ctx, user.DID, impressions); err != nil {
			s.logger.Warn("failed to record impressions", "error", err)
		}
	}

	since := time.Now().AddDate(0, 0, -7).Format(time.RFC3339)

	personalTrending, err := s.dbs.Articles.ListTrendingArticlesForUser(ctx, user.DID, since, 5, 0)
	if err != nil {
		s.logger.Warn("failed to list personal trending", "error", err, "did", user.DID)
	}

	globalTrending, err := s.dbs.Articles.ListTrendingArticles(ctx, user.DID, since, 10, 0)
	if err != nil {
		s.logger.Warn("failed to list global trending", "error", err, "did", user.DID)
	}

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
