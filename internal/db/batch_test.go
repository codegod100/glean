package db

import (
	"context"
	"testing"
	"time"

	"gotest.tools/v3/assert"
)

func TestBatchCreateUsers_InsertsAll(t *testing.T) {
	ctx := context.Background()
	dbs := setupTestDB(t)

	err := dbs.Users.BatchCreateUsers(ctx, []string{"did:test:u1", "did:test:u2"})
	assert.NilError(t, err)

	u1, err := dbs.Users.GetUser(ctx, "did:test:u1")
	assert.NilError(t, err)
	assert.Equal(t, u1.DID, "did:test:u1")

	u2, err := dbs.Users.GetUser(ctx, "did:test:u2")
	assert.NilError(t, err)
	assert.Equal(t, u2.DID, "did:test:u2")
}

func TestBatchCreateUsers_IgnoresExisting(t *testing.T) {
	ctx := context.Background()
	dbs := setupTestDB(t)

	_, err := dbs.Users.CreateUser(ctx, "did:test:u1")
	assert.NilError(t, err)

	err = dbs.Users.BatchCreateUsers(ctx, []string{"did:test:u1"})
	assert.NilError(t, err)

	u, err := dbs.Users.GetUser(ctx, "did:test:u1")
	assert.NilError(t, err)
	assert.Equal(t, u.DID, "did:test:u1")
}

func TestBatchCreateUsers_Empty(t *testing.T) {
	ctx := context.Background()
	dbs := setupTestDB(t)

	err := dbs.Users.BatchCreateUsers(ctx, nil)
	assert.NilError(t, err)
}

func seedSubscriptionData(t *testing.T, ctx context.Context, dbs *Store) (userDID string) {
	t.Helper()
	userDID = "did:test:subuser"
	_, err := dbs.SQLDB().ExecContext(ctx, `INSERT INTO users (did) VALUES (?)`, userDID)
	assert.NilError(t, err)
	return userDID
}

func TestBatchUpsertFeeds_InsertsAll(t *testing.T) {
	ctx := context.Background()
	dbs := setupTestDB(t)

	feeds := []*Feed{
		{FeedURL: "https://a.com/feed.xml", Title: NullStr("Feed A")},
		{FeedURL: "https://b.com/feed.xml", Title: NullStr("Feed B")},
	}
	err := dbs.Articles.BatchUpsertFeeds(ctx, feeds)
	assert.NilError(t, err)

	f, err := dbs.Articles.GetFeed(ctx, "https://a.com/feed.xml")
	assert.NilError(t, err)
	assert.Equal(t, f.Title.String, "Feed A")

	f, err = dbs.Articles.GetFeed(ctx, "https://b.com/feed.xml")
	assert.NilError(t, err)
	assert.Equal(t, f.Title.String, "Feed B")
}

func TestBatchUpsertFeeds_UpdatesExisting(t *testing.T) {
	ctx := context.Background()
	dbs := setupTestDB(t)

	err := dbs.Articles.UpsertFeed(ctx, &Feed{FeedURL: "https://a.com/feed.xml", Title: NullStr("Old Title")})
	assert.NilError(t, err)

	feeds := []*Feed{
		{FeedURL: "https://a.com/feed.xml", Title: NullStr("New Title")},
	}
	err = dbs.Articles.BatchUpsertFeeds(ctx, feeds)
	assert.NilError(t, err)

	f, err := dbs.Articles.GetFeed(ctx, "https://a.com/feed.xml")
	assert.NilError(t, err)
	assert.Equal(t, f.Title.String, "New Title")
}

func TestBatchReconcileSubscriptions_CreatesNew(t *testing.T) {
	ctx := context.Background()
	dbs := setupTestDB(t)
	userDID := seedSubscriptionData(t, ctx, dbs)

	_ = dbs.Articles.UpsertFeed(ctx, &Feed{FeedURL: "https://a.com/feed.xml", Title: NullStr("Feed A")})
	_ = dbs.Articles.UpsertFeed(ctx, &Feed{FeedURL: "https://b.com/feed.xml", Title: NullStr("Feed B")})

	subs := []SubData{
		{FeedURL: "https://a.com/feed.xml", Title: "Feed A", URI: "at://uri1", CID: "cid1"},
		{FeedURL: "https://b.com/feed.xml", Title: "Feed B", URI: "at://uri2", CID: "cid2"},
	}
	err := dbs.Articles.BatchReconcileSubscriptions(ctx, userDID, subs)
	assert.NilError(t, err)

	subs2, err := dbs.Articles.ListSubscriptions(ctx, userDID, "", 10, 0)
	assert.NilError(t, err)
	assert.Equal(t, len(subs2), 2)

	f, err := dbs.Articles.GetFeed(ctx, "https://a.com/feed.xml")
	assert.NilError(t, err)
	assert.Equal(t, f.SubscriberCount, 1)
}

