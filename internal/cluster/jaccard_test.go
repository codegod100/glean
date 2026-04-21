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
		if r["feed_url"] == "https://a.com/feed" || r["feed_url"] == "https://b.com/feed" {
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
		assert.Assert(t, !subscribedFeeds[r["feed_url"].(string)],
			"should not recommend a feed the user already subscribes to")
	}
}
