package atproto

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"pkg.rbrt.fr/glean/internal/db"
)

var sentinelErrors = []error{db.ErrDuplicateSubscription, db.ErrDuplicateLike}

func isSentinel(err error) bool {
	for _, s := range sentinelErrors {
		if errors.Is(err, s) {
			return true
		}
	}
	return false
}

type StreamDBHandler struct {
	articles *db.DB
	users    *db.DB
	logger   *slog.Logger
}

func NewStreamDBHandler(articles, users *db.DB, logger *slog.Logger) *StreamDBHandler {
	return &StreamDBHandler{articles: articles, users: users, logger: logger}
}

func (h *StreamDBHandler) Handle(ctx context.Context, event *Event) error {
	switch event.Collection {
	case CollectionSubscription:
		return h.handleSubscription(ctx, event)
	case CollectionSkyreaderSubscription:
		return h.handleSkyreaderSubscription(ctx, event)
	case CollectionLike:
		return h.handleLike(ctx, event)
	case CollectionAnnotation:
		return h.handleAnnotation(ctx, event)
	case CollectionMarginNote:
		return h.handleMarginNote(ctx, event)
	case CollectionBskyFollow, CollectionTangledFollow:
		return h.handleFollow(ctx, event)
	}
	return nil
}

func (h *StreamDBHandler) handleSubscription(ctx context.Context, event *Event) error {
	switch event.Type {
	case "create", "update":
		var rec SubscriptionRecord
		if err := json.Unmarshal(event.Value, &rec); err != nil {
			return err
		}
		if rec.FeedURL == "" {
			return nil
		}

		_ = h.articles.UpsertFeed(ctx, &db.Feed{FeedURL: rec.FeedURL, Title: db.NullStr(rec.Title)})
		err := h.articles.CreateSubscription(ctx, event.DID, rec.FeedURL, rec.Title, rec.Category, event.URI, event.CID)
		if isSentinel(err) {
			return nil
		}
		return err

	case "delete":
		parsed, ok := ParseRecordURI(event.URI)
		if !ok {
			return nil
		}
		sub, err := h.articles.GetSubscriptionByURI(ctx, event.DID, event.URI)
		if err == nil && sub != nil {
			return h.articles.DeleteSubscription(ctx, event.DID, sub.FeedURL)
		}
		_ = parsed
	}
	return nil
}

func (h *StreamDBHandler) handleLike(ctx context.Context, event *Event) error {
	switch event.Type {
	case "create":
		var rec LikeRecord
		if err := json.Unmarshal(event.Value, &rec); err != nil {
			return err
		}
		if rec.FeedURL == "" || rec.ArticleURL == "" {
			return nil
		}

		t, _ := time.Parse(time.RFC3339, rec.CreatedAt)
		err := h.articles.CreateLike(ctx, &db.Like{
			URI:        event.URI,
			AuthorDID:  event.DID,
			FeedURL:    rec.FeedURL,
			ArticleURL: rec.ArticleURL,
			CreatedAt:  sql.NullTime{Time: t, Valid: true},
			CID:        sql.NullString{String: event.CID, Valid: event.CID != ""},
		})
		if isSentinel(err) {
			return nil
		}
		return err

	case "delete":
		return h.articles.DeleteLike(ctx, event.URI)
	}
	return nil
}

func (h *StreamDBHandler) handleAnnotation(ctx context.Context, event *Event) error {
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
		return h.articles.CreateAnnotation(ctx, a)

	case "delete":
		return h.articles.DeleteAnnotation(ctx, event.URI)
	}
	return nil
}

func (h *StreamDBHandler) handleFollow(ctx context.Context, event *Event) error {
	switch event.Type {
	case "create":
		var rec FollowRecord
		if err := json.Unmarshal(event.Value, &rec); err != nil {
			return err
		}
		if rec.Subject == "" {
			return nil
		}
		return h.users.UpsertFollow(ctx, event.DID, rec.Subject, event.URI, event.CID)

	case "delete":
		return h.users.DeleteFollowByURI(ctx, event.URI)
	}
	return nil
}

func (h *StreamDBHandler) handleMarginNote(ctx context.Context, event *Event) error {
	switch event.Type {
	case "create", "update":
		var rec MarginNoteRecord
		if err := json.Unmarshal(event.Value, &rec); err != nil {
			return err
		}

		articleURL, quote, note, tags := rec.ToAnnotation()
		if articleURL == "" {
			return nil
		}

		feedURL := h.resolveFeedURL(ctx, articleURL)

		t, _ := time.Parse(time.RFC3339, rec.CreatedAt)
		a := &db.Annotation{
			URI:        event.URI,
			AuthorDID:  event.DID,
			FeedURL:    feedURL,
			ArticleURL: articleURL,
			Quote:      db.NullStr(quote),
			Note:       db.NullStr(note),
			Tags:       db.NullStrTags(tags),
			CreatedAt:  sql.NullTime{Time: t, Valid: true},
			CID:        sql.NullString{String: event.CID, Valid: event.CID != ""},
		}
		return h.articles.CreateAnnotation(ctx, a)

	case "delete":
	}
	return nil
}

func (h *StreamDBHandler) handleSkyreaderSubscription(ctx context.Context, event *Event) error {
	switch event.Type {
	case "create", "update":
		var rec SkyreaderSubscriptionRecord
		if err := json.Unmarshal(event.Value, &rec); err != nil {
			return err
		}
		if rec.FeedURL == "" {
			return nil
		}

		_ = h.articles.UpsertFeed(ctx, &db.Feed{FeedURL: rec.FeedURL, Title: db.NullStr(rec.Title), SiteURL: db.NullStr(rec.SiteURL)})
		err := h.articles.CreateSubscription(ctx, event.DID, rec.FeedURL, rec.Title, "", event.URI, event.CID)
		if isSentinel(err) {
			return nil
		}
		return err

	case "delete":
	}
	return nil
}

func (h *StreamDBHandler) resolveFeedURL(ctx context.Context, articleURL string) string {
	article, err := h.articles.GetArticleByURL(ctx, articleURL)
	if err != nil {
		return ""
	}
	return article.FeedURL
}
