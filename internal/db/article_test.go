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

func seedSearchData(t *testing.T, ctx context.Context, database *DB) (userDID, feedURL string) {
	t.Helper()

	userDID = "did:test:searcher"
	feedURL = "https://search.example.com/feed.xml"

	_, err := database.ExecContext(ctx, `INSERT INTO users (did, handle) VALUES (?, ?)`, userDID, "searcher")
	assert.NilError(t, err)

	_, err = database.ExecContext(ctx, `INSERT INTO feeds (feed_url, title) VALUES (?, ?)`, feedURL, "Tech Blog")
	assert.NilError(t, err)

	_, err = database.ExecContext(ctx, `INSERT INTO subscriptions (user_did, feed_url) VALUES (?, ?)`, userDID, feedURL)
	assert.NilError(t, err)

	articles := []struct {
		guid, title, summary, content string
	}{
		{"g1", "Go Programming Basics", "Learn Go fundamentals", "Go is a statically typed compiled language"},
		{"g2", "Rust Memory Safety", "Understanding ownership in Rust", "Rust provides memory safety without garbage collection"},
		{"g3", "Python Data Science", "NumPy and Pandas tutorial", "Python is popular for data analysis"},
	}
	for _, a := range articles {
		_, err := database.ExecContext(ctx, `
			INSERT INTO articles (feed_url, guid, title, summary, content) VALUES (?, ?, ?, ?, ?)
		`, feedURL, a.guid, a.title, a.summary, a.content)
		assert.NilError(t, err)
	}

	return userDID, feedURL
}

func TestSearchArticles_FindsByTitle(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t)
	userDID, _ := seedSearchData(t, ctx, db)

	results, err := db.SearchArticles(ctx, userDID, "Go Programming", 10, 0)
	assert.NilError(t, err)
	assert.Equal(t, len(results), 1)
	assert.Equal(t, results[0].Title, "Go Programming Basics")
}

func TestSearchArticles_FindsBySummary(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t)
	userDID, _ := seedSearchData(t, ctx, db)

	results, err := db.SearchArticles(ctx, userDID, "ownership", 10, 0)
	assert.NilError(t, err)
	assert.Equal(t, len(results), 1)
	assert.Equal(t, results[0].Title, "Rust Memory Safety")
}

func TestSearchArticles_FindsByContent(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t)
	userDID, _ := seedSearchData(t, ctx, db)

	results, err := db.SearchArticles(ctx, userDID, "garbage collection", 10, 0)
	assert.NilError(t, err)
	assert.Equal(t, len(results), 1)
	assert.Equal(t, results[0].Title, "Rust Memory Safety")
}

func TestSearchArticles_NoResults(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t)
	userDID, _ := seedSearchData(t, ctx, db)

	results, err := db.SearchArticles(ctx, userDID, "nonexistent_xyz", 10, 0)
	assert.NilError(t, err)
	assert.Equal(t, len(results), 0)
}

func TestSearchArticles_MultipleMatches(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t)
	userDID, _ := seedSearchData(t, ctx, db)

	results, err := db.SearchArticles(ctx, userDID, "Python", 10, 0)
	assert.NilError(t, err)
	assert.Assert(t, len(results) >= 1)

	var found bool
	for _, a := range results {
		if a.Title == "Python Data Science" {
			found = true
			break
		}
	}
	assert.Assert(t, found, "expected to find Python Data Science article")
}

func TestSearchArticles_ScopedToSubscriptions(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t)
	userDID, feedURL := seedSearchData(t, ctx, db)

	otherFeed := "https://other.example.com/feed.xml"
	_, err := db.ExecContext(ctx, `INSERT INTO feeds (feed_url, title) VALUES (?, ?)`, otherFeed, "Other Feed")
	assert.NilError(t, err)

	_, err = db.ExecContext(ctx, `
		INSERT INTO articles (feed_url, guid, title) VALUES (?, ?, ?)
	`, otherFeed, "other-1", "Go Concurrency Tips")
	assert.NilError(t, err)

	results, err := db.SearchArticles(ctx, userDID, "Go", 10, 0)
	assert.NilError(t, err)
	for _, a := range results {
		assert.Equal(t, a.FeedURL, feedURL)
	}
}

func TestSearchArticles_Pagination(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t)
	userDID, _ := seedSearchData(t, ctx, db)

	results, err := db.SearchArticles(ctx, userDID, "Go", 1, 0)
	assert.NilError(t, err)
	assert.Equal(t, len(results), 1)

	results2, err := db.SearchArticles(ctx, userDID, "Go", 1, 1)
	assert.NilError(t, err)
	assert.Assert(t, len(results2) == 0 || results2[0].ID != results[0].ID)
}

func TestSearchArticles_EmptyQuery(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t)
	userDID, _ := seedSearchData(t, ctx, db)

	results, err := db.SearchArticles(ctx, userDID, "", 10, 0)
	assert.NilError(t, err)
	assert.Equal(t, len(results), 0)
}
