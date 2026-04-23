package db

import (
	"context"
	"testing"
	"time"

	"gotest.tools/v3/assert"
)

func TestBatchCreateUsers_InsertsAll(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t)

	users := []UserData{
		{DID: "did:test:u1", Handle: "user1", DisplayName: "User One", AvatarURL: "https://avatar1.png"},
		{DID: "did:test:u2", Handle: "user2", DisplayName: "User Two"},
	}
	err := db.BatchCreateUsers(ctx, users)
	assert.NilError(t, err)

	u1, err := db.GetUser(ctx, "did:test:u1")
	assert.NilError(t, err)
	assert.Equal(t, u1.Handle, "user1")
	assert.Equal(t, u1.DisplayName.String, "User One")

	u2, err := db.GetUser(ctx, "did:test:u2")
	assert.NilError(t, err)
	assert.Equal(t, u2.Handle, "user2")
	assert.Equal(t, u2.DisplayName.String, "User Two")
}

func TestBatchCreateUsers_UpsertsExisting(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t)

	_, err := db.CreateUser(ctx, "did:test:u1", "old-handle", "", "")
	assert.NilError(t, err)

	users := []UserData{
		{DID: "did:test:u1", Handle: "new-handle", DisplayName: "New Name"},
	}
	err = db.BatchCreateUsers(ctx, users)
	assert.NilError(t, err)

	u, err := db.GetUser(ctx, "did:test:u1")
	assert.NilError(t, err)
	assert.Equal(t, u.Handle, "new-handle")
	assert.Equal(t, u.DisplayName.String, "New Name")
}

func TestBatchCreateUsers_Empty(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t)

	err := db.BatchCreateUsers(ctx, nil)
	assert.NilError(t, err)
}

func TestBatchCreateUsers_DoesNotOverwriteWithEmpty(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t)

	_, err := db.CreateUser(ctx, "did:test:u1", "handle", "Existing Name", "https://avatar.png")
	assert.NilError(t, err)

	users := []UserData{
		{DID: "did:test:u1", Handle: "", DisplayName: "", AvatarURL: ""},
	}
	err = db.BatchCreateUsers(ctx, users)
	assert.NilError(t, err)

	u, err := db.GetUser(ctx, "did:test:u1")
	assert.NilError(t, err)
	assert.Equal(t, u.Handle, "handle")
	assert.Equal(t, u.DisplayName.String, "Existing Name")
	assert.Equal(t, u.AvatarURL.String, "https://avatar.png")
}

func seedSubscriptionData(t *testing.T, ctx context.Context, database *DB) (userDID string) {
	t.Helper()
	userDID = "did:test:subuser"
	_, err := database.ExecContext(ctx, `INSERT INTO users (did, handle) VALUES (?, ?)`, userDID, "subuser")
	assert.NilError(t, err)
	return userDID
}

func TestBatchUpsertFeeds_InsertsAll(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t)

	feeds := []*Feed{
		{FeedURL: "https://a.com/feed.xml", Title: NullStr("Feed A")},
		{FeedURL: "https://b.com/feed.xml", Title: NullStr("Feed B")},
	}
	err := db.BatchUpsertFeeds(ctx, feeds)
	assert.NilError(t, err)

	f, err := db.GetFeed(ctx, "https://a.com/feed.xml")
	assert.NilError(t, err)
	assert.Equal(t, f.Title.String, "Feed A")

	f, err = db.GetFeed(ctx, "https://b.com/feed.xml")
	assert.NilError(t, err)
	assert.Equal(t, f.Title.String, "Feed B")
}

func TestBatchUpsertFeeds_UpdatesExisting(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t)

	err := db.UpsertFeed(ctx, &Feed{FeedURL: "https://a.com/feed.xml", Title: NullStr("Old Title")})
	assert.NilError(t, err)

	feeds := []*Feed{
		{FeedURL: "https://a.com/feed.xml", Title: NullStr("New Title")},
	}
	err = db.BatchUpsertFeeds(ctx, feeds)
	assert.NilError(t, err)

	f, err := db.GetFeed(ctx, "https://a.com/feed.xml")
	assert.NilError(t, err)
	assert.Equal(t, f.Title.String, "New Title")
}

func TestBatchReconcileSubscriptions_CreatesNew(t *testing.T) {
	ctx := context.Background()
	database := setupTestDB(t)
	userDID := seedSubscriptionData(t, ctx, database)

	_ = database.UpsertFeed(ctx, &Feed{FeedURL: "https://a.com/feed.xml", Title: NullStr("Feed A")})
	_ = database.UpsertFeed(ctx, &Feed{FeedURL: "https://b.com/feed.xml", Title: NullStr("Feed B")})

	subs := []SubData{
		{FeedURL: "https://a.com/feed.xml", Title: "Feed A", URI: "at://uri1", CID: "cid1"},
		{FeedURL: "https://b.com/feed.xml", Title: "Feed B", URI: "at://uri2", CID: "cid2"},
	}
	err := database.BatchReconcileSubscriptions(ctx, userDID, subs)
	assert.NilError(t, err)

	subs2, err := database.ListSubscriptions(ctx, userDID, "", 10, 0)
	assert.NilError(t, err)
	assert.Equal(t, len(subs2), 2)

	f, err := database.GetFeed(ctx, "https://a.com/feed.xml")
	assert.NilError(t, err)
	assert.Equal(t, f.SubscriberCount, 1)
}

