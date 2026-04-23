package server

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"time"

	"pkg.rbrt.fr/glean/internal/atproto"
	"pkg.rbrt.fr/glean/internal/cluster"
	"pkg.rbrt.fr/glean/internal/db"
	"pkg.rbrt.fr/glean/internal/feed"
)

func (s *Server) handleFeeds(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	category := r.URL.Query().Get("category")
	ctx := r.Context()

	page := pageFromRequest(r, 50)
	subs, err := s.dbs.Articles.ListSubscriptions(ctx, user.DID, category, page.Limit()+1, page.Offset())
	if err != nil {
		s.logger.Warn("failed to list subscriptions", "error", err, "did", user.DID)
	}
	totalFetched := len(subs)
	page = page.Paginate(totalFetched)
	if page.HasNext {
		subs = subs[:page.PageSize]
	}

	allSubs, err := s.dbs.Articles.ListSubscriptions(ctx, user.DID, "", 1000, 0)
	if err != nil {
		s.logger.Warn("failed to list all subscriptions", "error", err, "did", user.DID)
	}

	feedRecs, err := s.engine.GetFeedRecommendations(ctx, user.DID, 6)
	if err != nil {
		s.logger.Warn("failed to get feed recommendations", "error", err, "did", user.DID)
	}

	peopleRecs, err := s.engine.GetPeopleRecommendations(ctx, user.DID, 5)
	if err != nil {
		s.logger.Warn("failed to get people recommendations", "error", err, "did", user.DID)
	}
	resolvePeopleHandles(ctx, peopleRecs)

	if len(feedRecs) > 0 {
		impressions := make([]cluster.Impression, len(feedRecs))
		for i, rec := range feedRecs {
			impressions[i] = cluster.Impression{TargetType: "feed", TargetID: rec.FeedURL}
		}
		if err := s.engine.RecordImpressions(ctx, user.DID, impressions); err != nil {
			s.logger.Warn("failed to record impressions", "error", err)
		}
	}

	deadFeeds, err := s.dbs.Articles.ListDeadFeeds(ctx, user.DID, 7)
	if err != nil {
		s.logger.Warn("failed to list dead feeds", "error", err, "did", user.DID)
	}

	categories, err := s.dbs.Articles.GetCategories(ctx, user.DID)
	if err != nil {
		s.logger.Warn("failed to get categories", "error", err, "did", user.DID)
	}

	s.render(w, r, "feeds.html", map[string]any{
		"User":                  user,
		"Subscriptions":         subs,
		"SubscriptionCount":     len(allSubs),
		"Categories":            categories,
		"Category":              category,
		"FeedRecommendations":   feedRecs,
		"PeopleRecommendations": peopleRecs,
		"DeadFeeds":             deadFeeds,
		"Page":                  page,
		"BaseURL":               "/feeds",
		"QueryParams":           buildQueryParams(map[string]string{"category": category}),
	})
}

func (s *Server) handleAddFeed(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	feedURL := r.FormValue("feed_url")
	category := r.FormValue("category")

	if feedURL == "" {
		http.Error(w, "url required", http.StatusBadRequest)
		return
	}

	result, _, _, err := s.fetcher.Fetch(r.Context(), feedURL, "", "")
	if err != nil {
		result, feedURL, err = s.discoverFeed(r.Context(), feedURL)
	}
	if err != nil {
		s.logger.Error("failed to fetch feed", "error", err, "url", feedURL)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var feedTitle string
	var faviconURL string
	if result != nil {
		feedTitle = result.Feed.Title
		faviconURL = result.Feed.FaviconURL
		if faviconURL == "" {
			go func() {
				if f := feed.ResolveFavicon(context.Background(), feedURL, result.Feed.SiteURL); f != "" {
					_ = s.dbs.Articles.UpdateFeedFavicon(context.Background(), feedURL, f)
				}
			}()
		}
	}

	f := &db.Feed{
		FeedURL:     feedURL,
		Title:       nullString(feedTitle),
		SiteURL:     nullString(result.Feed.SiteURL),
		Description: nullString(result.Feed.Description),
		FeedType:    nullString(result.Feed.Type),
		FaviconURL:  nullString(faviconURL),
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
			http.Error(w, "failed to write subscription to PDS: "+err.Error(), http.StatusBadGateway)
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

	if err := s.engine.MarkImpressionActed(r.Context(), user.DID, "feed", feedURL); err != nil {
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
		"FeedURL":     sub.FeedURL,
		"FeedTitle":   sub.FeedTitle,
		"Category":    sub.Category,
		"FaviconURL":  sub.FaviconURL,
		"UnreadCount": sub.UnreadCount,
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
					http.Error(w, "failed to delete subscription from PDS: "+delErr.Error(), http.StatusBadGateway)
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
	for _, fu := range feedURLs {
		f := &db.Feed{
			FeedURL:     fu.URL,
			Title:       nullString(fu.Title),
			SiteURL:     nullString(fu.SiteURL),
			Description: nullString(fu.Description),
		}
		if upsertErr := s.dbs.Articles.UpsertFeed(r.Context(), f); upsertErr != nil {
			s.logger.Error("failed to upsert feed", "error", upsertErr)
			continue
		}

		go func(feedURL, siteURL string) {
			if fav := feed.ResolveFavicon(context.Background(), feedURL, siteURL); fav != "" {
				if err := s.dbs.Articles.UpdateFeedFavicon(context.Background(), feedURL, fav); err != nil {
					s.logger.Warn("failed to update favicon", "error", err, "feed", feedURL)
				}
			}
		}(fu.URL, fu.SiteURL)

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
		ff := &feed.Feed{
			URL:          f.FeedURL,
			Title:        f.Title.String,
			SiteURL:      f.SiteURL.String,
			Description:  f.Description.String,
			Type:         f.FeedType.String,
			ETag:         f.Etag.String,
			LastModified: f.LastModified.String,
		}
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

	ff := &feed.Feed{
		URL:          f.FeedURL,
		Title:        f.Title.String,
		SiteURL:      f.SiteURL.String,
		Description:  f.Description.String,
		Type:         f.FeedType.String,
		ETag:         f.Etag.String,
		LastModified: f.LastModified.String,
	}
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

func (s *Server) handleDismissFeedRecommendation(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	feedURL := r.FormValue("feed_url")
	if feedURL == "" {
		http.Error(w, "feed_url required", http.StatusBadRequest)
		return
	}

	reason := r.FormValue("reason")
	if reason == "" {
		reason = "not_interested"
	}

	if err := s.engine.DismissFeed(r.Context(), user.DID, feedURL, reason); err != nil {
		s.logger.Error("failed to dismiss feed recommendation", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (s *Server) discoverFeed(ctx context.Context, feedURL string) (*feed.ParseResult, string, error) {
	discovered, err := feed.Discover(ctx, feedURL)
	if err != nil || len(discovered.FeedURLs) == 0 {
		return nil, feedURL, fmt.Errorf("no feeds found at %s", feedURL)
	}

	for _, candidate := range discovered.FeedURLs {
		result, _, _, fetchErr := s.fetcher.Fetch(ctx, candidate, "", "")
		if fetchErr == nil && result != nil {
			return result, candidate, nil
		}
	}

	return nil, feedURL, fmt.Errorf("no feeds found at %s", feedURL)
}

func nullString(s string) sql.NullString {
	return sql.NullString{String: s, Valid: s != ""}
}
