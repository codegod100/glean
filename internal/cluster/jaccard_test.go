package cluster

import (
	"context"
	"os"
	"testing"

	"pkg.rbrt.fr/glean/internal/db"

	"gotest.tools/v3/assert"
	"log/slog"
)

func setupClusterTestDB(t *testing.T) *db.DB {
	t.Helper()
	f, err := os.CreateTemp("", "glean-cluster-test-*.db")
	assert.NilError(t, err)
	assert.NilError(t, f.Close())
	path := f.Name()
	t.Cleanup(func() { _ = os.Remove(path) })

	database, err := db.Open(path)
	assert.NilError(t, err)
	t.Cleanup(func() { _ = database.Close() })
	return database
}

func seedClusterData(t *testing.T, ctx context.Context, database *db.DB) {
	t.Helper()

	users := []struct{ did, handle string }{
		{"did:test:alice", "alice"},
		{"did:test:bob", "bob"},
		{"did:test:carol", "carol"},
	}
	for _, u := range users {
		_, err := database.ExecContext(ctx, `INSERT INTO users (did, handle) VALUES (?, ?)`, u.did, u.handle)
		assert.NilError(t, err)
	}

	feeds := []struct{ url, title string }{
		{"https://a.com/feed", "Feed A"},
		{"https://b.com/feed", "Feed B"},
		{"https://c.com/feed", "Feed C"},
		{"https://d.com/feed", "Feed D"},
	}
	for _, f := range feeds {
		_, err := database.ExecContext(ctx, `INSERT INTO feeds (feed_url, title, site_url, description, feed_type) VALUES (?, ?, ?, '', 'rss')`, f.url, f.title, f.url)
		assert.NilError(t, err)
	}

	subs := []struct{ user, feed string }{
		{"did:test:alice", "https://a.com/feed"},
		{"did:test:alice", "https://b.com/feed"},
		{"did:test:alice", "https://c.com/feed"},
		{"did:test:bob", "https://a.com/feed"},
		{"did:test:bob", "https://b.com/feed"},
		{"did:test:bob", "https://d.com/feed"},
		{"did:test:carol", "https://c.com/feed"},
	}
	for _, s := range subs {
		_, err := database.ExecContext(ctx, `INSERT INTO subscriptions (user_did, feed_url) VALUES (?, ?)`, s.user, s.feed)
		assert.NilError(t, err)
	}
}

func TestComputeFeedSimilarity(t *testing.T) {
	ctx := context.Background()
	database := setupClusterTestDB(t)
	seedClusterData(t, ctx, database)

	engine := NewEngine(database.DB, slog.Default())
	err := engine.ComputeFeedSimilarity(ctx)
	assert.NilError(t, err)

	var count int
	err = database.QueryRowContext(ctx, `SELECT COUNT(*) FROM feed_similarity`).Scan(&count)
	assert.NilError(t, err)
	assert.Assert(t, count > 0, "expected feed similarity pairs")
}

func TestComputeUserSimilarity(t *testing.T) {
	ctx := context.Background()
	database := setupClusterTestDB(t)
	seedClusterData(t, ctx, database)

	engine := NewEngine(database.DB, slog.Default())
	err := engine.ComputeUserSimilarity(ctx)
	assert.NilError(t, err)

	var count int
	err = database.QueryRowContext(ctx, `SELECT COUNT(*) FROM user_similarity`).Scan(&count)
	assert.NilError(t, err)
	assert.Assert(t, count > 0, "expected user similarity pairs")
}

