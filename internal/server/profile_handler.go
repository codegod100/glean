package server

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"pkg.rbrt.fr/glean/internal/atproto"
)

func (s *Server) handleProfile(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	param := chi.URLParam(r, "did")

	var did string
	if strings.HasPrefix(param, "did:") {
		did = param
	} else {
		profileUser, err := s.dbs.Articles.GetUserByHandle(ctx, param)
		if err == nil {
			did = profileUser.DID
		} else {
			resolved, err := atproto.ResolveHandle(ctx, param)
			if err != nil {
				s.logger.Warn("failed to resolve handle", "error", err, "handle", param)
				http.Error(w, "handle not found", http.StatusNotFound)
				return
			}
			did = resolved
		}
	}

	profileUser, err := s.dbs.Articles.GetUser(ctx, did)
	if err != nil {
		s.logger.Warn("failed to get user", "error", err, "did", did)
		http.Error(w, "user not found", http.StatusNotFound)
		return
	}

	if !profileUser.AvatarURL.Valid || profileUser.AvatarURL.String == "" {
		_, displayName, avatarURL, err := atproto.FetchProfile(ctx, did)
		if err == nil && avatarURL != "" {
			if err := s.dbs.Articles.UpdateUserProfile(ctx, did, displayName, avatarURL); err != nil {
				s.logger.Warn("failed to update user profile", "error", err, "did", did)
			}
			profileUser.DisplayName = nullString(displayName)
			profileUser.AvatarURL = nullString(avatarURL)
		}
	}

	subs, err := s.dbs.Articles.ListSubscriptions(ctx, did, "", 50, 0)
	if err != nil {
		s.logger.Warn("failed to list subscriptions", "error", err, "did", did)
	}

	annotations, err := s.dbs.Articles.ListAnnotations(ctx, "", "", did, 50, 0)
	if err != nil {
		s.logger.Warn("failed to list annotations", "error", err, "did", did)
	}

	subCount, err := s.dbs.Articles.GetSubscriptionCount(ctx, did)
	if err != nil {
		s.logger.Warn("failed to get subscription count", "error", err, "did", did)
	}

	user := currentUser(r)

	s.render(w, r, "profile.html", map[string]any{
		"User":              user,
		"CurrentUserDID":    user.DID,
		"ProfileUser":       profileUser,
		"Subscriptions":     subs,
		"Annotations":       annotations,
		"SubscriptionCount": subCount,
		"AnnotationCount":   len(annotations),
	})
}
