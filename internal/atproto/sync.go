// Sync implements per-user reconciliation using com.atproto.repo.listRecords.
// This is a lighter alternative to full ATProto backfilling (which uses
// com.atproto.sync.getRepo with revision tracking and event buffering).
// The full approach is unnecessary here because:
//   - we only sync known users (not the entire network)
//   - the Jetstream consumer handles real-time events concurrently
//   - all reconcile operations are idempotent
package atproto

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"time"

	"pkg.rbrt.fr/glean/internal/db"
)

type Sync struct {
	articles *db.ArticleStore
	users    *db.UserStore
	client   *Client
	logger   *slog.Logger
}

func NewSync(articles *db.ArticleStore, users *db.UserStore, client *Client, logger *slog.Logger) *Sync {
	return &Sync{articles: articles, users: users, client: client, logger: logger}
}

func (s *Sync) Run(ctx context.Context, userDID string) error {
	s.logger.Info("syncing from PDS", "did", userDID)

	if err := s.syncCollection(ctx, userDID, CollectionSubscription, s.batchReconcileSubscriptions); err != nil {
		s.logger.Error("sync subscriptions failed", "error", err, "did", userDID)
	}
	if err := s.syncCollection(ctx, userDID, CollectionSkyreaderSubscription, s.batchReconcileSkyreaderSubscriptions); err != nil {
		s.logger.Error("sync skyreader subscriptions failed", "error", err, "did", userDID)
	}
	if err := s.syncCollection(ctx, userDID, CollectionLike, s.batchReconcileLikes); err != nil {
		s.logger.Error("sync likes failed", "error", err, "did", userDID)
	}
	if err := s.syncCollection(ctx, userDID, CollectionAnnotation, s.batchReconcileAnnotations); err != nil {
		s.logger.Error("sync annotations failed", "error", err, "did", userDID)
	}
	if err := s.syncCollection(ctx, userDID, CollectionMarginNote, s.batchReconcileMarginNotes); err != nil {
		s.logger.Error("sync margin notes failed", "error", err, "did", userDID)
	}
	if err := s.syncFollows(ctx, userDID); err != nil {
		s.logger.Error("sync follows failed", "error", err, "did", userDID)
	}

	return nil
}

func (s *Sync) syncCollection(ctx context.Context, userDID, collection string, fn func(ctx context.Context, userDID string, records []Record) error) error {
	var allRecords []Record
	cursor := ""
	for {
		records, next, err := s.client.ListRecords(ctx, userDID, collection, 100, cursor)
		if err != nil {
			return err
		}
		allRecords = append(allRecords, records...)
		if next == "" || len(records) == 0 {
			break
		}
		cursor = next
	}
	if len(allRecords) == 0 {
		return nil
	}
	return fn(ctx, userDID, allRecords)
}

func (s *Sync) batchReconcileSubscriptions(ctx context.Context, userDID string, records []Record) error {
	var feeds []*db.Feed
	var subs []db.SubData

	for _, r := range records {
		var rec SubscriptionRecord
		if err := json.Unmarshal(r.Value, &rec); err != nil {
			continue
		}
		if rec.FeedURL == "" {
			continue
		}
		feeds = append(feeds, &db.Feed{FeedURL: rec.FeedURL, Title: db.NullStr(rec.Title)})
		subs = append(subs, db.SubData{
			FeedURL:  rec.FeedURL,
			Title:    rec.Title,
			Category: rec.Category,
			URI:      r.URI,
			CID:      r.CID,
		})
	}

	if len(feeds) > 0 {
		if err := s.articles.BatchUpsertFeeds(ctx, feeds); err != nil {
			return fmt.Errorf("upsert feeds: %w", err)
		}
	}
	return s.articles.BatchReconcileSubscriptions(ctx, userDID, subs)
}

