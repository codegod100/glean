package server

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"pkg.rbrt.fr/glean/internal/atproto"
)

func (s *Server) handleProfile(w http.ResponseWriter, r *http.Request) {
	param := chi.URLParam(r, "did")

	var did string
	if strings.HasPrefix(param, "did:") {
		did = param
	} else {
		profileUser, err := s.db.GetUserByHandle(r.Context(), param)
		if err == nil {
			did = profileUser.DID
		} else {
			resolved, err := atproto.ResolveHandle(r.Context(), param)
			if err != nil {
				http.Error(w, "handle not found", http.StatusNotFound)
				return
			}
			did = resolved
		}
	}

	profileUser, err := s.db.GetUser(r.Context(), did)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	if !profileUser.AvatarURL.Valid || profileUser.AvatarURL.String == "" {
		_, displayName, avatarURL, err := atproto.FetchProfile(r.Context(), did)
		if err == nil && avatarURL != "" {
			_ = s.db.UpdateUserProfile(r.Context(), did, displayName, avatarURL)
			profileUser.DisplayName = nullString(displayName)
			profileUser.AvatarURL = nullString(avatarURL)
		}
	}

	subs, _ := s.db.ListSubscriptions(r.Context(), did, "", 50, 0)
	annotations, _ := s.db.ListAnnotations(r.Context(), "", "", did, 50, 0)
	subCount, _ := s.db.GetSubscriptionCount(r.Context(), did)

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