func TestBatchReconcileSubscriptions_BackfillsURI(t *testing.T) {
	ctx := context.Background()
	dbs := setupTestDB(t)
	userDID := seedSubscriptionData(t, ctx, dbs)

	_ = dbs.Articles.UpsertFeed(ctx, &Feed{FeedURL: "https://a.com/feed.xml"})

	err := dbs.Articles.CreateSubscription(ctx, userDID, "https://a.com/feed.xml", "Feed A", "", "", "")
	assert.NilError(t, err)

	subs := []SubData{
		{FeedURL: "https://a.com/feed.xml", URI: "at://new-uri", CID: "new-cid"},
	}
	err = dbs.Articles.BatchReconcileSubscriptions(ctx, userDID, subs)
	assert.NilError(t, err)

	s, err := dbs.Articles.GetSubscription(ctx, userDID, "https://a.com/feed.xml")
	assert.NilError(t, err)
	assert.Equal(t, s.URI.String, "at://new-uri")

	f, err := dbs.Articles.GetFeed(ctx, "https://a.com/feed.xml")
	assert.NilError(t, err)
	assert.Equal(t, f.SubscriberCount, 1)
}

func TestBatchReconcileSubscriptions_SkipsExistingWithURI(t *testing.T) {
	ctx := context.Background()
	dbs := setupTestDB(t)
	userDID := seedSubscriptionData(t, ctx, dbs)

	_ = dbs.Articles.UpsertFeed(ctx, &Feed{FeedURL: "https://a.com/feed.xml"})
	err := dbs.Articles.CreateSubscription(ctx, userDID, "https://a.com/feed.xml", "Feed A", "", "at://existing", "cid")
	assert.NilError(t, err)

	subs := []SubData{
		{FeedURL: "https://a.com/feed.xml", URI: "at://different", CID: "cid2"},
	}
	err = dbs.Articles.BatchReconcileSubscriptions(ctx, userDID, subs)
	assert.NilError(t, err)

	s, err := dbs.Articles.GetSubscription(ctx, userDID, "https://a.com/feed.xml")
	assert.NilError(t, err)
	assert.Equal(t, s.URI.String, "at://existing")
}

func TestBatchReconcileSubscriptions_UpdatesCategoryAndTitle(t *testing.T) {
	ctx := context.Background()
	dbs := setupTestDB(t)
	userDID := seedSubscriptionData(t, ctx, dbs)

	_ = dbs.Articles.UpsertFeed(ctx, &Feed{FeedURL: "https://a.com/feed.xml", Title: NullStr("Feed A")})
	err := dbs.Articles.CreateSubscription(ctx, userDID, "https://a.com/feed.xml", "Feed A", "old-cat", "at://existing", "cid")
	assert.NilError(t, err)

	subs := []SubData{
		{FeedURL: "https://a.com/feed.xml", Title: "New Title", Category: "new-cat", URI: "at://existing", CID: "cid"},
	}
	err = dbs.Articles.BatchReconcileSubscriptions(ctx, userDID, subs)
	assert.NilError(t, err)

	s, err := dbs.Articles.GetSubscription(ctx, userDID, "https://a.com/feed.xml")
	assert.NilError(t, err)
	assert.Equal(t, s.Category.String, "new-cat")
	assert.Equal(t, s.FeedTitle, "New Title")

	f, err := dbs.Articles.GetFeed(ctx, "https://a.com/feed.xml")
	assert.NilError(t, err)
	assert.Equal(t, f.SubscriberCount, 1)
}

func TestCreateSubscription_UpdatesCategoryAndTitle(t *testing.T) {
	ctx := context.Background()
	dbs := setupTestDB(t)
	userDID := seedSubscriptionData(t, ctx, dbs)

	_ = dbs.Articles.UpsertFeed(ctx, &Feed{FeedURL: "https://a.com/feed.xml", Title: NullStr("Feed A")})
	err := dbs.Articles.CreateSubscription(ctx, userDID, "https://a.com/feed.xml", "Feed A", "old-cat", "at://existing", "cid")
	assert.NilError(t, err)

	err = dbs.Articles.CreateSubscription(ctx, userDID, "https://a.com/feed.xml", "New Title", "new-cat", "at://existing", "cid2")
	assert.NilError(t, err)

	s, err := dbs.Articles.GetSubscription(ctx, userDID, "https://a.com/feed.xml")
	assert.NilError(t, err)
	assert.Equal(t, s.Category.String, "new-cat")
	assert.Equal(t, s.FeedTitle, "New Title")
	assert.Equal(t, s.CID.String, "cid2")
}

