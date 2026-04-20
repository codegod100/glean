package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"pkg.rbrt.fr/glean/internal/atproto"
	"pkg.rbrt.fr/glean/internal/db"
	"pkg.rbrt.fr/glean/internal/feed"
)

func (s *Server) handleFeeds(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	category := r.URL.Query().Get("category")

	page := pageFromRequest(r, 50)
	subs, _ := s.db.ListSubscriptions(r.Context(), user.DID, category, page.FetchLimit(), page.Offset)
	page = page.Paginate(len(subs))
	if page.HasMore {
		subs = subs[:page.Limit]
	}

	allSubs, _ := s.db.ListSubscriptions(r.Context(), user.DID, "", 1000, 0)
	feedRecs, _ := s.db.GetFeedRecommendations(r.Context(), user.DID, 10)
	peopleRecs, _ := s.db.GetPeopleRecommendations(r.Context(), user.DID, 5)
	deadFeeds, _ := s.db.ListDeadFeeds(r.Context(), user.DID, 7)

	seen := make(map[string]bool)
	var categories []string
	for _, sub := range allSubs {
		if sub.Category.Valid && sub.Category.String != "" && !seen[sub.Category.String] {
			seen[sub.Category.String] = true
			categories = append(categories, sub.Category.String)
		}
	}

	s.render(w, r, "feeds.html", map[string]any{
		"User":                  user,
		"Subscriptions":         subs,
		"Categories":            categories,
		"Category":              category,
		"FeedRecommendations":   feedRecs,
		"PeopleRecommendations": peopleRecs,
		"DeadFeeds":             deadFeeds,
		"Page":                  page,
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
		s.logger.Error("failed to fetch feed", "error", err, "url", feedURL)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if result != nil {
		f := &db.Feed{
			FeedURL:     feedURL,
			Title:       nullString(result.Feed.Title),
			SiteURL:     nullString(result.Feed.SiteURL),
			Description: nullString(result.Feed.Description),
			FeedType:    nullString(result.Feed.Type),
		}
		if err := s.db.UpsertFeed(r.Context(), f); err != nil {
			s.logger.Error("failed to upsert feed", "error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		siteURL := result.Feed.SiteURL
		if siteURL != "" {
			go func() {
				discResult, err := feed.Discover(context.Background(), siteURL)
				if err == nil && discResult.Favicon != "" {
					_ = s.db.UpdateFeedFavicon(context.Background(), feedURL, discResult.Favicon)
				}
			}()
		}
	}

	var feedTitle string
	if result != nil {
		feedTitle = result.Feed.Title
	}

	var subURI, subCID string
	if client := s.pdsClientForUser(r); client != nil {
		record := atproto.SubscriptionRecord{
			CreatedAt: time.Now().Format(time.RFC3339),
			FeedURL:   feedURL,
			Title:     feedTitle,
			Category:  category,
		}
		uri, cid, err := client.CreateRecord(r.Context(), user.DID, "at.glean.subscription", record)
		if err != nil {
			s.logger.Error("failed to write subscription to PDS", "error", err)
			http.Error(w, "failed to write subscription to PDS: "+err.Error(), http.StatusBadGateway)
			return
		}
		subURI = uri
		subCID = cid
	}

	if err := s.db.CreateSubscription(r.Context(), user.DID, feedURL, category, subURI, subCID); err != nil {
		s.logger.Error("failed to create subscription", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	subs, _ := s.db.ListSubscriptions(r.Context(), user.DID, "", 100, 0)
	s.render(w, r, "feed_list.html", map[string]any{
		"User":          user,
		"Subscriptions": subs,
	})
}

func (s *Server) handleRemoveFeed(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	feedURL := r.FormValue("url")

	if feedURL == "" {
		http.Error(w, "url required", http.StatusBadRequest)
		return
	}

	sub, err := s.db.GetSubscription(r.Context(), user.DID, feedURL)
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

	if err := s.db.DeleteSubscription(r.Context(), user.DID, feedURL); err != nil {
		s.logger.Error("failed to delete subscription", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
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
			FeedURL: fu.URL,
			Title:   nullString(fu.Title),
		}
		if upsertErr := s.db.UpsertFeed(r.Context(), f); upsertErr != nil {
			s.logger.Error("failed to upsert feed", "error", upsertErr)
			continue
		}

		var subURI, subCID string
		if client != nil {
			record := atproto.SubscriptionRecord{
				CreatedAt: time.Now().Format(time.RFC3339),
				FeedURL:   fu.URL,
				Title:     fu.Title,
				Category:  fu.Category,
			}
			uri, cid, err := client.CreateRecord(r.Context(), user.DID, "at.glean.subscription", record)
			if err != nil {
				s.logger.Error("failed to write subscription to PDS", "error", err, "url", fu.URL)
				continue
			}
			subURI = uri
			subCID = cid
		}

		if subErr := s.db.CreateSubscription(r.Context(), user.DID, fu.URL, fu.Category, subURI, subCID); subErr != nil {
			s.logger.Error("failed to create subscription", "error", subErr)
			continue
		}
		added++
	}

	subs, _ := s.db.ListSubscriptions(r.Context(), user.DID, "", 100, 0)
	s.render(w, r, "feeds.html", map[string]any{
		"User":          user,
		"Subscriptions": subs,
		"AddedCount":    added,
	})
}

func (s *Server) handleOPMLDownload(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	subs, _ := s.db.ListSubscriptions(r.Context(), user.DID, "", 1000, 0)

	var feedURLs []feed.FeedURL
	for _, sub := range subs {
		f, err := s.db.GetFeed(r.Context(), sub.FeedURL)
		title := ""
		if err == nil {
			title = f.Title.String
		}
		feedURLs = append(feedURLs, feed.FeedURL{
			URL:   sub.FeedURL,
			Title: title,
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
	subs, _ := s.db.ListSubscriptions(r.Context(), user.DID, "", 100, 0)
	s.render(w, r, "feeds.html", map[string]any{
		"User":          user,
		"Subscriptions": subs,
	})
}

func (s *Server) handleRefreshFeeds(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	store := db.NewFeedStoreAdapter(s.db)
	scheduler := feed.NewScheduler(store, slog.Default())

	subs, _ := s.db.ListSubscriptions(r.Context(), user.DID, "", 100, 0)
	for _, sub := range subs {
		f, err := s.db.GetFeed(r.Context(), sub.FeedURL)
		if err != nil {
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
		scheduler.FetchFeed(r.Context(), ff)
	}

	subs, _ = s.db.ListSubscriptions(r.Context(), user.DID, "", 100, 0)
	s.render(w, r, "feeds.html", map[string]any{
		"User":          user,
		"Subscriptions": subs,
	})
}

func (s *Server) handleDiscoverFeedURL(w http.ResponseWriter, r *http.Request) {
	siteURL := r.URL.Query().Get("url")
	if siteURL == "" {
		http.Error(w, "url required", http.StatusBadRequest)
		return
	}

	result, err := feed.Discover(r.Context(), siteURL)
	if err != nil {
		s.logger.Error("feed discovery failed", "error", err, "url", siteURL)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	type discoveryResponse struct {
		FeedURLs []string `json:"feed_urls"`
		Favicon  string   `json:"favicon"`
	}
	json.NewEncoder(w).Encode(discoveryResponse{
		FeedURLs: result.FeedURLs,
		Favicon:  result.Favicon,
	})
}

func (s *Server) handleUpdateFeedInterval(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	feedURL := r.FormValue("feed_url")
	intervalStr := r.FormValue("interval")

	if feedURL == "" {
		http.Error(w, "feed_url required", http.StatusBadRequest)
		return
	}

	interval, err := strconv.Atoi(intervalStr)
	if err != nil || interval < 5 {
		http.Error(w, "interval must be >= 5 minutes", http.StatusBadRequest)
		return
	}

	if err := s.db.UpdateFeedInterval(r.Context(), feedURL, interval); err != nil {
		s.logger.Error("failed to update feed interval", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("HX-Refresh", "true")
	w.WriteHeader(http.StatusNoContent)
	_ = user
}

func nullString(s string) sql.NullString {
	return sql.NullString{String: s, Valid: s != ""}
}
