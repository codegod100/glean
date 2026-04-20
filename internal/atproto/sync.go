package atproto

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"pkg.rbrt.fr/glean/internal/db"
)

type Sync struct {
	db     *db.DB
	client *Client
	logger *slog.Logger
}

func NewSync(database *db.DB, client *Client, logger *slog.Logger) *Sync {
	return &Sync{db: database, client: client, logger: logger}
}

func (s *Sync) Run(ctx context.Context, userDID string) error {
	s.logger.Info("syncing from PDS", "did", userDID)

	if err := s.syncCollection(ctx, userDID, "at.glean.subscription", s.reconcileSubscription); err != nil {
		s.logger.Error("sync subscriptions failed", "error", err, "did", userDID)
	}
	if err := s.syncCollection(ctx, userDID, "at.glean.like", s.reconcileLike); err != nil {
		s.logger.Error("sync likes failed", "error", err, "did", userDID)
	}
	if err := s.syncCollection(ctx, userDID, "at.glean.annotation", s.reconcileAnnotation); err != nil {
		s.logger.Error("sync annotations failed", "error", err, "did", userDID)
	}
	if err := s.syncFollows(ctx, userDID); err != nil {
		s.logger.Error("sync follows failed", "error", err, "did", userDID)
	}

	return nil
}

type reconcileFunc func(ctx context.Context, userDID, uri, cid string, value json.RawMessage) error

func (s *Sync) syncCollection(ctx context.Context, userDID, collection string, fn reconcileFunc) error {
	cursor := ""
	for {
		records, next, err := s.client.ListRecords(ctx, userDID, collection, 100, cursor)
		if err != nil {
			return err
		}

		for _, r := range records {
			if err := fn(ctx, userDID, r.URI, r.CID, r.Value); err != nil {
				s.logger.Error("reconcile record error", "error", err, "uri", r.URI)
			}
		}

		if next == "" || len(records) == 0 {
			break
		}
		cursor = next
	}
	return nil
}

func (s *Sync) reconcileSubscription(ctx context.Context, userDID, uri, cid string, value json.RawMessage) error {
	var rec SubscriptionRecord
	if err := json.Unmarshal(value, &rec); err != nil {
		return err
	}

	if rec.FeedURL == "" {
		return nil
	}

	existing, err := s.db.GetSubscription(ctx, userDID, rec.FeedURL)
	if err == nil && existing != nil {
		if !existing.URI.Valid || existing.URI.String == "" {
			return s.db.UpdateSubscriptionURI(ctx, userDID, rec.FeedURL, uri, cid)
		}
		return nil
	}

	f := &db.Feed{FeedURL: rec.FeedURL, Title: db.NullStr(rec.Title)}
	_ = s.db.UpsertFeed(ctx, f)

	return s.db.CreateSubscription(ctx, userDID, rec.FeedURL, rec.Title, rec.Category, uri, cid)
}

func (s *Sync) reconcileLike(ctx context.Context, userDID, uri, cid string, value json.RawMessage) error {
	var rec LikeRecord
	if err := json.Unmarshal(value, &rec); err != nil {
		return err
	}

	if rec.FeedURL == "" || rec.ArticleURL == "" {
		return nil
	}

	exists, err := s.db.HasLiked(ctx, userDID, rec.FeedURL, rec.ArticleURL)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	t, _ := time.Parse(time.RFC3339, rec.CreatedAt)
	like := &db.Like{
		URI:        uri,
		AuthorDID:  userDID,
		FeedURL:    rec.FeedURL,
		ArticleURL: rec.ArticleURL,
		CreatedAt:  db.NullTime(t),
		CID:        db.NullStr(cid),
	}
	return s.db.CreateLike(ctx, like)
}

func (s *Sync) reconcileAnnotation(ctx context.Context, userDID, uri, cid string, value json.RawMessage) error {
	var rec AnnotationRecord
	if err := json.Unmarshal(value, &rec); err != nil {
		return err
	}

	if rec.FeedURL == "" || rec.ArticleURL == "" {
		return nil
	}

	var existing []*db.Annotation
	existing, _ = s.db.ListAnnotations(ctx, rec.FeedURL, rec.ArticleURL, userDID, 100, 0)
	for _, a := range existing {
		if a.URI == uri {
			return nil
		}
	}

	t, _ := time.Parse(time.RFC3339, rec.CreatedAt)
	a := &db.Annotation{
		URI:        uri,
		AuthorDID:  userDID,
		FeedURL:    rec.FeedURL,
		ArticleURL: rec.ArticleURL,
		Quote:      db.NullStr(rec.Quote),
		Note:       db.NullStr(rec.Note),
		Tags:       db.NullStrTags(rec.Tags),
		CreatedAt:  db.NullTime(t),
		CID:        db.NullStr(cid),
	}
	if rec.Rating > 0 {
		a.Rating = db.NullInt(int64(rec.Rating))
	}
	return s.db.CreateAnnotation(ctx, a)
}

func (s *Sync) syncFollows(ctx context.Context, userDID string) error {
	activeFollows := make(map[string]db.Follow)

	for _, collection := range []string{"app.bsky.graph.follow", "sh.tangled.graph.follow"} {
		cursor := ""
		for {
			records, next, err := s.client.ListRecords(ctx, userDID, collection, 100, cursor)
			if err != nil {
				return err
			}

			for _, r := range records {
				var rec FollowRecord
				if err := json.Unmarshal(r.Value, &rec); err != nil {
					continue
				}
				if rec.Subject == "" {
					continue
				}

				t, _ := time.Parse(time.RFC3339, rec.CreatedAt)
				activeFollows[rec.Subject] = db.Follow{
					URI:        db.NullStr(r.URI),
					CID:        db.NullStr(r.CID),
					FollowedAt: db.NullTime(t),
				}

				var handle string
				if rec.Subject != userDID {
					s.db.CreateUser(ctx, rec.Subject, handle, "", "")
				}
			}

			if next == "" || len(records) == 0 {
				break
			}
			cursor = next
		}
	}

	if len(activeFollows) == 0 {
		return nil
	}

	return s.db.SyncFollows(ctx, userDID, activeFollows)
}
