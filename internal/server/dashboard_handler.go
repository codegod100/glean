package server

import (
	"context"
	"net/http"
	"time"

	"golang.org/x/sync/errgroup"

	"pkg.rbrt.fr/glean/internal/atproto"
	"pkg.rbrt.fr/glean/internal/cluster"
	"pkg.rbrt.fr/glean/internal/feedback"
)

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	ctx := r.Context()

	var (
		unreadCount      int
		subCount         int
		articles         = make([]Article, 0, 5)
		personalTrending = make([]TrendingItem, 0, 5)
		globalTrending   = make([]TrendingItem, 0, 5)
		digestEnabled    bool
	)

	g, gCtx := errgroup.WithContext(ctx)

	g.Go(func() error {
		var err error
		unreadCount, err = s.dbs.Articles.GetUnreadCount(gCtx, user.DID, "", "")
		if err != nil {
			s.logger.Warn("failed to get unread count", "error", err, "did", user.DID)
		}
		return nil
	})

	g.Go(func() error {
		var err error
		subCount, err = s.dbs.Articles.GetSubscriptionCount(gCtx, user.DID)
		if err != nil {
			s.logger.Warn("failed to get subscription count", "error", err, "did", user.DID)
		}
		return nil
	})

	g.Go(func() error {
		rows, err := s.dbs.Articles.ListUnreadArticles(gCtx, user.DID, "", "", 5, 0, false)
		if err != nil {
			s.logger.Warn("failed to list unread articles", "error", err, "did", user.DID)
			return nil
		}
		articles = make([]Article, len(rows))
		for i, a := range rows {
			articles[i] = toArticle(a)
		}
		return nil
	})

	if err := g.Wait(); err != nil {
		s.logger.Warn("dashboard error", "error", err, "did", user.DID)
	}

	if subCount == 0 {
		rows, err := s.engine.GetGlobalTrending(ctx, user.DID, 5, 0)
		if err != nil {
			s.logger.Warn("failed to get global trending", "error", err, "did", user.DID)
		}
		globalTrending = make([]TrendingItem, len(rows))
		for i, t := range rows {
			globalTrending[i] = toTrendingItem(t)
		}
	} else {
		userLangs, _ := s.dbs.Users.GetLanguages(ctx, user.DID)
		rows, err := s.engine.GetPersonalTrending(ctx, user.DID, userLangs, 5, 0)
		if err != nil {
			s.logger.Warn("failed to get personal trending", "error", err, "did", user.DID)
		}
		personalTrending = make([]TrendingItem, len(rows))
		for i, t := range rows {
			personalTrending[i] = toTrendingItem(t)
		}
	}

	settings, _ := s.dbs.Users.GetSettings(ctx, user.DID)
	if settings != nil {
		digestEnabled = settings.DigestEnabled
	}

	writeJSON(w, http.StatusOK, dashboardResponse{
		User:              toUser(user),
		SubscriptionCount: subCount,
		UnreadCount:       unreadCount,
		Articles:          articles,
		PersonalTrending:  personalTrending,
		GlobalTrending:    globalTrending,
		DigestEnabled:     digestEnabled,
		HasLLM:            s.llm != nil,
		Now:               time.Now().Unix(),
	})
}

func (s *Server) handleArticleRecommendations(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	ctx := r.Context()

	userLangs, _ := s.dbs.Users.GetLanguages(ctx, user.DID)
	articleRecs, err := s.engine.GetArticleRecommendations(ctx, user.DID, userLangs, 5)
	if err != nil {
		s.logger.Warn("failed to get article recommendations", "error", err, "did", user.DID)
		writeJSON(w, http.StatusOK, articleRecsResponse{})
		return
	}

	recs := make([]Article, 0, len(articleRecs))
	for _, rec := range articleRecs {
		recs = append(recs, Article{
			ID:             rec.ArticleID,
			Title:          rec.Title,
			URL:            rec.URL,
			FeedURL:        rec.FeedURL,
			FeedTitle:      rec.FeedTitle,
			FeedFaviconURL: rec.FaviconURL,
			Author:         rec.Author,
			Summary:        rec.Summary,
			Published:      nullTime(rec.Published),
		})
	}

	var impressions []feedback.Impression
	for _, rec := range articleRecs {
		impressions = append(impressions, feedback.Impression{TargetType: "article", TargetID: rec.URL})
	}
	if len(impressions) > 0 {
		if err := s.feedback.RecordImpressions(ctx, user.DID, impressions); err != nil {
			s.logger.Warn("failed to record impressions", "error", err)
		}
	}

	writeJSON(w, http.StatusOK, articleRecsResponse{Articles: recs})
}

func (s *Server) handleFeedRecommendations(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	ctx := r.Context()

	subCount, _ := s.dbs.Articles.GetSubscriptionCount(ctx, user.DID)
	feedRecs, err := s.engine.GetFeedRecommendations(ctx, user.DID, 5)
	if err != nil {
		s.logger.Warn("failed to get feed recommendations", "error", err, "did", user.DID)
		writeJSON(w, http.StatusOK, feedRecsResponse{SubscriptionCount: subCount})
		return
	}

	recs := make([]FeedRecommendation, len(feedRecs))
	for i, rec := range feedRecs {
		recs[i] = toFeedRecommendation(rec)
	}

	var impressions []feedback.Impression
	for _, rec := range feedRecs {
		impressions = append(impressions, feedback.Impression{TargetType: "feed", TargetID: rec.FeedURL})
	}
	if len(impressions) > 0 {
		if err := s.feedback.RecordImpressions(ctx, user.DID, impressions); err != nil {
			s.logger.Warn("failed to record impressions", "error", err)
		}
	}

	writeJSON(w, http.StatusOK, feedRecsResponse{
		Feeds:             recs,
		SubscriptionCount: subCount,
	})
}

func (s *Server) handlePeopleRecommendations(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	ctx := r.Context()

	peopleRecs, err := s.engine.GetPeopleRecommendations(ctx, user.DID, 6)
	if err != nil {
		s.logger.Warn("failed to get people recommendations", "error", err, "did", user.DID)
		writeJSON(w, http.StatusOK, peopleRecsResponse{})
		return
	}

	resolvePeopleHandles(ctx, peopleRecs)

	followed := make([]PersonRecommendation, 0, len(peopleRecs))
	discover := make([]PersonRecommendation, 0, len(peopleRecs))
	for _, p := range peopleRecs {
		if p.IsFollowed {
			followed = append(followed, toPersonRecommendation(p))
		} else {
			discover = append(discover, toPersonRecommendation(p))
		}
	}

	writeJSON(w, http.StatusOK, peopleRecsResponse{
		Followed: followed,
		Discover: discover,
	})
}

func resolvePeopleHandles(ctx context.Context, people []*cluster.PersonRecommendation) {
	g, gCtx := errgroup.WithContext(ctx)
	g.SetLimit(5)
	for _, p := range people {
		g.Go(func() error {
			prof := atproto.ResolveProfile(gCtx, p.DID)
			p.Handle = prof.Handle
			p.DisplayName = prof.DisplayName
			p.AvatarURL = prof.AvatarURL
			return nil
		})
	}
	_ = g.Wait()
}
