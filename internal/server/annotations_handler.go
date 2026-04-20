package server

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"pkg.rbrt.fr/glean/internal/atproto"
	"pkg.rbrt.fr/glean/internal/db"
)

func (s *Server) handleLibrary(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)

	likedOffset, _ := strconv.Atoi(r.URL.Query().Get("liked_offset"))
	if likedOffset < 0 {
		likedOffset = 0
	}
	annotOffset, _ := strconv.Atoi(r.URL.Query().Get("annot_offset"))
	if annotOffset < 0 {
		annotOffset = 0
	}
	limit := 20

	articles, _ := s.db.ListLikedArticles(r.Context(), user.DID, limit+1, likedOffset)
	likedHasMore := len(articles) > limit
	if likedHasMore {
		articles = articles[:limit]
	}

	annotations, _ := s.db.ListAnnotations(r.Context(), "", "", user.DID, limit+1, annotOffset)
	annotHasMore := len(annotations) > limit
	if annotHasMore {
		annotations = annotations[:limit]
	}

	s.render(w, r, "library.html", map[string]any{
		"User":          user,
		"Articles":      articles,
		"Annotations":   annotations,
		"LikedHasMore":  likedHasMore,
		"AnnotHasMore":  annotHasMore,
		"NextLiked":     likedOffset + limit,
		"NextAnnot":     annotOffset + limit,
	})
}

func (s *Server) handleCreateAnnotation(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	a := &db.Annotation{
		AuthorDID:  user.DID,
		FeedURL:    r.FormValue("feed_url"),
		ArticleURL: r.FormValue("article_url"),
		Quote:      sql.NullString{String: r.FormValue("quote"), Valid: r.FormValue("quote") != ""},
		Note:       sql.NullString{String: r.FormValue("note"), Valid: r.FormValue("note") != ""},
		Tags:       sql.NullString{String: r.FormValue("tags"), Valid: r.FormValue("tags") != ""},
		CreatedAt:  sql.NullTime{Time: time.Now(), Valid: true},
	}
	if rv := r.FormValue("rating"); rv != "" {
		var n int64
		if _, err := fmt.Sscanf(rv, "%d", &n); err == nil {
			a.Rating = sql.NullInt64{Int64: n, Valid: true}
		}
	}

	if client := s.pdsClientForUser(r); client != nil {
		record := atproto.AnnotationRecord{
			CreatedAt:  time.Now().Format(time.RFC3339),
			FeedURL:    a.FeedURL,
			ArticleURL: a.ArticleURL,
			Quote:      a.Quote.String,
			Note:       a.Note.String,
			Rating:     int(a.Rating.Int64),
		}
		uri, cid, err := client.CreateRecord(r.Context(), user.DID, "at.glean.annotation", record)
		if err != nil {
			s.logger.Error("failed to write annotation to PDS", "error", err)
			http.Error(w, "failed to write annotation to PDS: "+err.Error(), http.StatusBadGateway)
			return
		}
		a.URI = uri
		a.CID = sql.NullString{String: cid, Valid: true}
	} else {
		a.URI = fmt.Sprintf("glean:annotation:%d", time.Now().UnixNano())
	}

	if err := s.db.CreateAnnotation(r.Context(), a); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
