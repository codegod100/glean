package server

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"time"

	"golang.org/x/sync/errgroup"

	"pkg.rbrt.fr/glean/internal/atproto"
	"pkg.rbrt.fr/glean/internal/cluster"
	"pkg.rbrt.fr/glean/internal/db"
	"pkg.rbrt.fr/glean/internal/feed"
	"pkg.rbrt.fr/glean/internal/feedback"
)

func (s *Server) handleFeeds(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	category := r.URL.Query().Get("category")
	ctx := r.Context()

	page := pageFromRequest(r, 50)

	var (
		subs       []*db.Subscription
		subCount   int
		feedRecs   []*cluster.FeedRecommendation
		peopleRecs []*cluster.PersonRecommendation
		deadFeeds  []*db.Feed
		categories []string
	)

	g, gCtx := errgroup.WithContext(ctx)

	g.Go(func() error {
		var err error
		subs, err = s.dbs.Articles.ListSubscriptions(gCtx, user.DID, category, page.Limit()+1, page.Offset())
		if err != nil {
			s.logger.Warn("failed to list subscriptions", "error", err, "did", user.DID)
			return nil
		}
		totalFetched := len(subs)
		page = page.Paginate(totalFetched)
		if page.HasNext {
			subs = subs[:page.PageSize]
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
		feedRecs, err = s.engine.GetFeedRecommendations(gCtx, user.DID, 6)
		if err != nil {
			s.logger.Warn("failed to get feed recommendations", "error", err, "did", user.DID)
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
		deadFeeds, err = s.dbs.Articles.ListDeadFeeds(gCtx, user.DID, 7)
		if err != nil {
			s.logger.Warn("failed to list dead feeds", "error", err, "did", user.DID)
		}
		return nil
	})

	g.Go(func() error {
		var err error
		categories, err = s.dbs.Articles.GetCategories(gCtx, user.DID)
		if err != nil {
			s.logger.Warn("failed to get categories", "error", err, "did", user.DID)
		}
		return nil
	})

	if err := g.Wait(); err != nil {
		s.logger.Warn("feeds phase 1 error", "error", err, "did", user.DID)
	}

	g2, gCtx2 := errgroup.WithContext(ctx)

	g2.Go(func() error {
		resolvePeopleHandles(gCtx2, peopleRecs)
		return nil
	})

	g2.Go(func() error {
		if len(feedRecs) > 0 {
			impressions := make([]feedback.Impression, len(feedRecs))
			for i, rec := range feedRecs {
				impressions[i] = feedback.Impression{TargetType: "feed", TargetID: rec.FeedURL}
			}
			if err := s.feedback.RecordImpressions(gCtx2, user.DID, impressions); err != nil {
				s.logger.Warn("failed to record impressions", "error", err)
			}
		}
		return nil
	})

	if err := g2.Wait(); err != nil {
		s.logger.Warn("feeds phase 2 error", "error", err, "did", user.DID)
	}

	var followedPeople, discoverPeople []*cluster.PersonRecommendation
	for _, p := range peopleRecs {
		if p.IsFollowed {
			followedPeople = append(followedPeople, p)
		} else {
			discoverPeople = append(discoverPeople, p)
		}
	}

	s.render(w, r, "feeds.html", map[string]any{
		"User":                user,
		"Subscriptions":       subs,
		"SubscriptionCount":   subCount,
		"Categories":          categories,
		"Category":            category,
		"FeedRecommendations": feedRecs,
		"FollowedPeople":      followedPeople,
		"DiscoverPeople":      discoverPeople,
		"DeadFeeds":           deadFeeds,
		"Page":                page,
		"BaseURL":             "/feeds",
		"QueryParams":         buildQueryParams(map[string]string{"category": category}),
	})
}

func (s *Server) storeFetchResult(ctx context.Context, feedURL, siteURL string, result *feed.ParseResult) {
	if result == nil {
		_ = s.dbs.Articles.MarkFeedFetched(ctx, feedURL)
		return
	}
	faviconURL := result.Feed.FaviconURL
	if faviconURL == "" {
		faviconURL = feed.ResolveFavicon(ctx, feedURL, siteURL)
	}
	if len(result.Articles) > 0 {
		if err := s.dbs.Articles.BatchUpsertArticles(ctx, result.Articles); err != nil {
			s.logger.Error("failed to store articles", "error", err, "feed", feedURL)
		}
	}
	_ = s.dbs.Articles.MarkFeedFetched(ctx, feedURL)
	if faviconURL != "" {
		_ = s.dbs.Articles.UpdateFeedFavicon(ctx, feedURL, faviconURL)
	}
}

func (s *Server) handleAddFeed(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	feedURL := r.FormValue("feed_url")
	category := r.FormValue("category")

	if feedURL == "" {
		http.Error(w, "url required", http.StatusBadRequest)
		return
	}

	if !atproto.IsATProtoFeedURL(feedURL) {
		u, err := url.Parse(feedURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			http.Error(w, "invalid feed URL", http.StatusBadRequest)
			return
		}
	}

	result, feedURL, err := s.fetcher.FetchOrDiscover(r.Context(), feedURL)
	if err != nil {
		s.logger.Error("failed to fetch feed", "error", err, "url", feedURL)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var feedTitle, faviconURL string
	if result != nil {
		feedTitle = result.Feed.Title
		faviconURL = result.Feed.FaviconURL
	}

	f := &db.Feed{
		FeedURL:     feedURL,
		Title:       db.NullStr(feedTitle),
		SiteURL:     db.NullStr(result.Feed.SiteURL),
		Description: db.NullStr(result.Feed.Description),
		FeedType:    db.NullStr(result.Feed.Type),
		FaviconURL:  db.NullStr(faviconURL),
	}
	if err := s.dbs.Articles.UpsertFeed(r.Context(), f); err != nil {
		s.logger.Error("failed to upsert feed", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var subURI, subCID string
	if client := s.pdsClientForUser(r); client != nil {
		record := atproto.SubscriptionRecord{
			CreatedAt: time.Now().Format(time.RFC3339),
			FeedURL:   feedURL,
			Title:     feedTitle,
			Category:  category,
		}
		uri, cid, err := client.CreateRecord(r.Context(), user.DID, atproto.CollectionSubscription, record)
		if err != nil {
			s.logger.Error("failed to write subscription to PDS", "error", err)
			http.Error(w, "failed to write subscription to PDS: "+err.Error(), http.StatusInternalServerError)
			return
		}
		subURI = uri
		subCID = cid
	}

	if err := s.dbs.Articles.CreateSubscription(r.Context(), user.DID, feedURL, feedTitle, category, subURI, subCID); err != nil {
		if errors.Is(err, db.ErrDuplicateSubscription) {
			http.Error(w, "Already subscribed to this feed.", http.StatusConflict)
			return
		}
		s.logger.Error("failed to create subscription", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	go s.storeFetchResult(context.WithoutCancel(r.Context()), feedURL, result.Feed.SiteURL, result)

	if err := s.feedback.MarkImpressionActed(r.Context(), user.DID, "feed", feedURL); err != nil {
		s.logger.Warn("failed to mark impression acted", "error", err)
	}
	sig := s.engine.GetDominantSignal(s.engine.GetWeights(r.Context(), user.DID))
	s.engine.RewardSignal(r.Context(), user.DID, sig)

	sub, err := s.dbs.Articles.GetSubscription(r.Context(), user.DID, feedURL)
	if err != nil {
		s.logger.Warn("failed to get subscription", "error", err)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	s.render(w, r, "feed-item.html", map[string]any{
		"User":        user,
		"ID":          sub.ID,
		"FeedURL":     sub.FeedURL,
		"FeedTitle":   sub.FeedTitle,
		"Category":    sub.Category,
		"FaviconURL":  sub.FaviconURL,
		"UnreadCount": sub.UnreadCount,
	})
}

func (s *Server) handleEditFeed(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	feedURL := r.FormValue("feed_url")
	category := r.FormValue("category")

	if feedURL == "" {
		http.Error(w, "url required", http.StatusBadRequest)
		return
	}

	sub, err := s.dbs.Articles.GetSubscription(r.Context(), user.DID, feedURL)
	if err != nil {
		s.logger.Error("failed to get subscription", "error", err)
		http.Error(w, "subscription not found", http.StatusNotFound)
		return
	}

	cid := sub.CID.String

	if sub.URI.Valid && sub.URI.String != "" {
		if client := s.pdsClientForUser(r); client != nil {
			parsed, ok := atproto.ParseRecordURI(sub.URI.String)
			if ok {
				record := atproto.SubscriptionRecord{
					CreatedAt: time.Now().Format(time.RFC3339),
					FeedURL:   feedURL,
					Title:     sub.FeedTitle,
					Category:  category,
				}
				_, newCID, putErr := client.PutRecord(r.Context(), user.DID, parsed.Collection, parsed.RKey, record)
				if putErr != nil {
					s.logger.Error("failed to put subscription record on PDS", "error", putErr)
					http.Error(w, "failed to update subscription on PDS: "+putErr.Error(), http.StatusInternalServerError)
					return
				}
				cid = newCID
			}
		}
	}

	if err := s.dbs.Articles.UpdateSubscription(r.Context(), user.DID, feedURL, sub.FeedTitle, category, sub.URI.String, cid); err != nil {
		s.logger.Error("failed to update subscription", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	updated, err := s.dbs.Articles.GetSubscription(r.Context(), user.DID, feedURL)
	if err != nil {
		s.logger.Warn("failed to get updated subscription", "error", err)
		w.WriteHeader(http.StatusNoContent)
		return
	}

	s.render(w, r, "feed-item.html", map[string]any{
		"User":        user,
		"ID":          updated.ID,
		"FeedURL":     updated.FeedURL,
		"FeedTitle":   updated.FeedTitle,
		"Category":    updated.Category,
		"FaviconURL":  updated.FaviconURL,
		"UnreadCount": updated.UnreadCount,
	})
}

func (s *Server) handleRemoveFeed(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	feedURL := r.FormValue("url")

	if feedURL == "" {
		http.Error(w, "url required", http.StatusBadRequest)
		return
	}

	sub, err := s.dbs.Articles.GetSubscription(r.Context(), user.DID, feedURL)
	if err == nil && sub.URI.Valid {
		if client := s.pdsClientForUser(r); client != nil {
			parsed, ok := atproto.ParseRecordURI(sub.URI.String)
			if ok {
				if delErr := client.DeleteRecord(r.Context(), user.DID, parsed.Collection, parsed.RKey); delErr != nil {
					s.logger.Error("failed to delete subscription from PDS", "error", delErr)
					http.Error(w, "failed to delete subscription from PDS: "+delErr.Error(), http.StatusInternalServerError)
					return
				}
			}
		}
	}

	if err := s.dbs.Articles.DeleteSubscription(r.Context(), user.DID, feedURL); err != nil {
		s.logger.Error("failed to delete subscription", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleClearAllSubscriptions(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)

	subs, err := s.dbs.Articles.ListSubscriptions(r.Context(), user.DID, "", 1000, 0)
	if err != nil {
		s.logger.Warn("failed to list subscriptions", "error", err, "did", user.DID)
	}
	if client := s.pdsClientForUser(r); client != nil {
		for _, sub := range subs {
			if sub.URI.Valid {
				parsed, ok := atproto.ParseRecordURI(sub.URI.String)
				if ok {
					if delErr := client.DeleteRecord(r.Context(), user.DID, parsed.Collection, parsed.RKey); delErr != nil {
						s.logger.Error("failed to delete subscription from PDS", "error", delErr, "uri", sub.URI.String)
						http.Error(w, "failed to delete subscription from PDS: "+delErr.Error(), http.StatusInternalServerError)
						return
					}
				}
			}
		}
	}

	if err := s.dbs.Articles.DeleteAllSubscriptions(r.Context(), user.DID); err != nil {
		s.logger.Error("failed to clear subscriptions", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("HX-Redirect", "/feeds")
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleOPMLUpload(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	file, _, err := r.FormFile("opml")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer file.Close()

	opml, err := feed.ParseOPML(file)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	feedURLs := feed.ExtractFeedURLs(opml)
	var added int
	client := s.pdsClientForUser(r)

	var feedsToFetch []struct{ feedURL, siteURL string }
	for _, fu := range feedURLs {
		f := &db.Feed{
			FeedURL:     fu.URL,
			Title:       db.NullStr(fu.Title),
			SiteURL:     db.NullStr(fu.SiteURL),
			Description: db.NullStr(fu.Description),
		}
		if upsertErr := s.dbs.Articles.UpsertFeed(r.Context(), f); upsertErr != nil {
			s.logger.Error("failed to upsert feed", "error", upsertErr)
			continue
		}

		feedsToFetch = append(feedsToFetch, struct{ feedURL, siteURL string }{fu.URL, fu.SiteURL})

		var subURI, subCID string
		if client != nil {
			record := atproto.SubscriptionRecord{
				CreatedAt: time.Now().Format(time.RFC3339),
				FeedURL:   fu.URL,
				Title:     fu.Title,
				Category:  fu.Category,
			}
			uri, cid, err := client.CreateRecord(r.Context(), user.DID, atproto.CollectionSubscription, record)
			if err != nil {
				s.logger.Error("failed to write subscription to PDS", "error", err, "url", fu.URL)
				continue
			}
			subURI = uri
			subCID = cid
		}

		if subErr := s.dbs.Articles.CreateSubscription(r.Context(), user.DID, fu.URL, fu.Title, fu.Category, subURI, subCID); subErr != nil {
			if !errors.Is(subErr, db.ErrDuplicateSubscription) {
				s.logger.Error("failed to create subscription", "error", subErr)
			}
			continue
		}
		added++
	}

	go func() {
		g, ctx := errgroup.WithContext(context.WithoutCancel(r.Context()))
		g.SetLimit(5)
		for _, f := range feedsToFetch {
			g.Go(func() error {
				result, err := s.fetcher.Fetch(ctx, f.feedURL)
				if err != nil {
					s.logger.Error("failed to fetch feed", "error", err, "feed", f.feedURL)
					_ = s.dbs.Articles.MarkFeedFetchError(ctx, f.feedURL, err.Error())
					return nil
				}
				s.storeFetchResult(ctx, f.feedURL, f.siteURL, result)
				return nil
			})
		}
		if err := g.Wait(); err != nil {
			s.logger.Warn("opml fetch error", "error", err)
		}
	}()

	w.Header().Set("HX-Redirect", "/feeds")
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleOPMLDownload(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	subs, err := s.dbs.Articles.ListSubscriptions(r.Context(), user.DID, "", 1000, 0)
	if err != nil {
		s.logger.Warn("failed to list subscriptions", "error", err, "did", user.DID)
	}

	var feedURLs []feed.FeedURL
	for _, sub := range subs {
		feedURLs = append(feedURLs, feed.FeedURL{
			URL:      sub.FeedURL,
			Title:    sub.FeedTitle,
			Category: sub.Category.String,
		})
	}

	data, err := feed.GenerateOPML(feedURLs, "Glean Subscriptions")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/xml")
	w.Header().Set("Content-Disposition", "attachment; filename=glean-subscriptions.xml")
	_, _ = w.Write(data)
}

func (s *Server) handleFeedList(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	category := r.URL.Query().Get("category")
	subs, err := s.dbs.Articles.ListSubscriptions(r.Context(), user.DID, category, 100, 0)
	if err != nil {
		s.logger.Warn("failed to list subscriptions", "error", err, "did", user.DID)
	}
	s.render(w, r, "feed-list.html", map[string]any{
		"User":          user,
		"Subscriptions": subs,
	})
}

func (s *Server) handleRefreshFeeds(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	ctx := r.Context()

	go s.refreshUserFeeds(context.WithoutCancel(ctx), user.DID)

	category := r.URL.Query().Get("category")
	subs, err := s.dbs.Articles.ListSubscriptions(ctx, user.DID, category, 100, 0)
	if err != nil {
		s.logger.Warn("failed to list subscriptions", "error", err, "did", user.DID)
	}
	s.render(w, r, "feed-list.html", map[string]any{
		"User":          user,
		"Subscriptions": subs,
	})
}

func (s *Server) refreshUserFeeds(ctx context.Context, userDID string) {
	subs, err := s.dbs.Articles.ListSubscriptions(ctx, userDID, "", 100, 0)
	if err != nil {
		s.logger.Warn("failed to list subscriptions for refresh", "error", err, "did", userDID)
		return
	}

	seen := make(map[string]bool)
	for _, sub := range subs {
		if seen[sub.FeedURL] {
			continue
		}
		seen[sub.FeedURL] = true

		f, err := s.dbs.Articles.GetFeed(ctx, sub.FeedURL)
		if err != nil {
			s.logger.Warn("failed to get feed", "error", err, "feed", sub.FeedURL)
			continue
		}
		ff := f.ToFeed()
		s.scheduler.FetchFeed(ctx, ff)
	}
}

func (s *Server) handleRetryFeed(w http.ResponseWriter, r *http.Request) {
	feedURL := r.FormValue("url")
	if feedURL == "" {
		http.Error(w, "url required", http.StatusBadRequest)
		return
	}

	f, err := s.dbs.Articles.GetFeed(r.Context(), feedURL)
	if err != nil {
		http.Error(w, "feed not found", http.StatusNotFound)
		return
	}

	ff := f.ToFeed()
	s.scheduler.FetchFeed(r.Context(), ff)

	user := currentUser(r)
	deadFeeds, err := s.dbs.Articles.ListDeadFeeds(r.Context(), user.DID, 7)
	if err != nil {
		s.logger.Warn("failed to list dead feeds", "error", err, "did", user.DID)
	}
	if len(deadFeeds) == 0 {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(""))
		return
	}

	s.render(w, r, "dead-feeds.html", map[string]any{
		"DeadFeeds": deadFeeds,
	})
}
