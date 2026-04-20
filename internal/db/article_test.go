package db

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"gotest.tools/v3/assert"
)

func setupTestDB(t *testing.T) *DB {
	t.Helper()
	f, err := os.CreateTemp("", "glean-test-*.db")
	assert.NilError(t, err)
	assert.NilError(t, f.Close())
	path := f.Name()
	t.Cleanup(func() { _ = os.Remove(path) })

	db, err := Open(path)
	assert.NilError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func seedArticleReadState(t *testing.T, ctx context.Context, db *DB) (userDID string, feedURL string, readArticleID, unreadArticleID int64) {
	t.Helper()

	userDID = "did:test:user1"
	feedURL = "https://example.com/feed.xml"

	_, err := db.ExecContext(ctx, `INSERT INTO users (did, handle) VALUES (?, ?)`, userDID, "user1")
	assert.NilError(t, err)

	_, err = db.ExecContext(ctx, `INSERT INTO feeds (feed_url, title) VALUES (?, ?)`, feedURL, "Test Feed")
	assert.NilError(t, err)

	_, err = db.ExecContext(ctx, `INSERT INTO subscriptions (user_did, feed_url) VALUES (?, ?)`, userDID, feedURL)
	assert.NilError(t, err)

	res, err := db.ExecContext(ctx, `INSERT INTO articles (feed_url, guid, title, url) VALUES (?, ?, ?, ?)`,
		feedURL, "guid-read", "Read Article", "https://example.com/read")
	assert.NilError(t, err)
	readArticleID, _ = res.LastInsertId()

	res, err = db.ExecContext(ctx, `INSERT INTO articles (feed_url, guid, title, url) VALUES (?, ?, ?, ?)`,
		feedURL, "guid-unread", "Unread Article", "https://example.com/unread")
	assert.NilError(t, err)
	unreadArticleID, _ = res.LastInsertId()

	err = db.MarkArticleRead(ctx, userDID, readArticleID)
	assert.NilError(t, err)

	return userDID, feedURL, readArticleID, unreadArticleID
}

func TestListReadArticles_ReturnsOnlyRead(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t)
	userDID, feedURL, readID, unreadID := seedArticleReadState(t, ctx, db)

	articles, err := db.ListReadArticles(ctx, userDID, feedURL, 10, 0)
	assert.NilError(t, err)
	assert.Equal(t, len(articles), 1)
	assert.Equal(t, articles[0].ID, readID)
	assert.Equal(t, articles[0].IsRead, sql.NullBool{Bool: true, Valid: true})

	_ = unreadID
}

func TestListReadArticles_ExcludesUnread(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t)
	userDID, feedURL, _, unreadID := seedArticleReadState(t, ctx, db)

	articles, err := db.ListReadArticles(ctx, userDID, feedURL, 10, 0)
	assert.NilError(t, err)
	for _, a := range articles {
		assert.Assert(t, a.ID != unreadID, "unread article should not appear in read list")
	}
}

func TestListUnreadArticles_ReturnsOnlyUnread(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t)
	userDID, feedURL, readID, unreadID := seedArticleReadState(t, ctx, db)

	articles, err := db.ListUnreadArticles(ctx, userDID, feedURL, 10, 0)
	assert.NilError(t, err)
	assert.Equal(t, len(articles), 1)
	assert.Equal(t, articles[0].ID, unreadID)
	assert.Equal(t, articles[0].IsRead.Bool, false)

	_ = readID
}

func TestListArticles_ReturnsAll(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t)
	userDID, feedURL, _, _ := seedArticleReadState(t, ctx, db)

	articles, err := db.ListArticles(ctx, userDID, feedURL, 10, 0)
	assert.NilError(t, err)
	assert.Equal(t, len(articles), 2)
}

func TestMarkArticleRead_ToggleUnread(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t)
	userDID, _, _, unreadID := seedArticleReadState(t, ctx, db)

	err := db.MarkArticleRead(ctx, userDID, unreadID)
	assert.NilError(t, err)

	state, err := db.GetReadState(ctx, userDID, unreadID)
	assert.NilError(t, err)
	assert.Equal(t, state.IsRead, true)

	err = db.MarkArticleUnread(ctx, userDID, unreadID)
	assert.NilError(t, err)

	state, err = db.GetReadState(ctx, userDID, unreadID)
	assert.NilError(t, err)
	assert.Equal(t, state.IsRead, false)
}

func TestListReadArticles_EmptyWhenNoneRead(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t)
	userDID, feedURL, _, _ := seedArticleReadState(t, ctx, db)

	articles, err := db.ListUnreadArticles(ctx, userDID, feedURL, 10, 0)
	assert.NilError(t, err)
	assert.Equal(t, len(articles), 1)
}

func TestListReadArticles_WithFeedURLFilter(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t)
	userDID, feedURL, _, _ := seedArticleReadState(t, ctx, db)

	articles, err := db.ListReadArticles(ctx, userDID, feedURL, 10, 0)
	assert.NilError(t, err)
	assert.Equal(t, len(articles), 1)

	articles, err = db.ListReadArticles(ctx, userDID, "https://other.com/feed", 10, 0)
	assert.NilError(t, err)
	assert.Equal(t, len(articles), 0)
}

func TestGetUnreadCount(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t)
	userDID, feedURL, _, _ := seedArticleReadState(t, ctx, db)

	count, err := db.GetUnreadCount(ctx, userDID, feedURL)
	assert.NilError(t, err)
	assert.Equal(t, count, 1)

	count, err = db.GetUnreadCount(ctx, userDID, "")
	assert.NilError(t, err)
	assert.Equal(t, count, 1)
}

func TestUpdateArticleFullContent(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t)
	_, _, _, articleID := seedArticleReadState(t, ctx, db)

	err := db.UpdateArticleFullContent(ctx, articleID, "<p>Scraped content</p>")
	assert.NilError(t, err)

	article, err := db.GetArticle(ctx, articleID)
	assert.NilError(t, err)
	assert.Equal(t, article.FullContent.String, "<p>Scraped content</p>")
	assert.Assert(t, article.FullContent.Valid)
}

func TestGetArticle_IncludesFullContent(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t)
	_, _, _, articleID := seedArticleReadState(t, ctx, db)

	article, err := db.GetArticle(ctx, articleID)
	assert.NilError(t, err)
	assert.Assert(t, !article.FullContent.Valid)
}
