package server

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"pkg.rbrt.fr/glean/internal/atproto"
	"pkg.rbrt.fr/glean/internal/db"
)

func writeStarButton(w http.ResponseWriter, articleID int64, starred bool) {
	cls := "text-gray-300 hover:text-yellow-500"
	ch := "&#9734;"
	if starred {
		cls = "text-yellow-500"
		ch = "&#9733;"
	}
	w.Header().Set("Content-Type", "text/html")
	_, _ = fmt.Fprintf(w, `<button hx-post="/articles/%d/star" hx-target="#star-btn" hx-swap="outerHTML" id="star-btn" class="text-lg %s">%s</button>`, articleID, cls, ch)
}

func writeLikeButton(w http.ResponseWriter, articleID int64, liked bool, count int) {
	cls := "text-gray-300 hover:text-red-500"
	if liked {
		cls = "text-red-500"
	}
	w.Header().Set("Content-Type", "text/html")
	_, _ = fmt.Fprintf(w, `<button hx-post="/articles/%d/like" hx-target="#like-btn" hx-swap="outerHTML" id="like-btn" class="text-lg %s">&#9829; <span class="text-sm text-gray-600">%d</span></button>`, articleID, cls, count)
}

func writeReadButton(w http.ResponseWriter, articleID int64, isRead bool) {
	label := "Mark read"
	action := "read"
	if isRead {
		label = "Mark unread"
		action = "unread"
	}
	w.Header().Set("Content-Type", "text/html")
	_, _ = fmt.Fprintf(w, `<button hx-post="/articles/%d/%s" hx-target="#read-btn" hx-swap="outerHTML" id="read-btn" class="text-xs border border-gray-300 rounded px-2 py-1 hover:bg-gray-50">%s</button>`, articleID, action, label)
}

func (s *Server) handleArticles(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	feedURL := r.URL.Query().Get("feed")

	articles, err := s.db.ListArticles(r.Context(), user.DID, feedURL, 50, 0)
	if err != nil {
		s.logger.Error("failed to list articles", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.render(w, r, "articles.html", map[string]any{
		"User":     user,
		"Articles": articles,
		"FeedURL":  feedURL,
	})
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
		"User":        user,
		"Article":     article,
		"Feed":        feed,
		"ReadState":   readState,
		"LikeCount":   likeCount,
		"HasLiked":    liked,
		"Annotations": annotations,
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
	writeReadButton(w, id, true)
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
	writeReadButton(w, id, false)
}

func (s *Server) handleStar(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	rs, _ := s.db.GetReadState(r.Context(), user.DID, id)
	if rs.IsStarred {
		if err := s.db.UnstarArticle(r.Context(), user.DID, id); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeStarButton(w, id, false)
	} else {
		if err := s.db.StarArticle(r.Context(), user.DID, id); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeStarButton(w, id, true)
	}
}

func (s *Server) handleUnstar(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	if err := s.db.UnstarArticle(r.Context(), user.DID, id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeStarButton(w, id, false)
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
			uri, _, err := client.CreateRecord(r.Context(), user.DID, "at.glean.like", likeRecord)
			if err != nil {
				s.logger.Warn("failed to write like to PDS", "error", err)
			}

			like := &db.Like{
				URI:        uri,
				AuthorDID:  user.DID,
				FeedURL:    article.FeedURL,
				ArticleURL: article.URL.String,
				CreatedAt:  sql.NullTime{Time: time.Now(), Valid: true},
			}
			if err := s.db.CreateLike(r.Context(), like); err != nil {
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
			if err := s.db.CreateLike(r.Context(), like); err != nil {
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
