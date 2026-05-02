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
	assert.NilError(t, dbs.Articles.RecountSubscriberCounts(ctx))

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
	assert.NilError(t, dbs.Articles.RecountSubscriberCounts(ctx))

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
	assert.NilError(t, dbs.Articles.RecountSubscriberCounts(ctx))

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

func TestBatchReconcileSubscriptions_DeletesOrphaned(t *testing.T) {
	ctx := context.Background()
	dbs := setupTestDB(t)
	userDID := seedSubscriptionData(t, ctx, dbs)

	_ = dbs.Articles.UpsertFeed(ctx, &Feed{FeedURL: "https://a.com/feed.xml", Title: NullStr("A")})
	_ = dbs.Articles.UpsertFeed(ctx, &Feed{FeedURL: "https://b.com/feed.xml", Title: NullStr("B")})
	_ = dbs.Articles.UpsertFeed(ctx, &Feed{FeedURL: "https://c.com/feed.xml", Title: NullStr("C")})

	subs := []SubData{
		{FeedURL: "https://a.com/feed.xml", Title: "A", URI: "at://uri-a", CID: "cid-a"},
		{FeedURL: "https://b.com/feed.xml", Title: "B", URI: "at://uri-b", CID: "cid-b"},
		{FeedURL: "https://c.com/feed.xml", Title: "C", URI: "at://uri-c", CID: "cid-c"},
	}
	err := dbs.Articles.BatchReconcileSubscriptions(ctx, userDID, subs)
	assert.NilError(t, err)

	err = dbs.Articles.DeleteOrphanedSubscriptions(ctx, userDID, map[string]bool{
		"https://a.com/feed.xml": true,
		"https://c.com/feed.xml": true,
	})
	assert.NilError(t, err)
	assert.NilError(t, dbs.Articles.RecountSubscriberCounts(ctx))

	list, err := dbs.Articles.ListSubscriptions(ctx, userDID, "", 10, 0)
	assert.NilError(t, err)
	assert.Equal(t, len(list), 2)

	feedURLs := map[string]bool{}
	for _, s := range list {
		feedURLs[s.FeedURL] = true
	}
	assert.Assert(t, feedURLs["https://a.com/feed.xml"])
	assert.Assert(t, feedURLs["https://c.com/feed.xml"])

	f, err := dbs.Articles.GetFeed(ctx, "https://b.com/feed.xml")
	assert.NilError(t, err)
	assert.Equal(t, f.SubscriberCount, 0)
}

func TestBatchReconcileSubscriptions_PreservesLocalOnly(t *testing.T) {
	ctx := context.Background()
	dbs := setupTestDB(t)
	userDID := seedSubscriptionData(t, ctx, dbs)

	_ = dbs.Articles.UpsertFeed(ctx, &Feed{FeedURL: "https://a.com/feed.xml"})
	_ = dbs.Articles.UpsertFeed(ctx, &Feed{FeedURL: "https://b.com/feed.xml"})

	err := dbs.Articles.CreateSubscription(ctx, userDID, "https://a.com/feed.xml", "A", "", "at://uri-a", "cid")
	assert.NilError(t, err)
	err = dbs.Articles.CreateSubscription(ctx, userDID, "https://b.com/feed.xml", "B", "", "", "")
	assert.NilError(t, err)

	err = dbs.Articles.DeleteOrphanedSubscriptions(ctx, userDID, map[string]bool{
		"https://a.com/feed.xml": true,
	})
	assert.NilError(t, err)

	list, err := dbs.Articles.ListSubscriptions(ctx, userDID, "", 10, 0)
	assert.NilError(t, err)
	assert.Equal(t, len(list), 2)
}

func TestBatchReconcileSubscriptions_DeletesAllWhenEmpty(t *testing.T) {
	ctx := context.Background()
	dbs := setupTestDB(t)
	userDID := seedSubscriptionData(t, ctx, dbs)

	_ = dbs.Articles.UpsertFeed(ctx, &Feed{FeedURL: "https://a.com/feed.xml"})
	_ = dbs.Articles.UpsertFeed(ctx, &Feed{FeedURL: "https://b.com/feed.xml"})

	subs := []SubData{
		{FeedURL: "https://a.com/feed.xml", URI: "at://uri-a", CID: "cid-a"},
		{FeedURL: "https://b.com/feed.xml", URI: "at://uri-b", CID: "cid-b"},
	}
	err := dbs.Articles.BatchReconcileSubscriptions(ctx, userDID, subs)
	assert.NilError(t, err)

	err = dbs.Articles.DeleteOrphanedSubscriptions(ctx, userDID, map[string]bool{})
	assert.NilError(t, err)

	list, err := dbs.Articles.ListSubscriptions(ctx, userDID, "", 10, 0)
	assert.NilError(t, err)
	assert.Equal(t, len(list), 0)
}