func TestCreateSubscription_ReturnsDuplicateWhenUnchanged(t *testing.T) {
	ctx := context.Background()
	dbs := setupTestDB(t)
	userDID := seedSubscriptionData(t, ctx, dbs)

	_ = dbs.Articles.UpsertFeed(ctx, &Feed{FeedURL: "https://a.com/feed.xml", Title: NullStr("Feed A")})
	err := dbs.Articles.CreateSubscription(ctx, userDID, "https://a.com/feed.xml", "Feed A", "cat", "at://existing", "cid")
	assert.NilError(t, err)

	err = dbs.Articles.CreateSubscription(ctx, userDID, "https://a.com/feed.xml", "Feed A", "cat", "at://existing", "cid")
	assert.Equal(t, err, ErrDuplicateSubscription)
}

func TestBatchCreateLikes_InsertsAll(t *testing.T) {
	ctx := context.Background()
	dbs := setupTestDB(t)

	now := NullTime(time.Now())
	likes := []*Like{
		{URI: "at://like1", AuthorDID: "did:test:u1", FeedURL: "https://a.com/feed", ArticleURL: "https://a.com/1", CreatedAt: now, CID: NullStr("cid1")},
		{URI: "at://like2", AuthorDID: "did:test:u1", FeedURL: "https://a.com/feed", ArticleURL: "https://a.com/2", CreatedAt: now, CID: NullStr("cid2")},
	}
	err := dbs.Articles.BatchCreateLikes(ctx, likes)
	assert.NilError(t, err)

	exists, err := dbs.Articles.HasLiked(ctx, "did:test:u1", "https://a.com/feed", "https://a.com/1")
	assert.NilError(t, err)
	assert.Equal(t, exists, true)

	exists, err = dbs.Articles.HasLiked(ctx, "did:test:u1", "https://a.com/feed", "https://a.com/2")
	assert.NilError(t, err)
	assert.Equal(t, exists, true)
}

func TestBatchCreateLikes_IgnoresDuplicates(t *testing.T) {
	ctx := context.Background()
	dbs := setupTestDB(t)

	now := NullTime(time.Now())
	likes := []*Like{
		{URI: "at://like1", AuthorDID: "did:test:u1", FeedURL: "https://a.com/feed", ArticleURL: "https://a.com/1", CreatedAt: now},
	}
	err := dbs.Articles.BatchCreateLikes(ctx, likes)
	assert.NilError(t, err)

	likes = append(likes, &Like{URI: "at://like1", AuthorDID: "did:test:u1", FeedURL: "https://a.com/feed", ArticleURL: "https://a.com/1", CreatedAt: now})
	err = dbs.Articles.BatchCreateLikes(ctx, likes)
	assert.NilError(t, err)
}

func TestBatchCreateAnnotations_InsertsAll(t *testing.T) {
	ctx := context.Background()
	dbs := setupTestDB(t)

	now := NullTime(time.Now())
	annotations := []*Annotation{
		{URI: "at://ann1", AuthorDID: "did:test:u1", FeedURL: "https://a.com/feed", ArticleURL: "https://a.com/1", Note: NullStr("Great"), CreatedAt: now},
		{URI: "at://ann2", AuthorDID: "did:test:u1", FeedURL: "https://a.com/feed", ArticleURL: "https://a.com/2", Note: NullStr("Nice"), CreatedAt: now},
	}
	err := dbs.Articles.BatchCreateAnnotations(ctx, annotations)
	assert.NilError(t, err)

	exists, err := dbs.Articles.AnnotationExists(ctx, "at://ann1")
	assert.NilError(t, err)
	assert.Equal(t, exists, true)

	exists, err = dbs.Articles.AnnotationExists(ctx, "at://ann2")
	assert.NilError(t, err)
	assert.Equal(t, exists, true)
}

func TestBatchCreateAnnotations_IgnoresDuplicates(t *testing.T) {
	ctx := context.Background()
	dbs := setupTestDB(t)

	now := NullTime(time.Now())
	annotations := []*Annotation{
		{URI: "at://ann1", AuthorDID: "did:test:u1", FeedURL: "https://a.com/feed", ArticleURL: "https://a.com/1", CreatedAt: now},
	}
	err := dbs.Articles.BatchCreateAnnotations(ctx, annotations)
	assert.NilError(t, err)

	annotations = append(annotations, &Annotation{URI: "at://ann1", AuthorDID: "did:test:u1", FeedURL: "https://a.com/feed", ArticleURL: "https://a.com/1", CreatedAt: now})
	err = dbs.Articles.BatchCreateAnnotations(ctx, annotations)
	assert.NilError(t, err)
}
