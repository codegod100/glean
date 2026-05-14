package atproto

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"testing"
	"time"

	"gotest.tools/v3/assert"

	"pkg.rbrt.fr/glean/internal/db"
	"pkg.rbrt.fr/glean/internal/feed"
)

func setupStreamTestDB(t *testing.T) *db.Store {
	t.Helper()
	f, err := os.CreateTemp("", "glean-stream-test-*.db")
	assert.NilError(t, err)
	assert.NilError(t, f.Close())
	path := f.Name()
	t.Cleanup(func() {
		for _, suffix := range []string{"", "_users", "_users-shm", "_users-wal", "_articles", "_articles-shm", "_articles-wal", "_recs", "_recs-shm", "_recs-wal"} {
			_ = os.Remove(path + suffix)
		}
	})
	dbs, err := db.Open(path)
	assert.NilError(t, err)
	t.Cleanup(func() { _ = dbs.Close() })
	return dbs
}

func TestHandleMarginNote_SkipsDuplicateFromGleanAnnotation(t *testing.T) {
	ctx := context.Background()
	dbs := setupStreamTestDB(t)
	handler := NewStreamDBHandler(dbs.Articles, dbs.Users, slog.Default())

	now := time.Now().Format(time.RFC3339)

	_ = dbs.Articles.UpsertFeed(ctx, &db.Feed{FeedURL: "https://a.com/feed"})

	err := dbs.Articles.CreateAnnotation(ctx, &db.Annotation{
		URI: "at://did:test:u1/at.glean.annotation/rkey1", AuthorDID: "did:test:u1",
		FeedURL: "https://a.com/feed", ArticleURL: "https://a.com/1",
		Quote: db.NullStr("selected text"), Note: db.NullStr("my note"),
		CreatedAt: db.NullTime(time.Now()),
	})
	assert.NilError(t, err)

	marginRec := NewMarginNoteRecord("https://a.com/1", "selected text", "my note", nil, now)
	value, err := json.Marshal(marginRec)
	assert.NilError(t, err)

	event := &Event{
		Type:       actionCreate,
		DID:        "did:test:u1",
		Collection: CollectionMarginNote,
		RKey:       "margin-rkey1",
		URI:        "at://did:test:u1/at.margin.note/margin-rkey1",
		CID:        "cid-margin1",
		Value:      value,
	}

	err = handler.Handle(ctx, event)
	assert.NilError(t, err)

	exists, err := dbs.Articles.AnnotationExists(ctx, "at://did:test:u1/at.margin.note/margin-rkey1")
	assert.NilError(t, err)
	assert.Equal(t, exists, false, "margin note should not create a duplicate annotation")
}

func TestHandleMarginNote_CreatesWhenNoDuplicate(t *testing.T) {
	ctx := context.Background()
	dbs := setupStreamTestDB(t)
	handler := NewStreamDBHandler(dbs.Articles, dbs.Users, slog.Default())

	now := time.Now().Format(time.RFC3339)

	_ = dbs.Articles.UpsertFeed(ctx, &db.Feed{FeedURL: "https://a.com/feed"})
	_ = dbs.Articles.BatchUpsertArticles(ctx, []feed.Article{{
		FeedURL: "https://a.com/feed", URL: "https://a.com/1", Title: "Test",
	}})

	marginRec := NewMarginNoteRecord("https://a.com/1", "some quote", "some note", nil, now)
	value, err := json.Marshal(marginRec)
	assert.NilError(t, err)

	event := &Event{
		Type:       actionCreate,
		DID:        "did:test:u1",
		Collection: CollectionMarginNote,
		RKey:       "margin-rkey2",
		URI:        "at://did:test:u1/at.margin.note/margin-rkey2",
		CID:        "cid-margin2",
		Value:      value,
	}

	err = handler.Handle(ctx, event)
	assert.NilError(t, err)

	exists, err := dbs.Articles.AnnotationExists(ctx, "at://did:test:u1/at.margin.note/margin-rkey2")
	assert.NilError(t, err)
	assert.Equal(t, exists, true, "margin note should be stored when no duplicate exists")
}

func TestHandleMarginNote_SkipsWhenSameContentDifferentURI(t *testing.T) {
	ctx := context.Background()
	dbs := setupStreamTestDB(t)
	handler := NewStreamDBHandler(dbs.Articles, dbs.Users, slog.Default())

	now := time.Now().Format(time.RFC3339)

	_ = dbs.Articles.UpsertFeed(ctx, &db.Feed{FeedURL: "https://a.com/feed"})

	err := dbs.Articles.CreateAnnotation(ctx, &db.Annotation{
		URI: "at://did:test:u1/at.glean.annotation/rkey3", AuthorDID: "did:test:u1",
		FeedURL: "https://a.com/feed", ArticleURL: "https://a.com/1",
		Note:      db.NullStr("identical note"),
		CreatedAt: db.NullTime(time.Now()),
	})
	assert.NilError(t, err)

	marginRec := NewMarginNoteRecord("https://a.com/1", "", "identical note", nil, now)
	value, err := json.Marshal(marginRec)
	assert.NilError(t, err)

	event := &Event{
		Type:       actionCreate,
		DID:        "did:test:u1",
		Collection: CollectionMarginNote,
		RKey:       "margin-rkey3",
		URI:        "at://did:test:u1/at.margin.note/margin-rkey3",
		CID:        "cid-margin3",
		Value:      value,
	}

	err = handler.Handle(ctx, event)
	assert.NilError(t, err)

	var count int
	err = dbs.SQLDB().QueryRow(`SELECT COUNT(*) FROM articles.annotations WHERE article_url = 'https://a.com/1'`).Scan(&count)
	assert.NilError(t, err)
	assert.Equal(t, count, 1)
}

func TestHandleMarginNote_SkipsEmptyArticleURL(t *testing.T) {
	ctx := context.Background()
	dbs := setupStreamTestDB(t)
	handler := NewStreamDBHandler(dbs.Articles, dbs.Users, slog.Default())

	marginRec := NewMarginNoteRecord("", "quote", "note", nil, time.Now().Format(time.RFC3339))
	value, err := json.Marshal(marginRec)
	assert.NilError(t, err)

	event := &Event{
		Type:       actionCreate,
		DID:        "did:test:u1",
		Collection: CollectionMarginNote,
		RKey:       "margin-rkey4",
		URI:        "at://did:test:u1/at.margin.note/margin-rkey4",
		CID:        "cid-margin4",
		Value:      value,
	}

	err = handler.Handle(ctx, event)
	assert.NilError(t, err)

	var count int
	_ = dbs.SQLDB().QueryRow(`SELECT COUNT(*) FROM articles.annotations`).Scan(&count)
	assert.Equal(t, count, 0)
}