func TestListSubscriptionsWithoutURI_ReturnsOnlyLocal(t *testing.T) {
	ctx := context.Background()
	dbs := setupTestDB(t)
	userDID := seedSubscriptionData(t, ctx, dbs)

	_ = dbs.Articles.UpsertFeed(ctx, &Feed{FeedURL: "https://a.com/feed.xml"})
	_ = dbs.Articles.UpsertFeed(ctx, &Feed{FeedURL: "https://b.com/feed.xml"})
	_ = dbs.Articles.UpsertFeed(ctx, &Feed{FeedURL: "https://c.com/feed.xml"})

	assert.NilError(t, dbs.Articles.CreateSubscription(ctx, userDID, "https://a.com/feed.xml", "A", "cat", "", ""))
	assert.NilError(t, dbs.Articles.CreateSubscription(ctx, userDID, "https://b.com/feed.xml", "B", "", "at://uri-b", "cid"))
	assert.NilError(t, dbs.Articles.CreateSubscription(ctx, userDID, "https://c.com/feed.xml", "C", "other", "", ""))

	subs, err := dbs.Articles.ListSubscriptionsWithoutURI(ctx, userDID)
	assert.NilError(t, err)
	assert.Equal(t, len(subs), 2)

	urls := map[string]bool{}
	for _, s := range subs {
		urls[s.FeedURL] = true
	}
	assert.Assert(t, urls["https://a.com/feed.xml"])
	assert.Assert(t, urls["https://c.com/feed.xml"])

	var aSub, cSub SubData
	for _, s := range subs {
		if s.FeedURL == "https://a.com/feed.xml" {
			aSub = s
		}
		if s.FeedURL == "https://c.com/feed.xml" {
			cSub = s
		}
	}
	assert.Equal(t, aSub.Title, "A")
	assert.Equal(t, aSub.Category, "cat")
	assert.Equal(t, cSub.Title, "C")
	assert.Equal(t, cSub.Category, "other")
}

func TestListSubscriptionsWithoutURI_EmptyWhenAllHaveURI(t *testing.T) {
	ctx := context.Background()
	dbs := setupTestDB(t)
	userDID := seedSubscriptionData(t, ctx, dbs)

	_ = dbs.Articles.UpsertFeed(ctx, &Feed{FeedURL: "https://a.com/feed.xml"})
	assert.NilError(t, dbs.Articles.CreateSubscription(ctx, userDID, "https://a.com/feed.xml", "A", "", "at://uri", "cid"))

	subs, err := dbs.Articles.ListSubscriptionsWithoutURI(ctx, userDID)
	assert.NilError(t, err)
	assert.Equal(t, len(subs), 0)
}

func TestUpdateSubscriptionURI_UpdatesSubscription(t *testing.T) {
	ctx := context.Background()
	dbs := setupTestDB(t)
	userDID := seedSubscriptionData(t, ctx, dbs)

	_ = dbs.Articles.UpsertFeed(ctx, &Feed{FeedURL: "https://a.com/feed.xml"})
	assert.NilError(t, dbs.Articles.CreateSubscription(ctx, userDID, "https://a.com/feed.xml", "A", "cat", "", ""))

	assert.NilError(t, dbs.Articles.UpdateSubscriptionURI(ctx, userDID, "https://a.com/feed.xml", "at://new-uri", "new-cid"))

	s, err := dbs.Articles.GetSubscription(ctx, userDID, "https://a.com/feed.xml")
	assert.NilError(t, err)
	assert.Equal(t, s.URI.String, "at://new-uri")
	assert.Equal(t, s.CID.String, "new-cid")
	assert.Equal(t, s.FeedTitle, "A")
	assert.Equal(t, s.Category.String, "cat")
}

func TestUpdateSubscriptionURI_NoOverwriteIfExists(t *testing.T) {
	ctx := context.Background()
	dbs := setupTestDB(t)
	userDID := seedSubscriptionData(t, ctx, dbs)

	_ = dbs.Articles.UpsertFeed(ctx, &Feed{FeedURL: "https://a.com/feed.xml"})
	assert.NilError(t, dbs.Articles.CreateSubscription(ctx, userDID, "https://a.com/feed.xml", "A", "", "at://original", "cid1"))

	assert.NilError(t, dbs.Articles.UpdateSubscriptionURI(ctx, userDID, "https://a.com/feed.xml", "at://new-uri", "cid2"))

	s, err := dbs.Articles.GetSubscription(ctx, userDID, "https://a.com/feed.xml")
	assert.NilError(t, err)
	assert.Equal(t, s.URI.String, "at://new-uri")
	assert.Equal(t, s.CID.String, "cid2")
}

