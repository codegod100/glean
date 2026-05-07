package server

import (
	"context"
	"net/http"
	"time"

	"golang.org/x/sync/errgroup"

	"pkg.rbrt.fr/glean/internal/atproto"
	"pkg.rbrt.fr/glean/internal/cluster"
	"pkg.rbrt.fr/glean/internal/db"
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

	page := pageFromRequest(r, 25)
	since := time.Now().AddDate(0, 0, -7).Format(time.RFC3339)

	g, gCtx := errgroup.WithContext(ctx)

	g.Go(func() error {
		var err error
		unreadCount, err = s.dbs.Articles.GetUnreadCount(gCtx, user.DID, "")
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
		articles, err = s.dbs.Articles.ListUnreadArticles(gCtx, user.DID, "", page.Limit()+1, page.Offset())
		if err != nil {
			s.logger.Warn("failed to list unread articles", "error", err, "did", user.DID)
			return nil
		}
		totalFetched := len(articles)
		page = page.Paginate(totalFetched)
		if page.HasNext {
			articles = articles[:page.PageSize]
		}
		return nil
	})

	g.Go(func() error {
		var err error
		userLangs, err = s.dbs.Users.GetLanguages(gCtx, user.DID)
		if err != nil {
			s.logger.Warn("failed to get user languages", "error", err, "did", user.DID)
		}
		return nil
	})

	g.Go(func() error {
		var err error
		peopleRecs, err = s.engine.GetPeopleRecommendations(gCtx, user.DID, 5)
		if err != nil {
			s.logger.Warn("failed to get people recommendations", "error", err, "did", user.DID)
		}
		return nil
	})

	g.Go(func() error {
		var err error
		feedRecs, err = s.engine.GetFeedRecommendations(gCtx, user.DID, 5)
		if err != nil {
			s.logger.Warn("failed to get feed recommendations", "error", err, "did", user.DID)
		}
		return nil
	})

	g.Go(func() error {
		var err error
		globalTrending, err = s.dbs.Articles.ListTrendingArticles(gCtx, user.DID, since, 10, 0)
		if err != nil {
			s.logger.Warn("failed to list global trending", "error", err, "did", user.DID)
		}
		return nil
	})

	if err := g.Wait(); err != nil {
		s.logger.Warn("dashboard phase 1 error", "error", err, "did", user.DID)
	}

	g2, gCtx2 := errgroup.WithContext(ctx)

	g2.Go(func() error {
		var err error
		articleRecs, err = s.engine.GetArticleRecommendations(gCtx2, user.DID, userLangs, 5)
		if err != nil {
			s.logger.Warn("failed to get article recommendations", "error", err, "did", user.DID)
		}
		return nil
	})

	g2.Go(func() error {
		var err error
		personalTrending, err = s.dbs.Articles.ListTrendingArticlesForUser(gCtx2, user.DID, since, userLangs, 5, 0)
		if err != nil {
			s.logger.Warn("failed to list personal trending", "error", err, "did", user.DID)
		}
		return nil
	})

	g2.Go(func() error {
		resolvePeopleHandles(gCtx2, peopleRecs)
		return nil
	})

	if err := g2.Wait(); err != nil {
		s.logger.Warn("dashboard phase 2 error", "error", err, "did", user.DID)
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

	var followedPeople, discoverPeople []*cluster.PersonRecommendation
	for _, p := range peopleRecs {
		if p.IsFollowed {
			followedPeople = append(followedPeople, p)
		} else {
			discoverPeople = append(discoverPeople, p)
		}
	}

	s.render(w, r, "dashboard.html", map[string]any{
		"User":                   user,
		"UnreadCount":            unreadCount,
		"SubscriptionCount":      subCount,
		"Articles":               articles,
		"ArticleRecommendations": articleRecs,
		"FeedRecommendations":    feedRecs,
		"FollowedPeople":         followedPeople,
		"DiscoverPeople":         discoverPeople,
		"PersonalTrending":       personalTrending,
		"GlobalTrending":         globalTrending,
		"Page":                   page,
		"BaseURL":                "/dashboard",
		"QueryParams":            map[string]string{},
		"Now":                    time.Now(),
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
