package server

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"pkg.rbrt.fr/glean/internal/atproto"
	"pkg.rbrt.fr/glean/internal/db"
	"pkg.rbrt.fr/glean/internal/sanitize"
)

func writeLikeButton(w http.ResponseWriter, articleID int64, liked bool, count int) {
	fill := "none"
	colorCls := "text-spot-muted"
	if liked {
		fill = "currentColor"
		colorCls = "text-spot-red"
	}
	w.Header().Set("Content-Type", "text/html")
	_, _ = fmt.Fprintf(w, `<button hx-post="/articles/%d/like" hx-target="this" hx-swap="outerHTML" class="border border-spot-outline text-spot-text rounded-pill px-4 py-1.5 text-xs font-bold uppercase tracking-button hover:border-spot-text transition inline-flex items-center gap-1.5"><svg class="w-3.5 h-3.5 %s" fill="%s" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" d="M21 8.25c0-2.485-2.099-4.5-4.688-4.5-1.935 0-3.597 1.126-4.312 2.733-.715-1.607-2.377-2.733-4.313-2.733C5.1 3.75 3 5.765 3 8.25c0 7.22 9 12 9 12s9-4.78 9-12z"/></svg><span>%d</span></button>`, articleID, colorCls, fill, count)
}

func (s *Server) handleArticles(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	feedURL := r.URL.Query().Get("feed")
	status := r.URL.Query().Get("status")
	searchQuery := r.URL.Query().Get("q")

	page := pageFromRequest(r, 50)

	var articles []*db.Article
	var err error

	if searchQuery != "" {
		articles, err = s.db.SearchArticles(r.Context(), user.DID, searchQuery, page.Limit()+1, page.Offset())
	} else {
		switch status {
		case "unread":
			articles, err = s.db.ListUnreadArticles(r.Context(), user.DID, feedURL, page.Limit()+1, page.Offset())
		case "read":
			articles, err = s.db.ListReadArticles(r.Context(), user.DID, feedURL, page.Limit()+1, page.Offset())
		default:
			articles, err = s.db.ListArticles(r.Context(), user.DID, feedURL, page.Limit()+1, page.Offset())
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

	data := map[string]any{
		"User":        user,
		"Articles":    articles,
		"FeedURL":     feedURL,
		"Status":      status,
		"SearchQuery": searchQuery,
		"Page":        page,
		"BaseURL":     "/articles",
		"QueryParams": buildQueryParams(map[string]string{"feed": feedURL, "status": status, "q": searchQuery}),
		"Now":         time.Now(),
	}

	if feedURL != "" {
		if feed, err := s.db.GetFeed(r.Context(), feedURL); err == nil {
			data["Feed"] = feed
		}
		unreadCount, _ := s.db.GetUnreadCount(r.Context(), user.DID, feedURL)
		data["FeedUnreadCount"] = unreadCount
	}

	if r.Header.Get("HX-Request") == "true" {
		s.render(w, r, "articles-content.html", data)
		return
	}

	s.render(w, r, "articles.html", data)
}

func (s *Server) handleNewArticleCount(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	sinceUnix, err := strconv.ParseInt(r.URL.Query().Get("since"), 10, 64)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	since := time.Unix(sinceUnix, 0)

	count, err := s.db.CountNewArticles(r.Context(), user.DID, since)
	if err != nil {
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
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	article, err := s.db.GetArticle(r.Context(), id)
	if err != nil {
		http.Error(w, "article not found", http.StatusNotFound)
		return
	}

	_ = s.db.MarkArticleRead(r.Context(), user.DID, id)

	readState, _ := s.db.GetReadState(r.Context(), user.DID, id)

	likeCount, _ := s.db.GetLikeCount(r.Context(), article.FeedURL, article.URL.String)
	liked := false
	if article.URL.Valid {
		liked, _ = s.db.HasLiked(r.Context(), user.DID, article.FeedURL, article.URL.String)
	}
	annotations, _ := s.db.ListAnnotations(r.Context(), "", article.URL.String, "", 20, 0)
	feed, _ := s.db.GetFeed(r.Context(), article.FeedURL)

	s.render(w, r, "article_detail.html", map[string]any{
		"User":           user,
		"CurrentUserDID": user.DID,
		"Article":        article,
		"Feed":           feed,
		"ReadState":      readState,
		"LikeCount":      likeCount,
		"HasLiked":       liked,
		"Annotations":    annotations,
	})
}

func (s *Server) handleMarkRead(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	if err := s.db.MarkArticleRead(r.Context(), user.DID, id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	_, _ = fmt.Fprintf(w, `<span id="read-btn-%d" class="text-xs text-spot-green uppercase tracking-button">Read</span>`, id)
}

func (s *Server) handleMarkUnread(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	if err := s.db.MarkArticleUnread(r.Context(), user.DID, id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	_, _ = fmt.Fprintf(w, `<span id="read-btn-%d" class="text-xs text-spot-muted uppercase tracking-button"></span>`, id)
}

func (s *Server) handleLikeArticle(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	article, err := s.db.GetArticle(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	liked, err := s.db.HasLiked(r.Context(), user.DID, article.FeedURL, article.URL.String)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if liked {
		existingLike, getErr := s.db.GetLike(r.Context(), user.DID, article.FeedURL, article.URL.String)
		if getErr != nil {
			http.Error(w, getErr.Error(), http.StatusInternalServerError)
			return
		}
		if existingLike.URI != "" {
			if client := s.pdsClientForUser(r); client != nil {
				parsed, ok := atproto.ParseRecordURI(existingLike.URI)
				if ok {
					if delErr := client.DeleteRecord(r.Context(), user.DID, parsed.Collection, parsed.RKey); delErr != nil {
						s.logger.Error("failed to delete like from PDS", "error", delErr)
						http.Error(w, "failed to delete like from PDS: "+delErr.Error(), http.StatusBadGateway)
						return
					}
				}
			}
		}
		if err := s.db.DeleteLikeByUserArticle(r.Context(), user.DID, article.FeedURL, article.URL.String); err != nil {
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
			uri, _, err := client.CreateRecord(r.Context(), user.DID, atproto.CollectionLike, likeRecord)
			if err != nil {
				s.logger.Error("failed to write like to PDS", "error", err)
				http.Error(w, "failed to write like to PDS: "+err.Error(), http.StatusBadGateway)
				return
			}

			like := &db.Like{
				URI:        uri,
				AuthorDID:  user.DID,
				FeedURL:    article.FeedURL,
				ArticleURL: article.URL.String,
				CreatedAt:  sql.NullTime{Time: time.Now(), Valid: true},
			}
			if err := s.db.CreateLike(r.Context(), like); err != nil && !errors.Is(err, db.ErrDuplicateLike) {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		} else {
			like := &db.Like{
				URI:        fmt.Sprintf("glean:like:%d", time.Now().UnixNano()),
				AuthorDID:  user.DID,
				FeedURL:    article.FeedURL,
				ArticleURL: article.URL.String,
				CreatedAt:  sql.NullTime{Time: time.Now(), Valid: true},
			}
			if err := s.db.CreateLike(r.Context(), like); err != nil && !errors.Is(err, db.ErrDuplicateLike) {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
	}

	likeCount, _ := s.db.GetLikeCount(r.Context(), article.FeedURL, article.URL.String)
	writeLikeButton(w, id, !liked, likeCount)
}

func (s *Server) handleMarkAllRead(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	feedURL := r.FormValue("feed")
	var err error
	if feedURL != "" {
		err = s.db.MarkAllRead(r.Context(), user.DID, feedURL)
	} else {
		err = s.db.MarkAllSubscribedRead(r.Context(), user.DID)
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("HX-Refresh", "true")
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleFetchContent(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	article, err := s.db.GetArticle(r.Context(), id)
	if err != nil {
		http.Error(w, "article not found", http.StatusNotFound)
		return
	}

	if !article.URL.Valid {
		s.logger.Warn("cannot fetch content: article has no URL", "id", id)
		http.Error(w, "article has no URL", http.StatusBadRequest)
		return
	}

	content, err := s.scraper.Scrape(r.Context(), article.URL.String)
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

	if err := s.db.UpdateArticleFullContent(r.Context(), id, cleaned); err != nil {
		s.logger.Error("failed to save full content", "error", err, "id", id)
	}

	w.Header().Set("Content-Type", "text/html")
	_, _ = fmt.Fprintf(w, `<div id="article-content" class="article-body">%s</div>`, cleaned)
	s.logger.Info("scraped article content", "id", id, "url", article.URL.String, "content_len", len(cleaned))
}
