package server

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"pkg.rbrt.fr/glean/internal/atproto"
	"pkg.rbrt.fr/glean/internal/langdetect"
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
			s.renderError(w, r, http.StatusNotFound, "Handle not found", "Could not find a user with that handle.")
			return
		}
		did = resolved
	}

	profileUser, err := s.dbs.Users.GetUser(ctx, did)
	if err != nil {
		s.logger.Warn("failed to get user", "error", err, "did", did)
		s.renderError(w, r, http.StatusNotFound, "User not found", "This user doesn't exist in Glean yet.")
		return
	}

	p := atproto.ResolveProfile(ctx, did)
	profileUser.Handle = p.Handle
	profileUser.DisplayName = p.DisplayName
	profileUser.AvatarURL = p.AvatarURL

	subs, err := s.dbs.Articles.ListSubscriptions(ctx, did, "", 50, 0)
	if err != nil {
		s.logger.Warn("failed to list subscriptions", "error", err, "did", did)
	}

	annotations, err := s.dbs.Articles.ListAnnotations(ctx, "", "", did, 50, 0)
	if err != nil {
		s.logger.Warn("failed to list annotations", "error", err, "did", did)
	}
	resolveAnnotationHandles(ctx, annotations)

	subCount, err := s.dbs.Articles.GetSubscriptionCount(ctx, did)
	if err != nil {
		s.logger.Warn("failed to get subscription count", "error", err, "did", did)
	}

	user := currentUser(r)

	userLangs, _ := s.dbs.Users.GetLanguages(ctx, user.DID)

	s.render(w, r, "profile.html", map[string]any{
		"User":               user,
		"CurrentUserDID":     user.DID,
		"ProfileUser":        profileUser,
		"Subscriptions":      subs,
		"Annotations":        annotations,
		"SubscriptionCount":  subCount,
		"AnnotationCount":    len(annotations),
		"UserLanguages":      userLangs,
		"AvailableLanguages": langdetect.KnownLanguages(),
	})
}