func TestBatchReconcileSubscriptions_BackfillsURI(t *testing.T) {
	ctx := context.Background()
	database := setupTestDB(t)
	userDID := seedSubscriptionData(t, ctx, database)

	_ = database.UpsertFeed(ctx, &Feed{FeedURL: "https://a.com/feed.xml"})

	err := database.CreateSubscription(ctx, userDID, "https://a.com/feed.xml", "Feed A", "", "", "")
	assert.NilError(t, err)

	subs := []SubData{
		{FeedURL: "https://a.com/feed.xml", URI: "at://new-uri", CID: "new-cid"},
	}
	err = database.BatchReconcileSubscriptions(ctx, userDID, subs)
	assert.NilError(t, err)

	s, err := database.GetSubscription(ctx, userDID, "https://a.com/feed.xml")
	assert.NilError(t, err)
	assert.Equal(t, s.URI.String, "at://new-uri")

	f, err := database.GetFeed(ctx, "https://a.com/feed.xml")
	assert.NilError(t, err)
	assert.Equal(t, f.SubscriberCount, 1)
}

func TestBatchReconcileSubscriptions_SkipsExistingWithURI(t *testing.T) {
	ctx := context.Background()
	database := setupTestDB(t)
	userDID := seedSubscriptionData(t, ctx, database)

	_ = database.UpsertFeed(ctx, &Feed{FeedURL: "https://a.com/feed.xml"})
	err := database.CreateSubscription(ctx, userDID, "https://a.com/feed.xml", "Feed A", "", "at://existing", "cid")
	assert.NilError(t, err)

	subs := []SubData{
		{FeedURL: "https://a.com/feed.xml", URI: "at://different", CID: "cid2"},
	}
	err = database.BatchReconcileSubscriptions(ctx, userDID, subs)
	assert.NilError(t, err)

	s, err := database.GetSubscription(ctx, userDID, "https://a.com/feed.xml")
	assert.NilError(t, err)
	assert.Equal(t, s.URI.String, "at://existing")
}

func TestBatchCreateLikes_InsertsAll(t *testing.T) {
	ctx := context.Background()
	database := setupTestDB(t)

	now := NullTime(time.Now())
	likes := []*Like{
		{URI: "at://like1", AuthorDID: "did:test:u1", FeedURL: "https://a.com/feed", ArticleURL: "https://a.com/1", CreatedAt: now, CID: NullStr("cid1")},
		{URI: "at://like2", AuthorDID: "did:test:u1", FeedURL: "https://a.com/feed", ArticleURL: "https://a.com/2", CreatedAt: now, CID: NullStr("cid2")},
	}
	err := database.BatchCreateLikes(ctx, likes)
	assert.NilError(t, err)

	exists, err := database.HasLiked(ctx, "did:test:u1", "https://a.com/feed", "https://a.com/1")
	assert.NilError(t, err)
	assert.Equal(t, exists, true)

	exists, err = database.HasLiked(ctx, "did:test:u1", "https://a.com/feed", "https://a.com/2")
	assert.NilError(t, err)
	assert.Equal(t, exists, true)
}

func TestBatchCreateLikes_IgnoresDuplicates(t *testing.T) {
	ctx := context.Background()
	database := setupTestDB(t)

	now := NullTime(time.Now())
	likes := []*Like{
		{URI: "at://like1", AuthorDID: "did:test:u1", FeedURL: "https://a.com/feed", ArticleURL: "https://a.com/1", CreatedAt: now},
	}
	err := database.BatchCreateLikes(ctx, likes)
	assert.NilError(t, err)

	likes = append(likes, &Like{URI: "at://like1", AuthorDID: "did:test:u1", FeedURL: "https://a.com/feed", ArticleURL: "https://a.com/1", CreatedAt: now})
	err = database.BatchCreateLikes(ctx, likes)
	assert.NilError(t, err)
}

func TestBatchCreateAnnotations_InsertsAll(t *testing.T) {
	ctx := context.Background()
	database := setupTestDB(t)

	now := NullTime(time.Now())
	annotations := []*Annotation{
		{URI: "at://ann1", AuthorDID: "did:test:u1", FeedURL: "https://a.com/feed", ArticleURL: "https://a.com/1", Note: NullStr("Great"), CreatedAt: now},
		{URI: "at://ann2", AuthorDID: "did:test:u1", FeedURL: "https://a.com/feed", ArticleURL: "https://a.com/2", Note: NullStr("Nice"), CreatedAt: now},
	}
	err := database.BatchCreateAnnotations(ctx, annotations)
	assert.NilError(t, err)

	exists, err := database.AnnotationExists(ctx, "at://ann1")
	assert.NilError(t, err)
	assert.Equal(t, exists, true)

	exists, err = database.AnnotationExists(ctx, "at://ann2")
	assert.NilError(t, err)
	assert.Equal(t, exists, true)
}

func TestBatchCreateAnnotations_IgnoresDuplicates(t *testing.T) {
	ctx := context.Background()
	database := setupTestDB(t)

	now := NullTime(time.Now())
	annotations := []*Annotation{
		{URI: "at://ann1", AuthorDID: "did:test:u1", FeedURL: "https://a.com/feed", ArticleURL: "https://a.com/1", CreatedAt: now},
	}
	err := database.BatchCreateAnnotations(ctx, annotations)
	assert.NilError(t, err)

	annotations = append(annotations, &Annotation{URI: "at://ann1", AuthorDID: "did:test:u1", FeedURL: "https://a.com/feed", ArticleURL: "https://a.com/1", CreatedAt: now})
	err = database.BatchCreateAnnotations(ctx, annotations)
	assert.NilError(t, err)
}
