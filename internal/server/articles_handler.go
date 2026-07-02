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
)

func (s *Server) handleArticles(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	ctx := r.Context()
	feedURL := r.URL.Query().Get("feed")
	status := r.URL.Query().Get("status")
	searchQuery := r.URL.Query().Get("q")
	sortOldest := r.URL.Query().Get("sort") == "oldest"
	category := r.URL.Query().Get("category")

	page := pageFromRequest(r, 50)

	if status == "" && searchQuery == "" {
		unreadCount, err := s.dbs.Articles.GetUnreadCount(ctx, user.DID, feedURL, category)
		if err != nil {
			s.logger.Warn("failed to get unread count", "error", err, "did", user.DID)
		}
		if unreadCount > 0 {
			status = "unread"
		} else {
			status = "all"
		}
	}

	var (
		articles []*db.Article
		err      error
	)

	if searchQuery != "" {
		articles, err = s.dbs.Articles.SearchArticles(ctx, user.DID, searchQuery, page.Limit()+1, page.Offset())
	} else {
		switch status {
		case "unread":
			articles, err = s.dbs.Articles.ListUnreadArticles(ctx, user.DID, feedURL, category, page.Limit()+1, page.Offset(), sortOldest)
		case "read":
			articles, err = s.dbs.Articles.ListReadArticles(ctx, user.DID, feedURL, category, page.Limit()+1, page.Offset(), sortOldest)
		default:
			articles, err = s.dbs.Articles.ListArticles(ctx, user.DID, feedURL, category, page.Limit()+1, page.Offset(), sortOldest)
		}
	}

	if err != nil {
		s.logger.Error("failed to list articles", "error", err)
		writeAPIError(w, http.StatusInternalServerError, err.Error())
		return
	}

	totalFetched := len(articles)
	page = page.Paginate(totalFetched)
	if page.HasNext {
		articles = articles[:page.PageSize]
	}

	settings, _ := s.dbs.Users.GetSettings(ctx, user.DID)
	expandedView := settings != nil && settings.ExpandedView

	categories, _ := s.dbs.Articles.GetCategories(ctx, user.DID)
	categories = nonNil(categories)

	out := make([]Article, len(articles))
	for i, a := range articles {
		out[i] = toArticle(a)
	}

	resp := articlesResponse{
		User:         toUser(user),
		Articles:     out,
		FeedURL:      feedURL,
		Status:       status,
		SearchQuery:  searchQuery,
		SortOldest:   sortOldest,
		Category:     category,
		Categories:   categories,
		ExpandedView: expandedView,
		Pagination:   page,
		Now:          time.Now().Unix(),
	}

	if feedURL != "" {
		if feed, err := s.dbs.Articles.GetFeed(ctx, feedURL); err == nil {
			resp.Feed = new(toFeed(feed))
		}
		_, subErr := s.dbs.Articles.GetSubscription(ctx, user.DID, feedURL)
		resp.IsSubscribed = subErr == nil
	}

	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleNewArticleCount(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	ctx := r.Context()
	sinceUnix, err := strconv.ParseInt(r.URL.Query().Get("since"), 10, 64)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid since")
		return
	}
	since := time.Unix(sinceUnix, 0)

	count, err := s.dbs.Articles.CountNewArticles(ctx, user.DID, since)
	if err != nil {
		s.logger.Error("failed to count new articles", "error", err, "did", user.DID)
		writeAPIError(w, http.StatusInternalServerError, "failed to count")
		return
	}

	writeJSON(w, http.StatusOK, newArticleCountResponse{Count: count})
}