func TestDeleteOrphanedLikes_RemovesOrphaned(t *testing.T) {
	ctx := context.Background()
	dbs := setupTestDB(t)

	now := NullTime(time.Now())
	likes := []*Like{
		{URI: "at://like1", AuthorDID: "did:test:u1", FeedURL: "https://a.com/feed", ArticleURL: "https://a.com/1", CreatedAt: now, CID: NullStr("cid1")},
		{URI: "at://like2", AuthorDID: "did:test:u1", FeedURL: "https://a.com/feed", ArticleURL: "https://a.com/2", CreatedAt: now, CID: NullStr("cid2")},
	}
	err := dbs.Articles.BatchCreateLikes(ctx, likes)
	assert.NilError(t, err)

	err = dbs.Articles.DeleteOrphanedLikes(ctx, "did:test:u1", map[string]bool{"at://like1": true})
	assert.NilError(t, err)

	exists, err := dbs.Articles.HasLiked(ctx, "did:test:u1", "https://a.com/feed", "https://a.com/1")
	assert.NilError(t, err)
	assert.Equal(t, exists, true)

	exists, err = dbs.Articles.HasLiked(ctx, "did:test:u1", "https://a.com/feed", "https://a.com/2")
	assert.NilError(t, err)
	assert.Equal(t, exists, false)
}

func TestDeleteSubscription_NoDecrementWhenNotFound(t *testing.T) {
	ctx := context.Background()
	dbs := setupTestDB(t)
	userDID := seedSubscriptionData(t, ctx, dbs)

	_ = dbs.Articles.UpsertFeed(ctx, &Feed{FeedURL: "https://a.com/feed.xml"})
	assert.NilError(t, dbs.Articles.CreateSubscription(ctx, userDID, "https://a.com/feed.xml", "A", "", "at://uri", "cid"))

	assert.NilError(t, dbs.Articles.DeleteSubscription(ctx, userDID, "https://a.com/feed.xml"))

	assert.NilError(t, dbs.Articles.DeleteSubscription(ctx, userDID, "https://a.com/feed.xml"))
}

func TestRecountSubscriberCounts(t *testing.T) {
	ctx := context.Background()
	dbs := setupTestDB(t)
	user1 := seedSubscriptionData(t, ctx, dbs)

	_, err := dbs.SQLDB().ExecContext(ctx, `INSERT INTO users (did) VALUES (?)`, "did:test:u2")
	assert.NilError(t, err)

	_ = dbs.Articles.UpsertFeed(ctx, &Feed{FeedURL: "https://a.com/feed.xml"})
	_ = dbs.Articles.UpsertFeed(ctx, &Feed{FeedURL: "https://b.com/feed.xml"})

	assert.NilError(t, dbs.Articles.CreateSubscription(ctx, user1, "https://a.com/feed.xml", "A", "", "", ""))
	assert.NilError(t, dbs.Articles.CreateSubscription(ctx, user1, "https://b.com/feed.xml", "B", "", "", ""))
	assert.NilError(t, dbs.Articles.CreateSubscription(ctx, "did:test:u2", "https://a.com/feed.xml", "A", "", "", ""))

	_, err = dbs.SQLDB().ExecContext(ctx, `UPDATE articles.feeds SET subscriber_count = 99 WHERE feed_url = 'https://a.com/feed.xml'`)
	assert.NilError(t, err)

	assert.NilError(t, dbs.Articles.RecountSubscriberCounts(ctx))

	f, err := dbs.Articles.GetFeed(ctx, "https://a.com/feed.xml")
	assert.NilError(t, err)
	assert.Equal(t, f.SubscriberCount, 2)

	f, err = dbs.Articles.GetFeed(ctx, "https://b.com/feed.xml")
	assert.NilError(t, err)
	assert.Equal(t, f.SubscriberCount, 1)
}

func TestDeleteOrphanedAnnotations_RemovesOrphaned(t *testing.T) {
	ctx := context.Background()
	dbs := setupTestDB(t)

	now := NullTime(time.Now())
	annotations := []*Annotation{
		{URI: "at://ann1", AuthorDID: "did:test:u1", FeedURL: "https://a.com/feed", ArticleURL: "https://a.com/1", Note: NullStr("Keep"), CreatedAt: now},
		{URI: "at://ann2", AuthorDID: "did:test:u1", FeedURL: "https://a.com/feed", ArticleURL: "https://a.com/2", Note: NullStr("Remove"), CreatedAt: now},
	}
	err := dbs.Articles.BatchCreateAnnotations(ctx, annotations)
	assert.NilError(t, err)

	err = dbs.Articles.DeleteOrphanedAnnotations(ctx, "did:test:u1", map[string]bool{"at://ann1": true})
	assert.NilError(t, err)

	exists, err := dbs.Articles.AnnotationExists(ctx, "at://ann1")
	assert.NilError(t, err)
	assert.Equal(t, exists, true)

	exists, err = dbs.Articles.AnnotationExists(ctx, "at://ann2")
	assert.NilError(t, err)
	assert.Equal(t, exists, false)
}
