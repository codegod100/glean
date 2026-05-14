package server

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"golang.org/x/sync/errgroup"

	"pkg.rbrt.fr/glean/internal/atproto"
	"pkg.rbrt.fr/glean/internal/db"
	"pkg.rbrt.fr/glean/internal/sanitize"
)

func writeLikeButton(w http.ResponseWriter, articleID int64, liked bool, count int, bordered bool) {
	fill := "none"
	likedCls := "text-spot-text bg-spot-hover hover:text-spot-red hover:bg-spot-red/15"
	if liked {
		fill = "currentColor"
		likedCls = "text-spot-red bg-spot-red/15 hover:bg-spot-red/25"
	}

	w.Header().Set("Content-Type", "text/html")

	if bordered {
		fmt.Fprintf(w, `<button hx-post="/articles/%d/like?bordered=true" hx-target="this" hx-swap="outerHTML" title="%s" class="group inline-flex items-center gap-1.5 text-[10px] uppercase tracking-button px-2.5 py-1 rounded-pill transition %s"><svg class="w-3.5 h-3.5" fill="%s" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" d="M21 8.25c0-2.485-2.099-4.5-4.688-4.5-1.935 0-3.597 1.126-4.312 2.733-.715-1.607-2.377-2.733-4.313-2.733C5.1 3.75 3 5.765 3 8.25c0 7.22 9 12 9 12s9-4.78 9-12z"/></svg><span>%d</span></button>`, articleID, likeTitle(liked), likedCls, fill, count)
	} else {
		fmt.Fprintf(w, `<button hx-post="/articles/%d/like" hx-target="this" hx-swap="outerHTML" title="%s" class="group inline-flex items-center gap-1 text-[10px] uppercase tracking-button px-2 py-0.5 rounded-pill transition %s"><svg class="w-3 h-3" fill="%s" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" d="M21 8.25c0-2.485-2.099-4.5-4.688-4.5-1.935 0-3.597 1.126-4.312 2.733-.715-1.607-2.377-2.733-4.313-2.733C5.1 3.75 3 5.765 3 8.25c0 7.22 9 12 9 12s9-4.78 9-12z"/></svg><span>%d</span></button>`, articleID, likeTitle(liked), likedCls, fill, count)
	}
}

func likeTitle(liked bool) string {
	if liked {
		return "Unlike"
	}
	return "Like"
}

