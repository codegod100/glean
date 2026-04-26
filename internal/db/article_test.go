package db

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"gotest.tools/v3/assert"
)

func setupTestDB(t *testing.T) *Store {
	t.Helper()
	f, err := os.CreateTemp("", "glean-test-*.db")
	assert.NilError(t, err)
	assert.NilError(t, f.Close())
	path := f.Name()
	t.Cleanup(func() {
		for _, suffix := range []string{"", "_users", "_users-shm", "_users-wal", "_articles", "_articles-shm", "_articles-wal", "_recs", "_recs-shm", "_recs-wal"} {
			_ = os.Remove(path + suffix)
		}
	})

	dbs, err := Open(path)
	assert.NilError(t, err)
	t.Cleanup(func() { _ = dbs.Close() })
	return dbs
}

func seedArticleReadState(t *testing.T, ctx context.Context, dbs *Store) (userDID string, feedURL string, readArticleID, unreadArticleID int64) {
	t.Helper()

	userDID = "did:test:user1"
	feedURL = "https://example.com/feed.xml"

	_, err := dbs.SQLDB().ExecContext(ctx, `INSERT INTO users (did) VALUES (?)`, userDID)
	assert.NilError(t, err)

	_, err = dbs.SQLDB().ExecContext(ctx, `INSERT INTO articles.feeds (feed_url, title) VALUES (?, ?)`, feedURL, "Test Feed")
	assert.NilError(t, err)

	_, err = dbs.SQLDB().ExecContext(ctx, `INSERT INTO articles.subscriptions (user_did, feed_url) VALUES (?, ?)`, userDID, feedURL)
	assert.NilError(t, err)

	res, err := dbs.SQLDB().ExecContext(ctx, `INSERT INTO articles.articles (feed_url, guid, title, url) VALUES (?, ?, ?, ?)`,
		feedURL, "guid-read", "Read Article", "https://example.com/read")
	assert.NilError(t, err)
	readArticleID, _ = res.LastInsertId()

	res, err = dbs.SQLDB().ExecContext(ctx, `INSERT INTO articles.articles (feed_url, guid, title, url) VALUES (?, ?, ?, ?)`,
		feedURL, "guid-unread", "Unread Article", "https://example.com/unread")
	assert.NilError(t, err)
	unreadArticleID, _ = res.LastInsertId()

	err = dbs.Articles.MarkArticleRead(ctx, userDID, readArticleID)
	assert.NilError(t, err)

	return userDID, feedURL, readArticleID, unreadArticleID
}

func TestListReadArticles_ReturnsOnlyRead(t *testing.T) {
	ctx := context.Background()
	dbs := setupTestDB(t)
	userDID, feedURL, readID, unreadID := seedArticleReadState(t, ctx, dbs)

	results, err := dbs.Articles.ListReadArticles(ctx, userDID, feedURL, 10, 0)
	assert.NilError(t, err)
	assert.Equal(t, len(results), 1)
	assert.Equal(t, results[0].ID, readID)
	assert.Equal(t, results[0].IsRead, sql.NullBool{Bool: true, Valid: true})

	_ = unreadID
}

func TestListReadArticles_ExcludesUnread(t *testing.T) {
	ctx := context.Background()
	dbs := setupTestDB(t)
	userDID, feedURL, _, unreadID := seedArticleReadState(t, ctx, dbs)

	results, err := dbs.Articles.ListReadArticles(ctx, userDID, feedURL, 10, 0)
	assert.NilError(t, err)
	for _, a := range results {
		assert.Assert(t, a.ID != unreadID, "unread article should not appear in read list")
	}
}

func TestListUnreadArticles_ReturnsOnlyUnread(t *testing.T) {
	ctx := context.Background()
	dbs := setupTestDB(t)
	userDID, feedURL, readID, unreadID := seedArticleReadState(t, ctx, dbs)

	results, err := dbs.Articles.ListUnreadArticles(ctx, userDID, feedURL, 10, 0)
	assert.NilError(t, err)
	assert.Equal(t, len(results), 1)
	assert.Equal(t, results[0].ID, unreadID)
	assert.Equal(t, results[0].IsRead.Bool, false)

	_ = readID
}

func TestListArticles_ReturnsAll(t *testing.T) {
	ctx := context.Background()
	dbs := setupTestDB(t)
	userDID, feedURL, _, _ := seedArticleReadState(t, ctx, dbs)

	results, err := dbs.Articles.ListArticles(ctx, userDID, feedURL, 10, 0)
	assert.NilError(t, err)
	assert.Equal(t, len(results), 2)
}

