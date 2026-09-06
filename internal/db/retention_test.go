package db

import (
	"context"
	"testing"
	"time"

	"gotest.tools/v3/assert"

	"pkg.rbrt.fr/glean/internal/feed"
)

func seedRetentionFixtures(t *testing.T, ctx context.Context, dbs *Store) {
	t.Helper()
	sql := dbs.SQLDB()

	// Known user + an unknown one whose rows must not survive purges.
	_, err := sql.ExecContext(ctx, `INSERT INTO users (did) VALUES ('did:test:known')`)
	assert.NilError(t, err)

	insertArticle := func(t *testing.T, id int64, feedURL, guid string, published time.Time) {
		t.Helper()
		_, err := sql.ExecContext(ctx, `
			INSERT INTO articles.articles (id, feed_url, guid, title, published)
			VALUES (?, ?, ?, 't', ?)`,
			id, feedURL, guid, published)
		assert.NilError(t, err)
	}

	now := time.Now()
	insertArticle(t, 1, "https://f.example/rss", "old-1", now.AddDate(0, 0, -60))
	insertArticle(t, 2, "https://f.example/rss", "fresh-2", now.AddDate(0, 0, -5))

	for _, tc := range []struct {
		did       string
		articleID int64
	}{
		{"did:test:known", 2},
		{"did:test:unknown", 1},
	} {
		_, err = sql.ExecContext(ctx,
			`INSERT INTO articles.read_state (user_did, article_id, is_read) VALUES (?, ?, 1)`,
			tc.did, tc.articleID)
		assert.NilError(t, err)
	}

	fixtures := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO articles.likes (uri, author_did, feed_url, article_url, created_at) VALUES (?, ?, 'f', 'a', CURRENT_TIMESTAMP)`, []any{"at://did:test:unknown/at.glean.like/1", "did:test:unknown"}},
		{`INSERT INTO articles.annotations (uri, author_did, feed_url, article_url, created_at) VALUES (?, ?, 'f', 'a', CURRENT_TIMESTAMP)`, []any{"at://did:test:unknown/at.glean.annotation/1", "did:test:unknown"}},
		{`INSERT INTO articles.subscriptions (user_did, feed_url) VALUES (?, 'f')`, []any{"did:test:unknown"}},
		{`INSERT INTO follows (user_did, target_did) VALUES (?, 'did:test:known')`, []any{"did:test:unknown"}},
		{`INSERT INTO recommendation_impressions (user_did, target_type, target_id) VALUES (?, 'feed', 'f')`, []any{"did:test:unknown"}},
		{`INSERT INTO dismissed_recommendations (user_did, target_type, target_id) VALUES (?, 'feed', 'f')`, []any{"did:test:unknown"}},
		{`INSERT INTO articles.likes (uri, author_did, feed_url, article_url, created_at) VALUES (?, 'did:test:known', 'f', 'a', CURRENT_TIMESTAMP)`, []any{"at://did:test:known/at.glean.like/1"}},
	}
	for _, f := range fixtures {
		_, err = sql.ExecContext(ctx, f.query, f.args...)
		assert.NilError(t, err)
	}
}

func count(t *testing.T, ctx context.Context, dbs *Store, query string, args ...any) int64 {
	t.Helper()
	var n int64
	err := dbs.SQLDB().QueryRowContext(ctx, query, args...).Scan(&n)
	assert.NilError(t, err)
	return n
}

func TestPurgeUnknownUserRows_RemovesOnlyUnknownUserRows(t *testing.T) {
	ctx := context.Background()
	dbs := setupTestDB(t)
	seedRetentionFixtures(t, ctx, dbs)

	stats, err := dbs.PurgeUnknownUserRows(ctx)
	assert.NilError(t, err)
	assert.Equal(t, int64(7), stats.Total())

	assert.Equal(t, int64(0), count(t, ctx, dbs, `SELECT COUNT(*) FROM articles.likes WHERE author_did = 'did:test:unknown'`))
	assert.Equal(t, int64(1), count(t, ctx, dbs, `SELECT COUNT(*) FROM articles.likes WHERE author_did = 'did:test:known'`))
	assert.Equal(t, int64(0), count(t, ctx, dbs, `SELECT COUNT(*) FROM articles.annotations WHERE author_did = 'did:test:unknown'`))
	assert.Equal(t, int64(0), count(t, ctx, dbs, `SELECT COUNT(*) FROM articles.subscriptions WHERE user_did = 'did:test:unknown'`))
	assert.Equal(t, int64(0), count(t, ctx, dbs, `SELECT COUNT(*) FROM follows WHERE user_did = 'did:test:unknown'`))
	assert.Equal(t, int64(0), count(t, ctx, dbs, `SELECT COUNT(*) FROM recommendation_impressions WHERE user_did = 'did:test:unknown'`))
	assert.Equal(t, int64(0), count(t, ctx, dbs, `SELECT COUNT(*) FROM dismissed_recommendations WHERE user_did = 'did:test:unknown'`))
	// Read state of known users survives.
	assert.Equal(t, int64(1), count(t, ctx, dbs, `SELECT COUNT(*) FROM articles.read_state WHERE user_did = 'did:test:known'`))
}

func TestPurgeExpiredArticles_DeletesOldArticlesAndTheirReadState(t *testing.T) {
	ctx := context.Background()
	dbs := setupTestDB(t)
	seedRetentionFixtures(t, ctx, dbs)

	n, err := dbs.PurgeExpiredArticles(ctx, 30)
	assert.NilError(t, err)
	assert.Equal(t, int64(1), n)

	assert.Equal(t, int64(0), count(t, ctx, dbs, `SELECT COUNT(*) FROM articles.articles WHERE guid = 'old-1'`))
	assert.Equal(t, int64(1), count(t, ctx, dbs, `SELECT COUNT(*) FROM articles.articles WHERE guid = 'fresh-2'`))
	// The deleted article's read state goes with it; the recent one stays.
	assert.Equal(t, int64(0), count(t, ctx, dbs, `SELECT COUNT(*) FROM articles.read_state r JOIN articles.articles a ON a.id = r.article_id WHERE a.guid = 'old-1'`))
	assert.Equal(t, int64(1), count(t, ctx, dbs, `SELECT COUNT(*) FROM articles.read_state WHERE article_id = 2`))
}

func TestPurgeExpiredArticles_ZeroDaysDisables(t *testing.T) {
	ctx := context.Background()
	dbs := setupTestDB(t)
	seedRetentionFixtures(t, ctx, dbs)

	n, err := dbs.PurgeExpiredArticles(ctx, 0)
	assert.NilError(t, err)
	assert.Equal(t, int64(0), n)
	assert.Equal(t, int64(2), count(t, ctx, dbs, `SELECT COUNT(*) FROM articles.articles`))
}

func TestReclaimSpace_RunsClean(t *testing.T) {
	ctx := context.Background()
	dbs := setupTestDB(t)
	seedRetentionFixtures(t, ctx, dbs)

	assert.NilError(t, dbs.ReclaimSpace(ctx))
}

func TestRunMaintenance_AppliesArticleRetention(t *testing.T) {
	ctx := context.Background()
	dbs := setupTestDB(t)
	seedRetentionFixtures(t, ctx, dbs)

	assert.NilError(t, dbs.RunMaintenance(ctx, 90, 30))
	assert.Equal(t, int64(1), count(t, ctx, dbs, `SELECT COUNT(*) FROM articles.articles`))
}

// A feed keeps serving entries past the retention cutoff, so a purged article
// comes back under a fresh surrogate id. Read state must come back with it.
func TestPurgeThenReingest_KeepsArticleRead(t *testing.T) {
	ctx := context.Background()
	dbs := setupTestDB(t)
	sqlDB := dbs.SQLDB()

	const did = "did:test:known"
	_, err := sqlDB.ExecContext(ctx, `INSERT INTO users (did) VALUES (?)`, did)
	assert.NilError(t, err)

	published := time.Now().AddDate(0, 0, -60)
	_, err = sqlDB.ExecContext(ctx, `
		INSERT INTO articles.articles (id, feed_url, guid, title, published)
		VALUES (1, 'https://f.example/rss', 'weekly-92', 't', ?)`, published)
	assert.NilError(t, err)
	assert.NilError(t, dbs.Articles.MarkArticleRead(ctx, did, 1))

	n, err := dbs.PurgeExpiredArticles(ctx, 30)
	assert.NilError(t, err)
	assert.Equal(t, int64(1), n)

	// The feed still lists it; ingest with no cutoff configured re-adds it.
	assert.NilError(t, dbs.Articles.BatchUpsertArticles(ctx, []feed.Article{{
		FeedURL:   "https://f.example/rss",
		GUID:      "weekly-92",
		Title:     "t",
		Published: published,
	}}))

	var id int64
	var isRead bool
	err = sqlDB.QueryRowContext(ctx, `
		SELECT a.id, COALESCE(r.is_read, 0)
		FROM articles.articles a
		LEFT JOIN articles.read_state r ON r.user_did = ? AND r.article_id = a.id
		WHERE a.guid = 'weekly-92'`, did).Scan(&id, &isRead)
	assert.NilError(t, err)
	assert.Assert(t, id != 1, "re-ingested article should have a new surrogate id")
	assert.Equal(t, true, isRead, "read state must survive purge and re-ingest")
}

func TestBatchUpsertArticles_SkipsArticlesPastRetention(t *testing.T) {
	ctx := context.Background()
	dbs := setupTestDB(t)
	dbs.Articles.SetRetentionDays(30)

	now := time.Now()
	assert.NilError(t, dbs.Articles.BatchUpsertArticles(ctx, []feed.Article{
		{FeedURL: "https://f.example/rss", GUID: "old", Title: "t", Published: now.AddDate(0, 0, -60)},
		{FeedURL: "https://f.example/rss", GUID: "fresh", Title: "t", Published: now.AddDate(0, 0, -5)},
		{FeedURL: "https://f.example/rss", GUID: "undated", Title: "t"},
	}))

	assert.Equal(t, int64(0), count(t, ctx, dbs, `SELECT COUNT(*) FROM articles.articles WHERE guid = 'old'`))
	assert.Equal(t, int64(1), count(t, ctx, dbs, `SELECT COUNT(*) FROM articles.articles WHERE guid = 'fresh'`))
	// Undated entries have no age to judge, so they are still ingested.
	assert.Equal(t, int64(1), count(t, ctx, dbs, `SELECT COUNT(*) FROM articles.articles WHERE guid = 'undated'`))
}