func (s *Server) handleArticles(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	ctx := r.Context()
	feedURL := r.URL.Query().Get("feed")
	status := r.URL.Query().Get("status")
	searchQuery := r.URL.Query().Get("q")

	page := pageFromRequest(r, 50)

	if status == "" && searchQuery == "" {
		unreadCount, err := s.dbs.Articles.GetUnreadCount(ctx, user.DID, feedURL)
		if err != nil {
			s.logger.Warn("failed to get unread count", "error", err, "did", user.DID)
		}
		if unreadCount > 0 {
			status = "unread"
		} else {
			status = "all"
		}
	}

	var articles []*db.Article
	var err error

	if searchQuery != "" {
		articles, err = s.dbs.Articles.SearchArticles(ctx, user.DID, searchQuery, page.Limit()+1, page.Offset())
	} else {
		switch status {
		case "unread":
			articles, err = s.dbs.Articles.ListUnreadArticles(ctx, user.DID, feedURL, page.Limit()+1, page.Offset())
		case "read":
			articles, err = s.dbs.Articles.ListReadArticles(ctx, user.DID, feedURL, page.Limit()+1, page.Offset())
		default:
			articles, err = s.dbs.Articles.ListArticles(ctx, user.DID, feedURL, page.Limit()+1, page.Offset())
		}
	}

	if err != nil {
		s.logger.Error("failed to list articles", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	totalFetched := len(articles)
	page = page.Paginate(totalFetched)
	if page.HasNext {
		articles = articles[:page.PageSize]
	}

	navSuffix := buildNavSuffix(feedURL, false)
	for _, a := range articles {
		a.NavSuffix = navSuffix
	}

	settings, _ := s.dbs.Users.GetSettings(ctx, user.DID)
	expandedView := false
	if settings != nil {
		expandedView = settings.ExpandedView
	}

	data := map[string]any{
		"User":         user,
		"Articles":     articles,
		"FeedURL":      feedURL,
		"Status":       status,
		"SearchQuery":  searchQuery,
		"Page":         page,
		"BaseURL":      "/articles",
		"QueryParams":  buildQueryParams(map[string]string{"feed": feedURL, "status": status, "q": searchQuery}),
		"Now":          time.Now(),
		"ExpandedView": expandedView,
	}

	if feedURL != "" {
		if feed, err := s.dbs.Articles.GetFeed(ctx, feedURL); err == nil {
			data["Feed"] = feed
		} else {
			s.logger.Warn("failed to get feed", "error", err, "feed", feedURL)
		}
		if _, err := s.dbs.Articles.GetSubscription(ctx, user.DID, feedURL); err == nil {
			data["IsSubscribed"] = true
		} else {
			data["IsSubscribed"] = false
		}
	}

	if r.Header.Get("HX-Request") == htmxRequestHeader {
		s.render(w, r, "articles-content.html", data)
		return
	}

	s.render(w, r, "articles.html", data)
}

func (s *Server) handleNewArticleCount(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	ctx := r.Context()
	sinceUnix, err := strconv.ParseInt(r.URL.Query().Get("since"), 10, 64)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	since := time.Unix(sinceUnix, 0)

	count, err := s.dbs.Articles.CountNewArticles(ctx, user.DID, since)
	if err != nil {
		s.logger.Error("failed to count new articles", "error", err, "did", user.DID)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	if count == 0 {
		w.Write([]byte(""))
		return
	}
	fmt.Fprintf(w, `<div id="new-articles-banner" class="bg-spot-green rounded-xl px-5 py-3 flex items-center justify-between mb-4"><span class="text-sm text-white font-medium">%d new article%s available.</span><a href="%s" class="text-sm font-bold text-white uppercase tracking-button hover:underline transition">Refresh</a></div>`, count, pluralS(count), r.URL.Query().Get("return"))
}

func pluralS(n int) string {
	if n != 1 {
		return "s"
	}
	return ""
}

func (s *Server) handleArticleDetail(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	ctx := r.Context()
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	article, err := s.dbs.Articles.GetArticle(ctx, id)
	if err != nil {
		http.Error(w, "article not found", http.StatusNotFound)
		return
	}

	var (
		readState   *db.ReadState
		likeCount   int
		liked       bool
		annotations []*db.Annotation
		feed        *db.Feed
		nextID      *int64
	)

	fromFeedURL := r.URL.Query().Get("from_feed")
	navLiked := r.URL.Query().Get("liked") == "1"

	g, gCtx := errgroup.WithContext(ctx)

	g.Go(func() error {
		if err := s.dbs.Articles.MarkArticleRead(gCtx, user.DID, id); err != nil {
			s.logger.Warn("failed to mark article read", "error", err, "id", id)
		}
		return nil
	})

	g.Go(func() error {
		var err error
		readState, err = s.dbs.Articles.GetReadState(gCtx, user.DID, id)
		if err != nil {
			s.logger.Warn("failed to get read state", "error", err, "id", id)
		}
		return nil
	})

	g.Go(func() error {
		if article.URL.Valid {
			var err error
			likeCount, err = s.dbs.Articles.GetLikeCount(gCtx, article.FeedURL, article.URL.String)
			if err != nil {
				s.logger.Warn("failed to get like count", "error", err, "feed", article.FeedURL)
			}
		}
		return nil
	})

	g.Go(func() error {
		if article.URL.Valid {
			var err error
			liked, err = s.dbs.Articles.HasLiked(gCtx, user.DID, article.FeedURL, article.URL.String)
			if err != nil {
				s.logger.Warn("failed to check if liked", "error", err)
			}
		}
		return nil
	})

	g.Go(func() error {
		var err error
		annotations, err = s.dbs.Articles.ListAnnotations(gCtx, "", article.URL.String, "", 20, 0)
		if err != nil {
			s.logger.Warn("failed to list annotations", "error", err)
			return nil
		}
		resolveAnnotationHandles(gCtx, annotations)
		return nil
	})

	g.Go(func() error {
		var err error
		feed, err = s.dbs.Articles.GetFeed(gCtx, article.FeedURL)
		if err != nil {
			s.logger.Warn("failed to get feed", "error", err, "feed", article.FeedURL)
		}
		return nil
	})

	g.Go(func() error {
		var err error
		nextID, err = s.dbs.Articles.GetNextArticleID(gCtx, user.DID, id, fromFeedURL, navLiked)
		if err != nil {
			s.logger.Warn("failed to get next article", "error", err, "id", id)
		}
		return nil
	})

	if err := g.Wait(); err != nil {
		s.logger.Warn("article detail error", "error", err, "id", id)
	}

	s.render(w, r, "article_detail.html", map[string]any{
		"User":           user,
		"CurrentUserDID": user.DID,
		"Article":        article,
		"Feed":           feed,
		"ReadState":      readState,
		"LikeCount":      likeCount,
		"HasLiked":       liked,
		"Annotations":    annotations,
		"NextID":         nextID,
		"NextSuffix":     buildNavSuffix(fromFeedURL, navLiked),
	})
}

func (s *Server) handleMarkRead(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	if err := s.dbs.Articles.MarkArticleRead(r.Context(), user.DID, id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w, `<button id="read-btn-%[1]d" hx-post="/articles/%[1]d/unread" hx-target="#read-btn-%[1]d" hx-swap="outerHTML" title="Mark as Unread" class="group inline-flex items-center gap-1 text-[10px] text-spot-secondary uppercase tracking-button px-2 py-0.5 rounded-pill bg-spot-hover hover:text-spot-green hover:bg-spot-green/15 transition"><svg class="w-3 h-3" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" d="M4.5 12.75l6 6 9-13.5"/></svg><span>Unread</span></button>`, id)
}

func (s *Server) handleMarkUnread(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	if err := s.dbs.Articles.MarkArticleUnread(r.Context(), user.DID, id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w, `<button id="read-btn-%[1]d" hx-post="/articles/%[1]d/read" hx-target="#read-btn-%[1]d" hx-swap="outerHTML" title="Mark as Read" class="group inline-flex items-center gap-1 text-[10px] text-spot-text uppercase tracking-button px-2 py-0.5 rounded-pill bg-spot-hover hover:text-spot-green hover:bg-spot-green/15 transition"><svg class="w-3 h-3" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" d="M4.5 12.75l6 6 9-13.5"/></svg><span>Read</span></button>`, id)
}

func (s *Server) handleLikeArticle(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	ctx := r.Context()
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	article, err := s.dbs.Articles.GetArticle(ctx, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	liked, err := s.dbs.Articles.HasLiked(ctx, user.DID, article.FeedURL, article.URL.String)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if liked {
		existingLike, getErr := s.dbs.Articles.GetLike(ctx, user.DID, article.FeedURL, article.URL.String)
		if getErr != nil {
			http.Error(w, getErr.Error(), http.StatusInternalServerError)
			return
		}
		if existingLike.URI != "" {
			if client := s.pdsClientForUser(r); client != nil {
				parsed, ok := atproto.ParseRecordURI(existingLike.URI)
				if ok {
					if delErr := client.DeleteRecord(ctx, user.DID, parsed.Collection, parsed.RKey); delErr != nil {
						s.logger.Error("failed to delete like from PDS", "error", delErr)
						http.Error(w, "failed to delete like from PDS: "+delErr.Error(), http.StatusInternalServerError)
						return
					}
				}
			}
		}
		if err := s.dbs.Articles.DeleteLikeByUserArticle(ctx, user.DID, article.FeedURL, article.URL.String); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	} else {
		likeRecord := atproto.LikeRecord{
			CreatedAt:  time.Now().Format(time.RFC3339),
			FeedURL:    article.FeedURL,
			ArticleURL: article.URL.String,
		}

		if client := s.pdsClientForUser(r); client != nil {
			uri, _, err := client.CreateRecord(ctx, user.DID, atproto.CollectionLike, likeRecord)
			if err != nil {
				s.logger.Error("failed to write like to PDS", "error", err)
				http.Error(w, "failed to write like to PDS: "+err.Error(), http.StatusInternalServerError)
				return
			}

			like := &db.Like{
				URI:        uri,
				AuthorDID:  user.DID,
				FeedURL:    article.FeedURL,
				ArticleURL: article.URL.String,
				CreatedAt:  sql.NullTime{Time: time.Now(), Valid: true},
			}
			if err := s.dbs.Articles.CreateLike(ctx, like); err != nil && !errors.Is(err, db.ErrDuplicateLike) {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if err := s.feedback.MarkImpressionActed(ctx, user.DID, "article", article.URL.String); err != nil {
				s.logger.Warn("failed to mark impression acted", "error", err)
			}
			sig := s.engine.GetDominantSignal(s.engine.GetWeights(ctx, user.DID))
			s.engine.RewardSignal(ctx, user.DID, sig)
		} else {
			like := &db.Like{
				URI:        fmt.Sprintf("glean:like:%d", time.Now().UnixNano()),
				AuthorDID:  user.DID,
				FeedURL:    article.FeedURL,
				ArticleURL: article.URL.String,
				CreatedAt:  sql.NullTime{Time: time.Now(), Valid: true},
			}
			if err := s.dbs.Articles.CreateLike(ctx, like); err != nil && !errors.Is(err, db.ErrDuplicateLike) {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if err := s.feedback.MarkImpressionActed(ctx, user.DID, "article", article.URL.String); err != nil {
				s.logger.Warn("failed to mark impression acted", "error", err)
			}
			sig := s.engine.GetDominantSignal(s.engine.GetWeights(ctx, user.DID))
			s.engine.RewardSignal(ctx, user.DID, sig)
		}
	}

	likeCount := 0
	if article.URL.Valid {
		likeCount, err = s.dbs.Articles.GetLikeCount(ctx, article.FeedURL, article.URL.String)
		if err != nil {
			s.logger.Warn("failed to get like count", "error", err)
		}
	}
	bordered := r.URL.Query().Get("bordered") == "true"
	writeLikeButton(w, id, !liked, likeCount, bordered)
}

func (s *Server) handleMarkAllRead(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	ctx := r.Context()
	feedURL := r.FormValue("feed")
	var err error
	if feedURL != "" {
		err = s.dbs.Articles.MarkAllRead(ctx, user.DID, feedURL)
	} else {
		err = s.dbs.Articles.MarkAllSubscribedRead(ctx, user.DID)
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("HX-Refresh", "true")
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleFetchContent(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	article, err := s.dbs.Articles.GetArticle(ctx, id)
	if err != nil {
		http.Error(w, "article not found", http.StatusNotFound)
		return
	}

	if !article.URL.Valid {
		s.logger.Warn("cannot fetch content: article has no URL", "id", id)
		http.Error(w, "article has no URL", http.StatusBadRequest)
		return
	}

	content, err := s.scraper.Scrape(ctx, article.URL.String)
	if err != nil {
		s.logger.Error("failed to scrape article", "error", err, "url", article.URL.String)
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprintf(w, `<div id="article-content" class="text-spot-secondary text-sm">Failed to fetch content. <button hx-post="/articles/%d/fetch-content" hx-target="#article-content" hx-swap="outerHTML" class="text-spot-green underline">Retry</button></div>`, id)
		return
	}

	if content == "" {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprintf(w, `<div id="article-content" class="text-spot-secondary text-sm">No readable content found. <a href="%s" target="_blank" rel="noopener noreferrer" class="text-spot-green underline">Read on original site</a></div>`, article.URL.String)
		return
	}

	cleaned := sanitize.HTML(content)

	if err := s.dbs.Articles.UpdateArticleFullContent(ctx, id, cleaned); err != nil {
		s.logger.Error("failed to save full content", "error", err, "id", id)
	}

	w.Header().Set("Content-Type", "text/html")
	_, _ = fmt.Fprintf(w, `<div id="article-content" class="article-body">%s</div>`, cleaned)
	s.logger.Info("scraped article content", "id", id, "url", article.URL.String, "content_len", len(cleaned))
}

func buildNavSuffix(feedURL string, liked bool) string {
	v := url.Values{}
	if feedURL != "" {
		v.Set("from_feed", feedURL)
	}
	if liked {
		v.Set("liked", "1")
	}
	if len(v) == 0 {
		return ""
	}
	return "?" + v.Encode()
}
