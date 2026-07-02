package server

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"pkg.rbrt.fr/glean/internal/atproto"
	"pkg.rbrt.fr/glean/internal/db"
	"pkg.rbrt.fr/glean/internal/ml"
)

func (s *Server) handleProfile(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	param := chi.URLParam(r, "did")

	var did string
	if strings.HasPrefix(param, "did:") {
		did = param
	} else {
		resolved, err := atproto.ResolveHandle(ctx, param)
		if err != nil {
			s.logger.Warn("failed to resolve handle", "error", err, "handle", param)
			writeAPIError(w, http.StatusNotFound, "handle not found")
			return
		}
		did = resolved
	}

	profileUser, err := s.dbs.Users.GetUser(ctx, did)
	if err != nil {
		s.logger.Warn("failed to get user", "error", err, "did", did)
		writeAPIError(w, http.StatusNotFound, "user not found")
		return
	}

	p := atproto.ResolveProfile(ctx, did)
	profileUser.Handle = p.Handle
	profileUser.DisplayName = p.DisplayName
	profileUser.AvatarURL = p.AvatarURL

	user := currentUser(r)

	subs, _ := s.dbs.Articles.ListSubscriptions(ctx, did, "", 50, 0)
	annotations, _ := s.dbs.Articles.ListAnnotations(ctx, "", "", did, 50, 0)
	resolveAnnotationHandles(ctx, annotations)
	subCount, _ := s.dbs.Articles.GetSubscriptionCount(ctx, did)
	userLangs, _ := s.dbs.Users.GetLanguages(ctx, user.DID)

	var expandedView, digestEnabled bool
	if settings, err := s.dbs.Users.GetSettings(ctx, user.DID); err == nil && settings != nil {
		expandedView = settings.ExpandedView
		digestEnabled = settings.DigestEnabled
	}

	writeJSON(w, http.StatusOK, profileResponse{
		User:               toUser(user),
		ProfileUser:        toUser(profileUser),
		Subscriptions:      toSubscriptions(subs),
		Annotations:        toAnnotations(annotations),
		SubscriptionCount:  subCount,
		AnnotationCount:    len(annotations),
		UserLanguages:      nonNil(userLangs),
		AvailableLanguages: ml.KnownLanguages(),
		ExpandedView:       expandedView,
		DigestEnabled:      digestEnabled,
	})
}

func toAnnotations(annotations []*db.Annotation) []Annotation {
	out := make([]Annotation, len(annotations))
	for i, a := range annotations {
		out[i] = toAnnotation(a)
	}
	return out
}