func (s *Server) handleArticleDetail(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	ctx := r.Context()
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid id")
		return
	}

	article, err := s.dbs.Articles.GetArticle(ctx, id)
	if err != nil {
		writeAPIError(w, http.StatusNotFound, "article not found")
		return
	}

	fromFeedURL := r.URL.Query().Get("from_feed")
	navLiked := r.URL.Query().Get("liked") == "1"
	navStatus := r.URL.Query().Get("status")

	var (
		likeCount   int
		liked       bool
		annotations []*db.Annotation
		feed        *db.Feed
		nextID      *int64
	)

	g, gCtx := errgroup.WithContext(ctx)

	g.Go(func() error {
		if err := s.dbs.Articles.MarkArticleRead(gCtx, user.DID, id); err != nil {
			s.logger.Warn("failed to mark article read", "error", err, "id", id)
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
		annotations, err = s.dbs.Articles.ListAnnotations(gCtx, article.FeedURL, article.URL.String, "", 20, 0)
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
		nextID, err = s.dbs.Articles.GetNextArticleID(gCtx, user.DID, id, fromFeedURL, navLiked, navStatus)
		if err != nil {
			s.logger.Warn("failed to get next article", "error", err, "id", id)
		}
		return nil
	})

	_ = g.Wait()

	dto := toArticle(article)
	dto.IsRead = true
	dto.LikeCount = likeCount
	dto.HasLiked = liked

	annots := make([]Annotation, len(annotations))
	for i, a := range annotations {
		annots[i] = toAnnotation(a)
	}

	resp := articleDetailResponse{
		User:           toUser(user),
		CurrentUserDID: user.DID,
		Article:        dto,
		Feed:           toFeed(feed),
		Annotations:    annots,
		NextID:         nextID,
		NextSuffix:     buildNavSuffix(fromFeedURL, navLiked, navStatus),
	}

	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleMarkRead(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := s.dbs.Articles.MarkArticleRead(r.Context(), user.DID, id); err != nil {
		writeAPIError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, articleStateResponse{ID: id, IsRead: true})
}

func (s *Server) handleMarkUnread(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := s.dbs.Articles.MarkArticleUnread(r.Context(), user.DID, id); err != nil {
		writeAPIError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, articleStateResponse{ID: id, IsRead: false})
}

func (s *Server) handleLikeArticle(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	ctx := r.Context()
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid id")
		return
	}

	article, err := s.dbs.Articles.GetArticle(ctx, id)
	if err != nil {
		writeAPIError(w, http.StatusNotFound, err.Error())
		return
	}

	liked, err := s.dbs.Articles.HasLiked(ctx, user.DID, article.FeedURL, article.URL.String)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if liked {
		existingLike, getErr := s.dbs.Articles.GetLike(ctx, user.DID, article.FeedURL, article.URL.String)
		if getErr != nil {
			writeAPIError(w, http.StatusInternalServerError, getErr.Error())
			return
		}
		if existingLike.URI != "" {
			if client := s.pdsClientForUser(r); client != nil {
				parsed, ok := atproto.ParseRecordURI(existingLike.URI)
				if ok {
					if delErr := client.DeleteRecord(ctx, user.DID, parsed.Collection, parsed.RKey); delErr != nil {
						s.logger.Error("failed to delete like from PDS", "error", delErr)
						writeAPIError(w, http.StatusInternalServerError, "failed to delete like from PDS: "+delErr.Error())
						return
					}
				}
			}
		}
		if err := s.dbs.Articles.DeleteLikeByUserArticle(ctx, user.DID, article.FeedURL, article.URL.String); err != nil {
			writeAPIError(w, http.StatusInternalServerError, err.Error())
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
				writeAPIError(w, http.StatusInternalServerError, "failed to write like to PDS: "+err.Error())
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
				writeAPIError(w, http.StatusInternalServerError, err.Error())
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
			if err := s.dbs.Articles.CreateLike(ctx, like); err != nil && !errors.Is(err, db.ErrDuplicateLike) {
				writeAPIError(w, http.StatusInternalServerError, err.Error())
				return
			}
		}
		if err := s.feedback.MarkImpressionActed(ctx, user.DID, "article", article.URL.String); err != nil {
			s.logger.Warn("failed to mark impression acted", "error", err)
		}
		sig := s.engine.GetDominantSignal(s.engine.GetWeights(ctx, user.DID))
		s.engine.RewardSignal(ctx, user.DID, sig)
	}

	likeCount := 0
	if article.URL.Valid {
		likeCount, err = s.dbs.Articles.GetLikeCount(ctx, article.FeedURL, article.URL.String)
		if err != nil {
			s.logger.Warn("failed to get like count", "error", err)
		}
	}

	writeJSON(w, http.StatusOK, likeResponse{
		ID:        id,
		Liked:     !liked,
		LikeCount: likeCount,
	})
}

func (s *Server) handleMarkAllRead(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	ctx := r.Context()
	if err := r.ParseForm(); err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}
	feedURL := r.FormValue("feed")
	var err error
	if feedURL != "" {
		err = s.dbs.Articles.MarkAllRead(ctx, user.DID, feedURL)
	} else {
		err = s.dbs.Articles.MarkAllSubscribedRead(ctx, user.DID)
	}
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleFetchContent(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid id")
		return
	}

	article, err := s.dbs.Articles.GetArticle(ctx, id)
	if err != nil {
		writeAPIError(w, http.StatusNotFound, "article not found")
		return
	}

	if !article.URL.Valid {
		writeAPIError(w, http.StatusBadRequest, "article has no URL")
		return
	}

	content, err := s.scraper.Scrape(ctx, article.URL.String)
	if err != nil {
		s.logger.Error("failed to scrape article", "error", err, "url", article.URL.String)
		writeAPIError(w, http.StatusBadGateway, "failed to fetch content")
		return
	}

	cleaned := sanitizeHTML(content)
	if cleaned != "" {
		if err := s.dbs.Articles.UpdateArticleFullContent(ctx, id, cleaned); err != nil {
			s.logger.Error("failed to save full content", "error", err, "id", id)
		}
	}

	writeJSON(w, http.StatusOK, fetchContentResponse{
		ID:          id,
		FullContent: cleaned,
	})
}

func buildNavSuffix(feedURL string, liked bool, status string) string {
	v := url.Values{}
	if feedURL != "" {
		v.Set("from_feed", feedURL)
	}
	if liked {
		v.Set("liked", "1")
	}
	if status != "" {
		v.Set("status", status)
	}
	if len(v) == 0 {
		return ""
	}
	return "?" + v.Encode()
}