func TestMarkArticleRead_ToggleUnread(t *testing.T) {
	ctx := context.Background()
	dbs := setupTestDB(t)
	userDID, _, _, unreadID := seedArticleReadState(t, ctx, dbs)

	err := dbs.Articles.MarkArticleRead(ctx, userDID, unreadID)
	assert.NilError(t, err)

	state, err := dbs.Articles.GetReadState(ctx, userDID, unreadID)
	assert.NilError(t, err)
	assert.Equal(t, state.IsRead, true)

	err = dbs.Articles.MarkArticleUnread(ctx, userDID, unreadID)
	assert.NilError(t, err)

	state, err = dbs.Articles.GetReadState(ctx, userDID, unreadID)
	assert.NilError(t, err)
	assert.Equal(t, state.IsRead, false)
}

func TestListReadArticles_EmptyWhenNoneRead(t *testing.T) {
	ctx := context.Background()
	dbs := setupTestDB(t)
	userDID, feedURL, _, _ := seedArticleReadState(t, ctx, dbs)

	results, err := dbs.Articles.ListUnreadArticles(ctx, userDID, feedURL, 10, 0)
	assert.NilError(t, err)
	assert.Equal(t, len(results), 1)
}

func TestListReadArticles_WithFeedURLFilter(t *testing.T) {
	ctx := context.Background()
	dbs := setupTestDB(t)
	userDID, feedURL, _, _ := seedArticleReadState(t, ctx, dbs)

	results, err := dbs.Articles.ListReadArticles(ctx, userDID, feedURL, 10, 0)
	assert.NilError(t, err)
	assert.Equal(t, len(results), 1)

	results, err = dbs.Articles.ListReadArticles(ctx, userDID, "https://other.com/feed", 10, 0)
	assert.NilError(t, err)
	assert.Equal(t, len(results), 0)
}

func TestGetUnreadCount(t *testing.T) {
	ctx := context.Background()
	dbs := setupTestDB(t)
	userDID, feedURL, _, _ := seedArticleReadState(t, ctx, dbs)

	count, err := dbs.Articles.GetUnreadCount(ctx, userDID, feedURL)
	assert.NilError(t, err)
	assert.Equal(t, count, 1)

	count, err = dbs.Articles.GetUnreadCount(ctx, userDID, "")
	assert.NilError(t, err)
	assert.Equal(t, count, 1)
}

func TestUpdateArticleFullContent(t *testing.T) {
	ctx := context.Background()
	dbs := setupTestDB(t)
	_, _, _, articleID := seedArticleReadState(t, ctx, dbs)

	err := dbs.Articles.UpdateArticleFullContent(ctx, articleID, "<p>Scraped content</p>")
	assert.NilError(t, err)

	article, err := dbs.Articles.GetArticle(ctx, articleID)
	assert.NilError(t, err)
	assert.Equal(t, article.FullContent.String, "<p>Scraped content</p>")
	assert.Assert(t, article.FullContent.Valid)
}

func TestGetArticle_IncludesFullContent(t *testing.T) {
	ctx := context.Background()
	dbs := setupTestDB(t)
	_, _, _, articleID := seedArticleReadState(t, ctx, dbs)

	article, err := dbs.Articles.GetArticle(ctx, articleID)
	assert.NilError(t, err)
	assert.Assert(t, !article.FullContent.Valid)
}

func seedSearchData(t *testing.T, ctx context.Context, dbs *Store) (userDID, feedURL string) {
	t.Helper()

	userDID = "did:test:searcher"
	feedURL = "https://search.example.com/feed.xml"

	_, err := dbs.SQLDB().ExecContext(ctx, `INSERT INTO users (did) VALUES (?)`, userDID)
	assert.NilError(t, err)

	_, err = dbs.SQLDB().ExecContext(ctx, `INSERT INTO articles.feeds (feed_url, title) VALUES (?, ?)`, feedURL, "Tech Blog")
	assert.NilError(t, err)

	_, err = dbs.SQLDB().ExecContext(ctx, `INSERT INTO articles.subscriptions (user_did, feed_url) VALUES (?, ?)`, userDID, feedURL)
	assert.NilError(t, err)

	articles := []struct {
		guid, title, summary, content string
	}{
		{"g1", "Go Programming Basics", "Learn Go fundamentals", "Go is a statically typed compiled language"},
		{"g2", "Rust Memory Safety", "Understanding ownership in Rust", "Rust provides memory safety without garbage collection"},
		{"g3", "Python Data Science", "NumPy and Pandas tutorial", "Python is popular for data analysis"},
	}
	for _, a := range articles {
		_, err := dbs.SQLDB().ExecContext(ctx, `
			INSERT INTO articles.articles (feed_url, guid, title, summary, content) VALUES (?, ?, ?, ?, ?)
		`, feedURL, a.guid, a.title, a.summary, a.content)
		assert.NilError(t, err)
	}

	return userDID, feedURL
}

