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

	if err := s.syncSubscriptions(ctx, userDID); err != nil {
		s.logger.Error("sync subscriptions failed", "error", err, "did", userDID)
	}
	// Recompute subscriber_count from subscriptions table rather than
	// maintaining it incrementally (done here to avoid drift from race conditions
	// between stream handler events and sync operations).
	if err := s.articles.RecountSubscriberCounts(ctx); err != nil {
		s.logger.Error("recount subscriber counts failed", "error", err, "did", userDID)
	}
	if err := s.syncLikes(ctx, userDID); err != nil {
		s.logger.Error("sync likes failed", "error", err, "did", userDID)
	}
	if err := s.syncAnnotations(ctx, userDID); err != nil {
		s.logger.Error("sync annotations failed", "error", err, "did", userDID)
	}
	if err := s.syncFollows(ctx, userDID); err != nil {
		s.logger.Error("sync follows failed", "error", err, "did", userDID)
	}

	return nil
}

func (s *Sync) listRecords(ctx context.Context, userDID, collection string) ([]Record, error) {
	var allRecords []Record
	cursor := ""
	for {
		records, next, err := s.client.ListRecords(ctx, userDID, collection, 100, cursor)
		if err != nil {
			return nil, err
		}
		allRecords = append(allRecords, records...)
		if next == "" || len(records) == 0 {
			break
		}
		cursor = next
	}
	return allRecords, nil
}

func (s *Sync) syncSubscriptions(ctx context.Context, userDID string) error {
	gleanRecs, err := s.listRecords(ctx, userDID, CollectionSubscription)
	if err != nil {
		return err
	}
	skyRecs, err := s.listRecords(ctx, userDID, CollectionSkyreaderSubscription)
	if err != nil {
		return err
	}

	var feeds []*db.Feed
	var subs []db.SubData
	activeFeedURLs := make(map[string]bool)

	for _, r := range gleanRecs {
		var rec SubscriptionRecord
		if err := json.Unmarshal(r.Value, &rec); err != nil {
			continue
		}
		if rec.FeedURL == "" {
			continue
		}
		activeFeedURLs[rec.FeedURL] = true
		feeds = append(feeds, &db.Feed{FeedURL: rec.FeedURL, Title: db.NullStr(rec.Title)})
		subs = append(subs, db.SubData{
			FeedURL:  rec.FeedURL,
			Title:    rec.Title,
			Category: rec.Category,
			URI:      r.URI,
			CID:      r.CID,
		})
	}

	for _, r := range skyRecs {
		var rec SkyreaderSubscriptionRecord
		if err := json.Unmarshal(r.Value, &rec); err != nil {
			continue
		}
		if rec.FeedURL == "" {
			continue
		}
		activeFeedURLs[rec.FeedURL] = true
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
			return fmt.Errorf("upsert feeds: %w", err)
		}
	}
	if err := s.articles.BatchReconcileSubscriptions(ctx, userDID, subs); err != nil {
		return err
	}
	if err := s.articles.DeleteOrphanedSubscriptions(ctx, userDID, activeFeedURLs); err != nil {
		return err
	}
	return s.backfillMissingPDSRecords(ctx, userDID)
}

func (s *Sync) syncLikes(ctx context.Context, userDID string) error {
	records, err := s.listRecords(ctx, userDID, CollectionLike)
	if err != nil {
		return err
	}

	var likes []*db.Like
	activeURIs := make(map[string]bool)
	for _, r := range records {
		var rec LikeRecord
		if err := json.Unmarshal(r.Value, &rec); err != nil {
			continue
		}
		if rec.FeedURL == "" || rec.ArticleURL == "" {
			continue
		}
		activeURIs[r.URI] = true
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

	if err := s.articles.BatchCreateLikes(ctx, likes); err != nil {
		return err
	}
	return s.articles.DeleteOrphanedLikes(ctx, userDID, activeURIs)
}

func (s *Sync) syncAnnotations(ctx context.Context, userDID string) error {
	annRecs, err := s.listRecords(ctx, userDID, CollectionAnnotation)
	if err != nil {
		return err
	}
	marginRecs, err := s.listRecords(ctx, userDID, CollectionMarginNote)
	if err != nil {
		return err
	}

	var annotations []*db.Annotation
	activeURIs := make(map[string]bool)

	for _, r := range annRecs {
		var rec AnnotationRecord
		if err := json.Unmarshal(r.Value, &rec); err != nil {
			continue
		}
		if rec.FeedURL == "" || rec.ArticleURL == "" {
			continue
		}
		activeURIs[r.URI] = true
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

	for _, r := range marginRecs {
		var rec MarginNoteRecord
		if err := json.Unmarshal(r.Value, &rec); err != nil {
			continue
		}
		articleURL, quote, note, tags := rec.ToAnnotation()
		if articleURL == "" {
			continue
		}
		activeURIs[r.URI] = true

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

	if err := s.articles.BatchCreateAnnotations(ctx, annotations); err != nil {
		return err
	}
	return s.articles.DeleteOrphanedAnnotations(ctx, userDID, activeURIs)
}

func (s *Sync) syncFollows(ctx context.Context, userDID string) error {
	activeFollows := make(map[string]db.Follow)

	for _, collection := range []string{CollectionBskyFollow, CollectionTangledFollow} {
		records, err := s.listRecords(ctx, userDID, collection)
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
	}

	if len(activeFollows) == 0 {
		return nil
	}

	if err := s.users.BatchCreateUsers(ctx, slices.Collect(maps.Keys(activeFollows))); err != nil {
		return fmt.Errorf("batch create users: %w", err)
	}

	return s.users.SyncFollows(ctx, userDID, activeFollows)
}

func (s *Sync) backfillMissingPDSRecords(ctx context.Context, userDID string) error {
	subs, err := s.articles.ListSubscriptionsWithoutURI(ctx, userDID)
	if err != nil {
		return fmt.Errorf("list subscriptions without URI: %w", err)
	}
	if len(subs) == 0 {
		return nil
	}

	for _, sub := range subs {
		record := SubscriptionRecord{
			CreatedAt: time.Now().Format(time.RFC3339),
			FeedURL:   sub.FeedURL,
			Title:     sub.Title,
			Category:  sub.Category,
		}
		uri, cid, err := s.client.CreateRecord(ctx, userDID, CollectionSubscription, record)
		if err != nil {
			s.logger.Error("failed to backfill PDS record", "error", err, "url", sub.FeedURL)
			continue
		}
		if err := s.articles.UpdateSubscriptionURI(ctx, userDID, sub.FeedURL, uri, cid); err != nil {
			s.logger.Error("failed to backfill subscription URI", "error", err, "url", sub.FeedURL)
			continue
		}
		s.logger.Info("backfilled PDS record", "url", sub.FeedURL, "uri", uri)
	}
	return nil
}