func (s *Sync) batchReconcileSkyreaderSubscriptions(ctx context.Context, userDID string, records []Record) error {
	var feeds []*db.Feed
	var subs []db.SubData

	for _, r := range records {
		var rec SkyreaderSubscriptionRecord
		if err := json.Unmarshal(r.Value, &rec); err != nil {
			continue
		}
		if rec.FeedURL == "" {
			continue
		}
		feeds = append(feeds, &db.Feed{FeedURL: rec.FeedURL, Title: db.NullStr(rec.Title), SiteURL: db.NullStr(rec.SiteURL)})
		subs = append(subs, db.SubData{
			FeedURL: rec.FeedURL,
			Title:   rec.Title,
			URI:     r.URI,
			CID:     r.CID,
		})
	}

	if len(feeds) > 0 {
		if err := s.articles.BatchUpsertFeeds(ctx, feeds); err != nil {
			return fmt.Errorf("failed to upsert feeds: %w", err)
		}
	}

	return s.articles.BatchReconcileSubscriptions(ctx, userDID, subs)
}

func (s *Sync) batchReconcileLikes(ctx context.Context, userDID string, records []Record) error {
	var likes []*db.Like

	for _, r := range records {
		var rec LikeRecord
		if err := json.Unmarshal(r.Value, &rec); err != nil {
			continue
		}
		if rec.FeedURL == "" || rec.ArticleURL == "" {
			continue
		}
		t, _ := time.Parse(time.RFC3339, rec.CreatedAt)
		likes = append(likes, &db.Like{
			URI:        r.URI,
			AuthorDID:  userDID,
			FeedURL:    rec.FeedURL,
			ArticleURL: rec.ArticleURL,
			CreatedAt:  db.NullTime(t),
			CID:        db.NullStr(r.CID),
		})
	}

	return s.articles.BatchCreateLikes(ctx, likes)
}

func (s *Sync) batchReconcileAnnotations(ctx context.Context, userDID string, records []Record) error {
	var annotations []*db.Annotation

	for _, r := range records {
		var rec AnnotationRecord
		if err := json.Unmarshal(r.Value, &rec); err != nil {
			continue
		}
		if rec.FeedURL == "" || rec.ArticleURL == "" {
			continue
		}
		t, _ := time.Parse(time.RFC3339, rec.CreatedAt)
		a := &db.Annotation{
			URI:        r.URI,
			AuthorDID:  userDID,
			FeedURL:    rec.FeedURL,
			ArticleURL: rec.ArticleURL,
			Quote:      db.NullStr(rec.Quote),
			Note:       db.NullStr(rec.Note),
			Tags:       db.NullStrTags(rec.Tags),
			CreatedAt:  db.NullTime(t),
			CID:        db.NullStr(r.CID),
		}
		if rec.Rating > 0 {
			a.Rating = db.NullInt(int64(rec.Rating))
		}
		annotations = append(annotations, a)
	}

	return s.articles.BatchCreateAnnotations(ctx, annotations)
}

func (s *Sync) batchReconcileMarginNotes(ctx context.Context, userDID string, records []Record) error {
	var annotations []*db.Annotation

	for _, r := range records {
		var rec MarginNoteRecord
		if err := json.Unmarshal(r.Value, &rec); err != nil {
			continue
		}
		articleURL, quote, note, tags := rec.ToAnnotation()
		if articleURL == "" {
			continue
		}

		feedURL := ""
		if article, err := s.articles.GetArticleByURL(ctx, articleURL); err == nil {
			feedURL = article.FeedURL
		}

		t, _ := time.Parse(time.RFC3339, rec.CreatedAt)
		annotations = append(annotations, &db.Annotation{
			URI:        r.URI,
			AuthorDID:  userDID,
			FeedURL:    feedURL,
			ArticleURL: articleURL,
			Quote:      db.NullStr(quote),
			Note:       db.NullStr(note),
			Tags:       db.NullStrTags(tags),
			CreatedAt:  db.NullTime(t),
			CID:        db.NullStr(r.CID),
		})
	}

	return s.articles.BatchCreateAnnotations(ctx, annotations)
}

func (s *Sync) syncFollows(ctx context.Context, userDID string) error {
	activeFollows := make(map[string]db.Follow)

	for _, collection := range []string{CollectionBskyFollow, CollectionTangledFollow} {
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

	if err := s.users.BatchCreateUsers(ctx, slices.Collect(maps.Keys(activeFollows))); err != nil {
		return fmt.Errorf("batch create users: %w", err)
	}

	return s.users.SyncFollows(ctx, userDID, activeFollows)
}