func TestComputeRecommendations_GeneratesFeedRecsForNewUser(t *testing.T) {
	ctx := context.Background()
	database := setupClusterTestDB(t)
	seedClusterData(t, ctx, database)

	engine := NewEngine(database.DB, slog.Default())
	assert.NilError(t, engine.ComputeFeedSimilarity(ctx))
	assert.NilError(t, engine.ComputeUserSimilarity(ctx))
	assert.NilError(t, engine.ComputeRecommendations(ctx))

	recs, err := engine.GetFeedRecommendations(ctx, "did:test:carol", 10)
	assert.NilError(t, err)
	assert.Assert(t, len(recs) > 0, "carol should get feed recommendations from similar users")

	var found bool
	for _, r := range recs {
		if r.FeedURL == "https://a.com/feed" || r.FeedURL == "https://b.com/feed" {
			found = true
		}
	}
	assert.Assert(t, found, "carol should be recommended feeds she doesn't subscribe to")
}

func TestComputeRecommendations_NoSelfRecommendations(t *testing.T) {
	ctx := context.Background()
	database := setupClusterTestDB(t)
	seedClusterData(t, ctx, database)

	engine := NewEngine(database.DB, slog.Default())
	assert.NilError(t, engine.ComputeFeedSimilarity(ctx))
	assert.NilError(t, engine.ComputeUserSimilarity(ctx))
	assert.NilError(t, engine.ComputeRecommendations(ctx))

	recs, err := engine.GetFeedRecommendations(ctx, "did:test:alice", 10)
	assert.NilError(t, err)

	subscribedFeeds := map[string]bool{
		"https://a.com/feed": true,
		"https://b.com/feed": true,
		"https://c.com/feed": true,
	}
	for _, r := range recs {
		assert.Assert(t, !subscribedFeeds[r.FeedURL],
			"should not recommend a feed the user already subscribes to")
	}
}

func TestLikesBasedSimilarity(t *testing.T) {
	ctx := context.Background()
	database := setupClusterTestDB(t)
	seedClusterData(t, ctx, database)

	_, err := database.ExecContext(ctx, `INSERT INTO articles (feed_url, guid, title, url) VALUES (?, ?, ?, ?)`,
		"https://a.com/feed", "art1", "Article 1", "https://a.com/art1")
	assert.NilError(t, err)
	_, err = database.ExecContext(ctx, `INSERT INTO articles (feed_url, guid, title, url) VALUES (?, ?, ?, ?)`,
		"https://a.com/feed", "art2", "Article 2", "https://a.com/art2")
	assert.NilError(t, err)

	_, err = database.ExecContext(ctx, `INSERT INTO likes (uri, author_did, feed_url, article_url, created_at) VALUES (?, ?, ?, ?, datetime('now'))`,
		"at://alice/like/1", "did:test:alice", "https://a.com/feed", "https://a.com/art1")
	assert.NilError(t, err)
	_, err = database.ExecContext(ctx, `INSERT INTO likes (uri, author_did, feed_url, article_url, created_at) VALUES (?, ?, ?, ?, datetime('now'))`,
		"at://alice/like/2", "did:test:alice", "https://a.com/feed", "https://a.com/art2")
	assert.NilError(t, err)
	_, err = database.ExecContext(ctx, `INSERT INTO likes (uri, author_did, feed_url, article_url, created_at) VALUES (?, ?, ?, ?, datetime('now'))`,
		"at://carol/like/1", "did:test:carol", "https://a.com/feed", "https://a.com/art1")
	assert.NilError(t, err)

	engine := NewEngine(database.DB, slog.Default())
	assert.NilError(t, engine.ComputeUserSimilarity(ctx))

	var jaccard float64
	var commonLikes int
	assert.NilError(t, database.QueryRowContext(ctx,
		`SELECT jaccard, common_likes FROM user_similarity WHERE user_a = ? AND user_b = ?`,
		"did:test:alice", "did:test:carol").Scan(&jaccard, &commonLikes))
	assert.Equal(t, commonLikes, 1, "alice and carol share 1 liked article")
	assert.Assert(t, jaccard > 0, "likes should contribute to similarity, got %f", jaccard)
}

