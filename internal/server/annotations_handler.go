package server

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"pkg.rbrt.fr/glean/internal/atproto"
	"pkg.rbrt.fr/glean/internal/db"
)

func (s *Server) handleLibrary(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)

	limit := 20

	likedPageNum, _ := strconv.Atoi(r.URL.Query().Get("liked_page"))
	if likedPageNum < 1 {
		likedPageNum = 1
	}
	annotPageNum, _ := strconv.Atoi(r.URL.Query().Get("annot_page"))
	if annotPageNum < 1 {
		annotPageNum = 1
	}

	likedPage := Pagination{Page: likedPageNum, PageSize: limit}
	annotPage := Pagination{Page: annotPageNum, PageSize: limit}

	articles, _ := s.db.ListLikedArticles(r.Context(), user.DID, limit+1, likedPage.Offset())
	likedHasMore := len(articles) > limit
	if likedHasMore {
		articles = articles[:limit]
	}
	likedPage = likedPage.Paginate(len(articles))
	if likedHasMore {
		likedPage.HasNext = true
		likedPage.NextPage = likedPage.Page + 1
	}

	annotations, _ := s.db.ListAnnotations(r.Context(), "", "", user.DID, limit+1, annotPage.Offset())
	annotHasMore := len(annotations) > limit
	if annotHasMore {
		annotations = annotations[:limit]
	}
	annotPage = annotPage.Paginate(len(annotations))
	if annotHasMore {
		annotPage.HasNext = true
		annotPage.NextPage = annotPage.Page + 1
	}

	s.render(w, r, "library.html", map[string]any{
		"User":           user,
		"CurrentUserDID": user.DID,
		"Articles":       articles,
		"Annotations":    annotations,
		"LikedPage":      likedPage,
		"AnnotPage":      annotPage,
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
		var tags []string
		if a.Tags.Valid && a.Tags.String != "" {
			tags = strings.Split(a.Tags.String, ",")
		}
		record := atproto.AnnotationRecord{
			CreatedAt:  time.Now().Format(time.RFC3339),
			FeedURL:    a.FeedURL,
			ArticleURL: a.ArticleURL,
			Quote:      a.Quote.String,
			Note:       a.Note.String,
			Tags:       tags,
			Rating:     int(a.Rating.Int64),
		}
		uri, cid, err := client.CreateRecord(r.Context(), user.DID, atproto.CollectionAnnotation, record)
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

func (s *Server) handleDeleteAnnotation(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	annotation, err := s.db.GetAnnotation(r.Context(), id)
	if err != nil {
		http.Error(w, "annotation not found", http.StatusNotFound)
		return
	}

	if annotation.AuthorDID != user.DID {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	if annotation.URI != "" {
		if client := s.pdsClientForUser(r); client != nil {
			parsed, ok := atproto.ParseRecordURI(annotation.URI)
			if ok {
				if delErr := client.DeleteRecord(r.Context(), user.DID, parsed.Collection, parsed.RKey); delErr != nil {
					s.logger.Error("failed to delete annotation from PDS", "error", delErr)
				}
			}
		}
	}

	if err := s.db.DeleteAnnotation(r.Context(), annotation.URI); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}
