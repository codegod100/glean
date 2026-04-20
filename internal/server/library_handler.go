package server

import (
	"net/http"
	"strconv"
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
	pageLimit := 20

	articles, _ := s.db.ListLikedArticles(r.Context(), user.DID, pageLimit+1, likedOffset)
	likedHasMore := len(articles) > pageLimit
	if likedHasMore {
		articles = articles[:pageLimit]
	}

	annotations, _ := s.db.ListAnnotations(r.Context(), "", "", user.DID, pageLimit+1, annotOffset)
	annotHasMore := len(annotations) > pageLimit
	if annotHasMore {
		annotations = annotations[:pageLimit]
	}

	s.render(w, r, "library.html", map[string]any{
		"User":         user,
		"Articles":     articles,
		"Annotations":  annotations,
		"LikedHasMore": likedHasMore,
		"AnnotHasMore": annotHasMore,
		"LikedOffset":  likedOffset,
		"AnnotOffset":  annotOffset,
		"NextLiked":    likedOffset + pageLimit,
		"NextAnnot":    annotOffset + pageLimit,
	})
}