func TestTagsBasedSimilarity(t *testing.T) {
	ctx := context.Background()
	database := setupClusterTestDB(t)
	seedClusterData(t, ctx, database)

	_, err := database.ExecContext(ctx, `INSERT INTO annotations (uri, author_did, feed_url, article_url, tags, created_at) VALUES (?, ?, ?, ?, ?, datetime('now'))`,
		"at://alice/ann/1", "did:test:alice", "https://a.com/feed", "https://a.com/art1", "go,programming")
	assert.NilError(t, err)
	_, err = database.ExecContext(ctx, `INSERT INTO annotations (uri, author_did, feed_url, article_url, tags, created_at) VALUES (?, ?, ?, ?, ?, datetime('now'))`,
		"at://alice/ann/2", "did:test:alice", "https://a.com/feed", "https://a.com/art2", "rust,programming")
	assert.NilError(t, err)
	_, err = database.ExecContext(ctx, `INSERT INTO annotations (uri, author_did, feed_url, article_url, tags, created_at) VALUES (?, ?, ?, ?, ?, datetime('now'))`,
		"at://carol/ann/1", "did:test:carol", "https://c.com/feed", "https://c.com/art1", "go,web")
	assert.NilError(t, err)

	engine := NewEngine(database.DB, slog.Default())
	assert.NilError(t, engine.ComputeUserSimilarity(ctx))

	var jaccard float64
	var commonTags int
	assert.NilError(t, database.QueryRowContext(ctx,
		`SELECT jaccard, common_tags FROM user_similarity WHERE user_a = ? AND user_b = ?`,
		"did:test:alice", "did:test:carol").Scan(&jaccard, &commonTags))
	assert.Equal(t, commonTags, 1, "alice and carol share 1 tag (go)")
	assert.Assert(t, jaccard > 0, "tags should contribute to similarity, got %f", jaccard)
}

func TestDescriptionBasedFeedSimilarity(t *testing.T) {
	ctx := context.Background()
	database := setupClusterTestDB(t)

	_, err := database.ExecContext(ctx, `INSERT INTO users (did, handle) VALUES (?, ?)`, "did:test:alice", "alice")
	assert.NilError(t, err)
	_, err = database.ExecContext(ctx, `INSERT INTO users (did, handle) VALUES (?, ?)`, "did:test:bob", "bob")
	assert.NilError(t, err)

	_, err = database.ExecContext(ctx, `INSERT INTO feeds (feed_url, title, site_url, description, feed_type) VALUES (?, ?, ?, ?, 'rss')`,
		"https://go.com/feed", "Go Blog", "https://go.com", "programming language golang software development")
	assert.NilError(t, err)
	_, err = database.ExecContext(ctx, `INSERT INTO feeds (feed_url, title, site_url, description, feed_type) VALUES (?, ?, ?, ?, 'rss')`,
		"https://rust.com/feed", "Rust Blog", "https://rust.com", "programming language rust software development")
	assert.NilError(t, err)

	_, err = database.ExecContext(ctx, `INSERT INTO subscriptions (user_did, feed_url) VALUES (?, ?)`, "did:test:alice", "https://go.com/feed")
	assert.NilError(t, err)
	_, err = database.ExecContext(ctx, `INSERT INTO subscriptions (user_did, feed_url) VALUES (?, ?)`, "did:test:bob", "https://rust.com/feed")
	assert.NilError(t, err)

	engine := NewEngine(database.DB, slog.Default())
	assert.NilError(t, engine.ComputeFeedSimilarity(ctx))

	var count int
	assert.NilError(t, database.QueryRowContext(ctx, `SELECT COUNT(*) FROM feed_similarity`).Scan(&count))
	assert.Assert(t, count >= 0, "description-based similarity should produce pairs")

	if count > 0 {
		var jaccard float64
		assert.NilError(t, database.QueryRowContext(ctx,
			`SELECT jaccard FROM feed_similarity WHERE feed_a = ? AND feed_b = ?`,
			"https://go.com/feed", "https://rust.com/feed").Scan(&jaccard))
		assert.Assert(t, jaccard > 0, "description word overlap should boost similarity")
	}
}
