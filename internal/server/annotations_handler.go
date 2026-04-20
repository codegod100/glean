package server

import (
	"database/sql"
	"fmt"
	"net/http"
	"time"

	"pkg.rbrt.fr/glean/internal/atproto"
	"pkg.rbrt.fr/glean/internal/db"
)

func (s *Server) handleAnnotations(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	articleURL := r.URL.Query().Get("article")
	annotations, _ := s.db.ListAnnotations(r.Context(), "", articleURL, "", 50, 0)
	s.render(w, r, "annotations.html", map[string]any{
		"User":        user,
		"Annotations": annotations,
		"ArticleURL":  articleURL,
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
