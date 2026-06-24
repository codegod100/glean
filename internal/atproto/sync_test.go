package atproto

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"gotest.tools/v3/assert"

	"pkg.rbrt.fr/glean/internal/db"
)

func setupSyncTestDB(t *testing.T) *db.Store {
	t.Helper()
	f, err := os.CreateTemp("", "glean-sync-test-*.db")
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

type rawRecord struct {
	URI   string          `json:"uri"`
	CID   string          `json:"cid"`
	Value json.RawMessage `json:"value"`
}

// newSyncTestServer returns an httptest PDS that replies to listRecords for the
// given collection -> records map.
func newSyncTestServer(t *testing.T, byCollection map[string][]Record) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/xrpc/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var raw []rawRecord
		for _, rec := range byCollection[r.URL.Query().Get("collection")] {
			raw = append(raw, rawRecord{URI: rec.URI, CID: rec.CID, Value: rec.Value})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"records": raw})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

func TestSyncAnnotations_DedupsMirroredMarginNote(t *testing.T) {
	ctx := context.Background()
	dbs := setupSyncTestDB(t)

	assert.NilError(t, dbs.Articles.UpsertFeed(ctx, &db.Feed{FeedURL: "https://a.com/feed"}))

	now := time.Now().Format(time.RFC3339)
	annValue, err := json.Marshal(AnnotationRecord{
		CreatedAt: now, FeedURL: "https://a.com/feed", ArticleURL: "https://a.com/1",
		Quote: "selected text", Note: "my note",
	})
	assert.NilError(t, err)

	marginRec := NewMarginNoteRecord("https://a.com/1", "selected text", "my note", nil, now)
	marginValue, err := json.Marshal(marginRec)
	assert.NilError(t, err)

	server := newSyncTestServer(t, map[string][]Record{
		CollectionAnnotation: {
			{URI: "at://did:test:u1/at.glean.annotation/rkey1", CID: "cid-a1", Value: annValue},
		},
		CollectionMarginNote: {
			{URI: "at://did:test:u1/at.margin.note/rkey1", CID: "cid-m1", Value: marginValue},
		},
	})

	sync := NewSync(dbs.Articles, dbs.Users, NewUnauthenticatedClient(server.URL), slog.Default())
	assert.NilError(t, sync.syncAnnotations(ctx, "did:test:u1"))

	anns, err := dbs.Articles.ListAnnotations(ctx, "", "", "", 100, 0)
	assert.NilError(t, err)
	assert.Equal(t, len(anns), 1, "mirrored margin note should not produce a duplicate row")
	if len(anns) == 1 {
		assert.Equal(t, anns[0].URI, "at://did:test:u1/at.glean.annotation/rkey1")
	}
}

func TestSyncAnnotations_KeepsExternalMarginNote(t *testing.T) {
	ctx := context.Background()
	dbs := setupSyncTestDB(t)

	assert.NilError(t, dbs.Articles.UpsertFeed(ctx, &db.Feed{FeedURL: "https://a.com/feed"}))

	now := time.Now().Format(time.RFC3339)
	marginRec := NewMarginNoteRecord("https://a.com/1", "some quote", "some note", nil, now)
	marginValue, err := json.Marshal(marginRec)
	assert.NilError(t, err)

	server := newSyncTestServer(t, map[string][]Record{
		CollectionAnnotation: {}, // no glean annotation
		CollectionMarginNote: {
			{URI: "at://did:test:u1/at.margin.note/rkey1", CID: "cid-m1", Value: marginValue},
		},
	})

	sync := NewSync(dbs.Articles, dbs.Users, NewUnauthenticatedClient(server.URL), slog.Default())
	assert.NilError(t, sync.syncAnnotations(ctx, "did:test:u1"))

	anns, err := dbs.Articles.ListAnnotations(ctx, "", "", "", 100, 0)
	assert.NilError(t, err)
	assert.Equal(t, len(anns), 1, "external margin note should be kept")
	assert.Equal(t, anns[0].URI, "at://did:test:u1/at.margin.note/rkey1")
}

