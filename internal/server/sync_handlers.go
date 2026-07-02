package server

import (
	"context"
	"sync"

	"github.com/bluesky-social/indigo/atproto/syntax"

	"pkg.rbrt.fr/glean/internal/atproto"
	"pkg.rbrt.fr/glean/internal/metrics"
)

func (s *Server) BackfillFromCollectionDir(ctx context.Context, collectionDirURL string, concurrency int) {
	if collectionDirURL == "" {
		return
	}

	s.logger.Info("backfilling from collection directory", "url", collectionDirURL)

	dids, err := atproto.FetchSubscriberDIDs(ctx, collectionDirURL)
	if err != nil {
		s.logger.Error("failed to fetch subscriber DIDs", "error", err)
		return
	}

	existing, err := s.dbs.Users.UserDIDs(ctx)
	if err != nil {
		s.logger.Error("failed to list existing users", "error", err)
		return
	}

	var missing []string
	for _, did := range dids {
		if !existing[did] {
			missing = append(missing, did)
		}
	}

	s.logger.Info("collection directory backfill", "total", len(dids), "missing", len(missing))

	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	for _, did := range missing {
		if ctx.Err() != nil {
			break
		}

		sem <- struct{}{}
		wg.Add(1)

		go func(did string) {
			defer func() { <-sem }()
			defer wg.Done()

			if _, err := s.dbs.Users.CreateUser(ctx, did); err != nil {
				s.logger.Error("failed to create user during backfill", "error", err, "did", did)
				return
			}

			pdsURL, err := atproto.ResolvePDSEndpoint(ctx, did)
			if err != nil {
				s.logger.Error("failed to resolve PDS for backfill", "error", err, "did", did)
				return
			}

			client := atproto.NewUnauthenticatedClient(pdsURL)
			sync := atproto.NewSync(s.dbs.Articles, s.dbs.Users, client, s.logger)
			if err := sync.Run(ctx, did); err != nil {
				s.logger.Error("backfill sync failed", "error", err, "did", did)
			}
		}(did)
	}

	wg.Wait()
	s.logger.Info("collection directory backfill complete")
}

func (s *Server) runSyncAll(ctx context.Context) {
	if n, err := s.oauthStore.CountActiveUsers(ctx); err == nil {
		metrics.ActiveUsers.Set(float64(n))
	}

	users, err := s.dbs.Users.ListUsers(ctx)
	if err != nil {
		s.logger.Error("failed to list users for sync", "error", err)
		return
	}

	for _, u := range users {
		sessionIDs, err := s.oauthStore.ListSessionsForDID(ctx, u.DID)
		if err != nil || len(sessionIDs) == 0 {
			continue
		}

		did, err := syntax.ParseDID(u.DID)
		if err != nil {
			continue
		}

		sess, err := s.oauth.ResumeSession(ctx, did, sessionIDs[0])
		if err != nil {
			s.logger.Warn("failed to resume session for periodic sync", "error", err, "did", u.DID)
			continue
		}

		client := atproto.NewClient(sess.APIClient())
		sync := atproto.NewSync(s.dbs.Articles, s.dbs.Users, client, s.logger)
		if err := sync.Run(ctx, u.DID); err != nil {
			metrics.SyncErrors.Inc()
			s.logger.Error("periodic sync failed", "error", err, "did", u.DID)
		}

		metrics.SyncRuns.Inc()
	}

	if err := s.dbs.Articles.RecountSubscriberCounts(ctx); err != nil {
		s.logger.Error("recount subscriber counts failed", "error", err)
	}
}