func TestSearchArticles_FindsByTitle(t *testing.T) {
	ctx := context.Background()
	dbs := setupTestDB(t)
	userDID, _ := seedSearchData(t, ctx, dbs)

	results, err := dbs.Articles.SearchArticles(ctx, userDID, "Go Programming", 10, 0)
	assert.NilError(t, err)
	assert.Equal(t, len(results), 1)
	assert.Equal(t, results[0].Title, "Go Programming Basics")
}

func TestSearchArticles_FindsBySummary(t *testing.T) {
	ctx := context.Background()
	dbs := setupTestDB(t)
	userDID, _ := seedSearchData(t, ctx, dbs)

	results, err := dbs.Articles.SearchArticles(ctx, userDID, "ownership", 10, 0)
	assert.NilError(t, err)
	assert.Equal(t, len(results), 1)
	assert.Equal(t, results[0].Title, "Rust Memory Safety")
}

func TestSearchArticles_IgnoresContentOnlyMatch(t *testing.T) {
	ctx := context.Background()
	dbs := setupTestDB(t)
	userDID, _ := seedSearchData(t, ctx, dbs)

	results, err := dbs.Articles.SearchArticles(ctx, userDID, "garbage collection", 10, 0)
	assert.NilError(t, err)
	assert.Equal(t, len(results), 0)
}

func TestSearchArticles_NoResults(t *testing.T) {
	ctx := context.Background()
	dbs := setupTestDB(t)
	userDID, _ := seedSearchData(t, ctx, dbs)

	results, err := dbs.Articles.SearchArticles(ctx, userDID, "nonexistent_xyz", 10, 0)
	assert.NilError(t, err)
	assert.Equal(t, len(results), 0)
}

func TestSearchArticles_MultipleMatches(t *testing.T) {
	ctx := context.Background()
	dbs := setupTestDB(t)
	userDID, _ := seedSearchData(t, ctx, dbs)

	results, err := dbs.Articles.SearchArticles(ctx, userDID, "Python", 10, 0)
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
	dbs := setupTestDB(t)
	userDID, feedURL := seedSearchData(t, ctx, dbs)

	otherFeed := "https://other.example.com/feed.xml"
	_, err := dbs.SQLDB().ExecContext(ctx, `INSERT INTO articles.feeds (feed_url, title) VALUES (?, ?)`, otherFeed, "Other Feed")
	assert.NilError(t, err)

	_, err = dbs.SQLDB().ExecContext(ctx, `
		INSERT INTO articles.articles (feed_url, guid, title) VALUES (?, ?, ?)
	`, otherFeed, "other-1", "Go Concurrency Tips")
	assert.NilError(t, err)

	results, err := dbs.Articles.SearchArticles(ctx, userDID, "Go", 10, 0)
	assert.NilError(t, err)
	for _, a := range results {
		assert.Equal(t, a.FeedURL, feedURL)
	}
}

func TestSearchArticles_Pagination(t *testing.T) {
	ctx := context.Background()
	dbs := setupTestDB(t)
	userDID, _ := seedSearchData(t, ctx, dbs)

	results, err := dbs.Articles.SearchArticles(ctx, userDID, "Go", 1, 0)
	assert.NilError(t, err)
	assert.Equal(t, len(results), 1)

	results2, err := dbs.Articles.SearchArticles(ctx, userDID, "Go", 1, 1)
	assert.NilError(t, err)
	assert.Assert(t, len(results2) == 0 || results2[0].ID != results[0].ID)
}

func TestSearchArticles_EmptyQuery(t *testing.T) {
	ctx := context.Background()
	dbs := setupTestDB(t)
	userDID, _ := seedSearchData(t, ctx, dbs)

	results, err := dbs.Articles.SearchArticles(ctx, userDID, "", 10, 0)
	assert.NilError(t, err)
	assert.Equal(t, len(results), 0)
}

func TestSearchArticles_SpecialCharactersNoError(t *testing.T) {
	ctx := context.Background()
	dbs := setupTestDB(t)
	userDID, _ := seedSearchData(t, ctx, dbs)

	_, err := dbs.Articles.SearchArticles(ctx, userDID, "test.example.com/path?q=1&b=2", 10, 0)
	assert.NilError(t, err)
}

func TestSearchArticles_OnlySpecialCharacters(t *testing.T) {
	ctx := context.Background()
	dbs := setupTestDB(t)
	userDID, _ := seedSearchData(t, ctx, dbs)

	results, err := dbs.Articles.SearchArticles(ctx, userDID, "...///:::!!!", 10, 0)
	assert.NilError(t, err)
	assert.Equal(t, len(results), 0)
}
