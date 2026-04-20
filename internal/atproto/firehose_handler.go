package atproto

import (
	"context"
	"encoding/json"
	"database/sql"
	"log/slog"
	"time"

	"pkg.rbrt.fr/glean/internal/db"
)

type FirehoseDBHandler struct {
	db     *db.DB
	logger *slog.Logger
}

func NewFirehoseDBHandler(database *db.DB, logger *slog.Logger) *FirehoseDBHandler {
	return &FirehoseDBHandler{db: database, logger: logger}
}

func (h *FirehoseDBHandler) Handle(ctx context.Context, event *FirehoseEvent) error {
	switch event.Collection {
	case "at.glean.subscription":
		return h.handleSubscription(ctx, event)
	case "at.glean.like":
		return h.handleLike(ctx, event)
	case "at.glean.annotation":
		return h.handleAnnotation(ctx, event)
	}
	return nil
}

func (h *FirehoseDBHandler) handleSubscription(ctx context.Context, event *FirehoseEvent) error {
	switch event.Type {
	case "create", "update":
		var rec SubscriptionRecord
		if err := json.Unmarshal(event.Value, &rec); err != nil {
			return err
		}
		if rec.FeedURL == "" {
			return nil
		}

		existing, err := h.db.GetSubscription(ctx, event.DID, rec.FeedURL)
		if err == nil && existing != nil {
			if !existing.URI.Valid || existing.URI.String == "" {
				return h.db.UpdateSubscriptionURI(ctx, event.DID, rec.FeedURL, event.URI, event.CID)
			}
			return nil
		}

		f := &db.Feed{FeedURL: rec.FeedURL, Title: db.NullStr(rec.Title)}
		_ = h.db.UpsertFeed(ctx, f)
		return h.db.CreateSubscription(ctx, event.DID, rec.FeedURL, rec.Title, rec.Category, event.URI, event.CID)

	case "delete":
		parsed, ok := ParseRecordURI(event.URI)
		if !ok {
			return nil
		}
		subs, _ := h.db.ListSubscriptions(ctx, event.DID, "", 100, 0)
		for _, sub := range subs {
			if sub.URI.Valid && sub.URI.String == event.URI {
				return h.db.DeleteSubscription(ctx, event.DID, sub.FeedURL)
			}
		}
		_ = parsed
	}
	return nil
}

func (h *FirehoseDBHandler) handleLike(ctx context.Context, event *FirehoseEvent) error {
	switch event.Type {
	case "create":
		var rec LikeRecord
		if err := json.Unmarshal(event.Value, &rec); err != nil {
			return err
		}
		if rec.FeedURL == "" || rec.ArticleURL == "" {
			return nil
		}

		exists, err := h.db.HasLiked(ctx, event.DID, rec.FeedURL, rec.ArticleURL)
		if err != nil || exists {
			return nil
		}

		t, _ := time.Parse(time.RFC3339, rec.CreatedAt)
		return h.db.CreateLike(ctx, &db.Like{
			URI:        event.URI,
			AuthorDID:  event.DID,
			FeedURL:    rec.FeedURL,
			ArticleURL: rec.ArticleURL,
			CreatedAt:  sql.NullTime{Time: t, Valid: true},
			CID:        sql.NullString{String: event.CID, Valid: event.CID != ""},
		})

	case "delete":
		return h.db.DeleteLike(ctx, event.URI)
	}
	return nil
}

func (h *FirehoseDBHandler) handleAnnotation(ctx context.Context, event *FirehoseEvent) error {
	switch event.Type {
	case "create":
		var rec AnnotationRecord
		if err := json.Unmarshal(event.Value, &rec); err != nil {
			return err
		}
		if rec.FeedURL == "" || rec.ArticleURL == "" {
			return nil
		}

		t, _ := time.Parse(time.RFC3339, rec.CreatedAt)
		a := &db.Annotation{
			URI:        event.URI,
			AuthorDID:  event.DID,
			FeedURL:    rec.FeedURL,
			ArticleURL: rec.ArticleURL,
			Quote:      db.NullStr(rec.Quote),
			Note:       db.NullStr(rec.Note),
			Tags:       db.NullStrTags(rec.Tags),
			CreatedAt:  sql.NullTime{Time: t, Valid: true},
			CID:        sql.NullString{String: event.CID, Valid: event.CID != ""},
		}
		if rec.Rating > 0 {
			a.Rating = sql.NullInt64{Int64: int64(rec.Rating), Valid: true}
		}
		return h.db.CreateAnnotation(ctx, a)

	case "delete":
		return h.db.DeleteAnnotation(ctx, event.URI)
	}
	return nil
}
