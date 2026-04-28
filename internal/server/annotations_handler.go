package server

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"golang.org/x/sync/errgroup"

	"pkg.rbrt.fr/glean/internal/atproto"
	"pkg.rbrt.fr/glean/internal/db"
)

func (s *Server) handleLibrary(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	ctx := r.Context()

	limit := 20

	likedPageNum, err := strconv.Atoi(r.URL.Query().Get("liked_page"))
	if err != nil || likedPageNum < 1 {
		likedPageNum = 1
	}
	annotPageNum, err := strconv.Atoi(r.URL.Query().Get("annot_page"))
	if err != nil || annotPageNum < 1 {
		annotPageNum = 1
	}

	likedPage := Pagination{Page: likedPageNum, PageSize: limit}
	annotPage := Pagination{Page: annotPageNum, PageSize: limit}

	articles, err := s.dbs.Articles.ListLikedArticles(ctx, user.DID, limit+1, likedPage.Offset())
	if err != nil {
		s.logger.Warn("failed to list liked articles", "error", err, "did", user.DID)
	}
	likedHasMore := len(articles) > limit
	if likedHasMore {
		articles = articles[:limit]
	}
	likedPage = likedPage.Paginate(len(articles))
	if likedHasMore {
		likedPage.HasNext = true
		likedPage.NextPage = likedPage.Page + 1
	}

	navSuffix := buildNavSuffix("", true)
	for _, a := range articles {
		a.NavSuffix = navSuffix
	}

	annotations, err := s.dbs.Articles.ListAnnotations(ctx, "", "", user.DID, limit+1, annotPage.Offset())
	if err != nil {
		s.logger.Warn("failed to list annotations", "error", err, "did", user.DID)
	}
	resolveAnnotationHandles(ctx, annotations)
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
	ctx := r.Context()
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
		uri, cid, err := client.CreateRecord(ctx, user.DID, atproto.CollectionAnnotation, record)
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

	if err := s.dbs.Articles.CreateAnnotation(ctx, a); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	a.AuthorHandle = atproto.ResolveProfile(r.Context(), user.DID).Handle
	s.render(w, r, "annotation-card.html", map[string]any{
		"annotation": a,
		"userDID":    user.DID,
	})
}

func (s *Server) handleDeleteAnnotation(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	ctx := r.Context()
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	annotation, err := s.dbs.Articles.GetAnnotation(ctx, id)
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
				if delErr := client.DeleteRecord(ctx, user.DID, parsed.Collection, parsed.RKey); delErr != nil {
					s.logger.Error("failed to delete annotation from PDS", "error", delErr)
					http.Error(w, "failed to delete annotation from PDS: "+delErr.Error(), http.StatusBadGateway)
					return
				}
			}
		}
	}

	if err := s.dbs.Articles.DeleteAnnotation(ctx, annotation.URI); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func resolveAnnotationHandles(ctx context.Context, annotations []*db.Annotation) {
	g, gCtx := errgroup.WithContext(ctx)
	g.SetLimit(5)
	for _, a := range annotations {
		g.Go(func() error {
			if a.AuthorDID != "" {
				a.AuthorHandle = atproto.ResolveProfile(gCtx, a.AuthorDID).Handle
			}
			return nil
		})
	}
	_ = g.Wait()
}
