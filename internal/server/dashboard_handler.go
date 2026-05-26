package server

import (
	"context"
	"database/sql"
	"net/http"
	"time"

	"golang.org/x/sync/errgroup"

	"pkg.rbrt.fr/glean/internal/atproto"
	"pkg.rbrt.fr/glean/internal/cluster"
	"pkg.rbrt.fr/glean/internal/db"
	"pkg.rbrt.fr/glean/internal/feedback"
)

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	ctx := r.Context()

	var (
		unreadCount      int
		subCount         int
		userLangs        []string
		articles         []*db.Article
		articleRecs      []*cluster.ArticleRecommendation
		peopleRecs       []*cluster.PersonRecommendation
		feedRecs         []*cluster.FeedRecommendation
		personalTrending []*db.TrendingItem
		globalTrending   []*db.TrendingItem
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
		var err error
		articles, err = s.dbs.Articles.ListUnreadArticles(gCtx, user.DID, "", "", 5, 0, false)
		if err != nil {
			s.logger.Warn("failed to list unread articles", "error", err, "did", user.DID)
		}
		return nil
	})

	g.Go(func() error {
		userLangs, _ = s.dbs.Users.GetLanguages(gCtx, user.DID)
		return nil
	})

	g.Go(func() error {
		var err error
		peopleRecs, err = s.engine.GetPeopleRecommendations(gCtx, user.DID, 6)
		return err
	})

	g.Go(func() error {
		var err error
		feedRecs, err = s.engine.GetFeedRecommendations(gCtx, user.DID, 5)
		return err
	})

	if err := g.Wait(); err != nil {
		s.logger.Warn("dashboard error", "error", err, "did", user.DID)
	}

	var err error
	if subCount == 0 {
		globalTrending, err = s.engine.GetGlobalTrending(gCtx, user.DID, 5, 0)
		if err != nil {
			s.logger.Warn("failed to get global trending", "error", err, "did", user.DID)
		}
	} else {
		personalTrending, err = s.engine.GetPersonalTrending(gCtx, user.DID, userLangs, 5, 0)
		if err != nil {
			s.logger.Warn("failed to get personal trending", "error", err, "did", user.DID)
		}

		articleRecs, err = s.engine.GetArticleRecommendations(ctx, user.DID, userLangs, 5)
		if err != nil {
			s.logger.Warn("failed to get article recommendations", "error", err, "did", user.DID)
		}
	}

	resolvePeopleHandles(ctx, peopleRecs)

	articleRecArticles := make([]*db.Article, len(articleRecs))
	for i, rec := range articleRecs {
		articleRecArticles[i] = &db.Article{
			ID:             rec.ArticleID,
			FeedURL:        rec.FeedURL,
			FeedTitle:      rec.FeedTitle,
			FeedFaviconURL: sql.NullString{String: rec.FaviconURL, Valid: rec.FaviconURL != ""},
			Title:          rec.Title,
			URL:            sql.NullString{String: rec.URL, Valid: rec.URL != ""},
			Author:         sql.NullString{String: rec.Author, Valid: rec.Author != ""},
			Summary:        sql.NullString{String: rec.Summary, Valid: rec.Summary != ""},
			Published:      rec.Published,
			IsRead:         sql.NullBool{Bool: false, Valid: true},
			DismissURL:     "/recs/dismiss-article",
			DismissField:   "article_url",
			DismissValue:   rec.URL,
		}
	}

	var impressions []feedback.Impression
	for _, rec := range articleRecs {
		impressions = append(impressions, feedback.Impression{TargetType: "article", TargetID: rec.URL})
	}
	for _, rec := range feedRecs {
		impressions = append(impressions, feedback.Impression{TargetType: "feed", TargetID: rec.FeedURL})
	}
	if len(impressions) > 0 {
		if err := s.feedback.RecordImpressions(ctx, user.DID, impressions); err != nil {
			s.logger.Warn("failed to record impressions", "error", err)
		}
	}

	var followedPeople, discoverPeople []*cluster.PersonRecommendation
	for _, p := range peopleRecs {
		if p.IsFollowed {
			followedPeople = append(followedPeople, p)
		} else {
			discoverPeople = append(discoverPeople, p)
		}
	}

	settings, _ := s.dbs.Users.GetSettings(ctx, user.DID)
	digestEnabled := settings != nil && settings.DigestEnabled

	s.render(w, r, "dashboard.html", map[string]any{
		"User":                   user,
		"SubscriptionCount":      subCount,
		"UnreadCount":            unreadCount,
		"Articles":               articles,
		"ArticleRecommendations": articleRecArticles,
		"FeedRecommendations":    feedRecs,
		"FollowedPeople":         followedPeople,
		"DiscoverPeople":         discoverPeople,
		"PersonalTrending":       personalTrending,
		"GlobalTrending":         globalTrending,
		"Now":                    time.Now(),
		"DigestEnabled":          digestEnabled,
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
