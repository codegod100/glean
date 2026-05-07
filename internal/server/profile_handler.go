package server

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"golang.org/x/sync/errgroup"

	"pkg.rbrt.fr/glean/internal/atproto"
	"pkg.rbrt.fr/glean/internal/db"
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

	var (
		subs        []*db.Subscription
		annotations []*db.Annotation
		subCount    int
		userLangs   []string
	)

	user := currentUser(r)

	g, gCtx := errgroup.WithContext(ctx)

	g.Go(func() error {
		var err error
		subs, err = s.dbs.Articles.ListSubscriptions(gCtx, did, "", 50, 0)
		if err != nil {
			s.logger.Warn("failed to list subscriptions", "error", err, "did", did)
		}
		return nil
	})

	g.Go(func() error {
		var err error
		annotations, err = s.dbs.Articles.ListAnnotations(gCtx, "", "", did, 50, 0)
		if err != nil {
			s.logger.Warn("failed to list annotations", "error", err, "did", did)
			return nil
		}
		resolveAnnotationHandles(gCtx, annotations)
		return nil
	})

	g.Go(func() error {
		var err error
		subCount, err = s.dbs.Articles.GetSubscriptionCount(gCtx, did)
		if err != nil {
			s.logger.Warn("failed to get subscription count", "error", err, "did", did)
		}
		return nil
	})

	g.Go(func() error {
		userLangs, _ = s.dbs.Users.GetLanguages(gCtx, user.DID)
		return nil
	})

	if err := g.Wait(); err != nil {
		s.logger.Warn("profile error", "error", err, "did", did)
	}

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