func TestSyncAnnotations_DedupsNoteOnlyMarginNote(t *testing.T) {
	ctx := context.Background()
	dbs := setupSyncTestDB(t)

	assert.NilError(t, dbs.Articles.UpsertFeed(ctx, &db.Feed{FeedURL: "https://a.com/feed"}))

	now := time.Now().Format(time.RFC3339)
	annValue, err := json.Marshal(AnnotationRecord{
		CreatedAt: now, FeedURL: "https://a.com/feed", ArticleURL: "https://a.com/1",
		Note: "just a comment",
	})
	assert.NilError(t, err)

	marginRec := NewMarginNoteRecord("https://a.com/1", "", "just a comment", nil, now)
	marginValue, err := json.Marshal(marginRec)
	assert.NilError(t, err)

	server := newSyncTestServer(t, map[string][]Record{
		CollectionAnnotation: {
			{URI: "at://did:test:u1/at.glean.annotation/rkey1", CID: "cid-a1", Value: annValue},
		},
		CollectionMarginNote: {
			{URI: "at://did:test:u1/at.margin.note/rkey1", CID: "cid-m1", Value: marginValue},
		},
	})

	sync := NewSync(dbs.Articles, dbs.Users, NewUnauthenticatedClient(server.URL), slog.Default())
	assert.NilError(t, sync.syncAnnotations(ctx, "did:test:u1"))

	anns, err := dbs.Articles.ListAnnotations(ctx, "", "", "", 100, 0)
	assert.NilError(t, err)
	assert.Equal(t, len(anns), 1, "note-only mirrored margin note should not duplicate")
}

func TestDeleteMirroredMarginNote_DeletesMatchingContent(t *testing.T) {
	ctx := context.Background()
	now := time.Now().Format(time.RFC3339)

	matching := NewMarginNoteRecord("https://a.com/1", "selected text", "my note", nil, now)
	matchingValue, err := json.Marshal(matching)
	assert.NilError(t, err)
	other := NewMarginNoteRecord("https://a.com/2", "other", "other note", nil, now)
	otherValue, err := json.Marshal(other)
	assert.NilError(t, err)

	var deleted []string
	mux := http.NewServeMux()
	mux.HandleFunc("/xrpc/com.atproto.repo.listRecords", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"records": []rawRecord{
			{URI: "at://did:test:u1/at.margin.note/match", CID: "c1", Value: matchingValue},
			{URI: "at://did:test:u1/at.margin.note/other", CID: "c2", Value: otherValue},
		}})
	})
	mux.HandleFunc("/xrpc/com.atproto.repo.deleteRecord", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		deleted = append(deleted, body["rkey"].(string))
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	client := NewUnauthenticatedClient(server.URL)
	err = DeleteMirroredMarginNotes(ctx, client, "did:test:u1", "https://a.com/1", "selected text", "my note")
	assert.NilError(t, err)

	assert.Equal(t, len(deleted), 1)
	assert.Equal(t, deleted[0], "match")
}

func TestDeleteMirroredMarginNote_NoMatchIsNoop(t *testing.T) {
	ctx := context.Background()
	now := time.Now().Format(time.RFC3339)

	other := NewMarginNoteRecord("https://a.com/2", "other", "other note", nil, now)
	otherValue, err := json.Marshal(other)
	assert.NilError(t, err)

	deleteCalled := false
	mux := http.NewServeMux()
	mux.HandleFunc("/xrpc/com.atproto.repo.listRecords", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"records": []rawRecord{
			{URI: "at://did:test:u1/at.margin.note/other", CID: "c2", Value: otherValue},
		}})
	})
	mux.HandleFunc("/xrpc/com.atproto.repo.deleteRecord", func(w http.ResponseWriter, r *http.Request) {
		deleteCalled = true
		w.WriteHeader(http.StatusOK)
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	client := NewUnauthenticatedClient(server.URL)
	err = DeleteMirroredMarginNotes(ctx, client, "did:test:u1", "https://a.com/1", "selected text", "my note")
	assert.NilError(t, err)
	assert.Equal(t, deleteCalled, false)
}
