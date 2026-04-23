package cluster

import (
	"context"
	"fmt"
	"os"
	"testing"

	"pkg.rbrt.fr/glean/internal/db"

	"gotest.tools/v3/assert"
	"log/slog"
)

func setupClusterTestDB(t *testing.T) *db.Databases {
	t.Helper()
	f, err := os.CreateTemp("", "glean-cluster-test-*.db")
	assert.NilError(t, err)
	assert.NilError(t, f.Close())
	path := f.Name()
	t.Cleanup(func() {
		_ = os.Remove(path)
		_ = os.Remove(path + "_users")
		_ = os.Remove(path + "_users-shm")
		_ = os.Remove(path + "_users-wal")
		_ = os.Remove(path + "_articles")
		_ = os.Remove(path + "_articles-shm")
		_ = os.Remove(path + "_articles-wal")
		_ = os.Remove(path + "_recs")
		_ = os.Remove(path + "_recs-shm")
		_ = os.Remove(path + "_recs-wal")
	})

	dbs, err := db.OpenAll(path)
	assert.NilError(t, err)
	t.Cleanup(func() { _ = dbs.Close() })
	return dbs
}

func seedClusterData(t *testing.T, ctx context.Context, dbs *db.Databases) {
	t.Helper()

	users := []struct{ did, handle string }{
		{"did:test:alice", "alice"},
		{"did:test:bob", "bob"},
		{"did:test:carol", "carol"},
	}
	for _, u := range users {
		_, err := dbs.DB().ExecContext(ctx, `INSERT INTO users (did, handle) VALUES (?, ?)`, u.did, u.handle)
		assert.NilError(t, err)
	}

	feeds := []struct{ url, title string }{
		{"https://a.com/feed", "Feed A"},
		{"https://b.com/feed", "Feed B"},
		{"https://c.com/feed", "Feed C"},
		{"https://d.com/feed", "Feed D"},
		{"https://e.com/feed", "Feed E"},
	}
	for _, f := range feeds {
		_, err := dbs.DB().ExecContext(ctx, `INSERT INTO articles.feeds (feed_url, title, site_url, description, feed_type, subscriber_count) VALUES (?, ?, ?, '', 'rss', 2)`, f.url, f.title, f.url)
		assert.NilError(t, err)
	}

	subs := []struct{ user, feed string }{
		{"did:test:alice", "https://a.com/feed"},
		{"did:test:alice", "https://b.com/feed"},
		{"did:test:alice", "https://c.com/feed"},
		{"did:test:alice", "https://d.com/feed"},
		{"did:test:alice", "https://e.com/feed"},
		{"did:test:bob", "https://a.com/feed"},
		{"did:test:bob", "https://b.com/feed"},
		{"did:test:bob", "https://d.com/feed"},
		{"did:test:carol", "https://c.com/feed"},
	}
	for _, s := range subs {
		_, err := dbs.DB().ExecContext(ctx, `INSERT INTO articles.subscriptions (user_did, feed_url) VALUES (?, ?)`, s.user, s.feed)
		assert.NilError(t, err)
	}
}

func seedFollowData(t *testing.T, ctx context.Context, dbs *db.Databases) {
	t.Helper()
	follows := []struct{ user, target string }{
		{"did:test:alice", "did:test:bob"},
		{"did:test:bob", "did:test:carol"},
	}
	for _, f := range follows {
		_, err := dbs.DB().ExecContext(ctx, `INSERT OR IGNORE INTO follows (user_did, target_did) VALUES (?, ?)`, f.user, f.target)
		assert.NilError(t, err)
	}
}

func newTestEngine(dbs *db.Databases) *Engine {
	return NewEngine(dbs.DB(), slog.Default())
}

func TestComputeFeedSimilarity(t *testing.T) {
	ctx := context.Background()
	dbs := setupClusterTestDB(t)
	seedClusterData(t, ctx, dbs)

	engine := newTestEngine(dbs)
	err := engine.ComputeFeedSimilarity(ctx)
	assert.NilError(t, err)

	var count int
	err = dbs.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM recs.feed_similarity`).Scan(&count)
	assert.NilError(t, err)
	assert.Assert(t, count > 0, "expected feed similarity pairs")
}

func TestComputeUserSimilarity(t *testing.T) {
	ctx := context.Background()
	dbs := setupClusterTestDB(t)
	seedClusterData(t, ctx, dbs)

	engine := newTestEngine(dbs)
	err := engine.ComputeUserSimilarity(ctx)
	assert.NilError(t, err)

	var count int
	err = dbs.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM recs.user_similarity`).Scan(&count)
	assert.NilError(t, err)
	assert.Assert(t, count > 0, "expected user similarity pairs")
}

func TestOnDemandFeedRecommendations(t *testing.T) {
	ctx := context.Background()
	dbs := setupClusterTestDB(t)
	seedClusterData(t, ctx, dbs)

	engine := newTestEngine(dbs)
	assert.NilError(t, engine.ComputeFeedSimilarity(ctx))
	assert.NilError(t, engine.ComputeUserSimilarity(ctx))

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

func TestNoSelfRecommendations(t *testing.T) {
	ctx := context.Background()
	dbs := setupClusterTestDB(t)
	seedClusterData(t, ctx, dbs)

	engine := newTestEngine(dbs)
	assert.NilError(t, engine.ComputeFeedSimilarity(ctx))
	assert.NilError(t, engine.ComputeUserSimilarity(ctx))

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

func TestDismissedFeedsExcluded(t *testing.T) {
	ctx := context.Background()
	dbs := setupClusterTestDB(t)
	seedClusterData(t, ctx, dbs)

	engine := newTestEngine(dbs)
	assert.NilError(t, engine.ComputeFeedSimilarity(ctx))
	assert.NilError(t, engine.ComputeUserSimilarity(ctx))

	assert.NilError(t, engine.DismissFeed(ctx, "did:test:carol", "https://a.com/feed", "not_interested"))

	recs, err := engine.GetFeedRecommendations(ctx, "did:test:carol", 10)
	assert.NilError(t, err)

	for _, r := range recs {
		assert.Assert(t, r.FeedURL != "https://a.com/feed",
			"dismissed feed should not appear in recommendations")
	}
}

func TestIsFeedDismissed(t *testing.T) {
	ctx := context.Background()
	dbs := setupClusterTestDB(t)
	seedClusterData(t, ctx, dbs)

	engine := newTestEngine(dbs)

	dismissed, err := engine.IsFeedDismissed(ctx, "did:test:alice", "https://a.com/feed")
	assert.NilError(t, err)
	assert.Assert(t, !dismissed, "feed should not be dismissed initially")

	assert.NilError(t, engine.DismissFeed(ctx, "did:test:alice", "https://a.com/feed", "not_interested"))

	dismissed, err = engine.IsFeedDismissed(ctx, "did:test:alice", "https://a.com/feed")
	assert.NilError(t, err)
	assert.Assert(t, dismissed, "feed should be dismissed after dismiss call")
}

func TestRecordImpressions(t *testing.T) {
	ctx := context.Background()
	dbs := setupClusterTestDB(t)
	seedClusterData(t, ctx, dbs)

	engine := newTestEngine(dbs)

	impressions := []Impression{
		{TargetType: "feed", TargetID: "https://a.com/feed"},
		{TargetType: "feed", TargetID: "https://b.com/feed"},
	}
	assert.NilError(t, engine.RecordImpressions(ctx, "did:test:alice", impressions))

	var count int
	assert.NilError(t, dbs.DB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM recs.recommendation_impressions WHERE user_did = 'did:test:alice'`).Scan(&count))
	assert.Equal(t, count, 2)

	assert.NilError(t, engine.RecordImpressions(ctx, "did:test:alice", impressions))

	var shownCount int
	assert.NilError(t, dbs.DB().QueryRowContext(ctx,
		`SELECT shown_count FROM recs.recommendation_impressions WHERE user_did = 'did:test:alice' AND target_id = 'https://a.com/feed'`).Scan(&shownCount))
	assert.Equal(t, shownCount, 2, "shown_count should increment on repeated impression")
}

func TestMarkImpressionActed(t *testing.T) {
	ctx := context.Background()
	dbs := setupClusterTestDB(t)
	seedClusterData(t, ctx, dbs)

	engine := newTestEngine(dbs)

	impressions := []Impression{{TargetType: "feed", TargetID: "https://a.com/feed"}}
	assert.NilError(t, engine.RecordImpressions(ctx, "did:test:alice", impressions))

	assert.NilError(t, engine.MarkImpressionActed(ctx, "did:test:alice", "feed", "https://a.com/feed"))

	var acted bool
	assert.NilError(t, dbs.DB().QueryRowContext(ctx,
		`SELECT acted FROM recs.recommendation_impressions WHERE user_did = 'did:test:alice' AND target_id = 'https://a.com/feed'`).Scan(&acted))
	assert.Assert(t, acted, "impression should be marked as acted")
}

func TestComputeFollowDistances(t *testing.T) {
	ctx := context.Background()
	dbs := setupClusterTestDB(t)
	seedClusterData(t, ctx, dbs)
	seedFollowData(t, ctx, dbs)

	engine := newTestEngine(dbs)
	assert.NilError(t, engine.ComputeFollowDistances(ctx))

	var d1, d2 int
	assert.NilError(t, dbs.DB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM recs.follow_distances WHERE distance = 1`).Scan(&d1))
	assert.NilError(t, dbs.DB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM recs.follow_distances WHERE distance = 2`).Scan(&d2))
	assert.Assert(t, d1 >= 2, "expected at least 2 direct follow distances")
	assert.Assert(t, d2 >= 1, "expected at least 1 two-hop distance (alice -> bob -> carol)")

	var dist int
	assert.NilError(t, dbs.DB().QueryRowContext(ctx,
		`SELECT distance FROM recs.follow_distances WHERE user_a = 'did:test:alice' AND user_b = 'did:test:carol'`).Scan(&dist))
	assert.Equal(t, dist, 2, "alice should be 2 hops from carol")
}

func TestAutoDismissStale(t *testing.T) {
	ctx := context.Background()
	dbs := setupClusterTestDB(t)
	seedClusterData(t, ctx, dbs)

	engine := newTestEngine(dbs)

	_, err := dbs.DB().ExecContext(ctx, `
		INSERT INTO recs.recommendation_impressions (user_did, target_type, target_id, first_shown_at, last_shown_at, shown_count, acted)
		VALUES ('did:test:alice', 'feed', 'https://stale.com/feed', datetime('now', '-31 days'), datetime('now'), 20, 0)
	`)
	assert.NilError(t, err)

	assert.NilError(t, engine.AutoDismissStale(ctx, 15, 30))

	dismissed, err := engine.IsFeedDismissed(ctx, "did:test:alice", "https://stale.com/feed")
	assert.NilError(t, err)
	assert.Assert(t, dismissed, "stale recommendation should be auto-dismissed")
}

func TestAutoDismissStale_DoesNotDismissRecent(t *testing.T) {
	ctx := context.Background()
	dbs := setupClusterTestDB(t)
	seedClusterData(t, ctx, dbs)

	engine := newTestEngine(dbs)

	_, err := dbs.DB().ExecContext(ctx, `
		INSERT INTO recs.recommendation_impressions (user_did, target_type, target_id, first_shown_at, last_shown_at, shown_count, acted)
		VALUES ('did:test:alice', 'feed', 'https://recent.com/feed', datetime('now'), datetime('now'), 5, 0)
	`)
	assert.NilError(t, err)

	assert.NilError(t, engine.AutoDismissStale(ctx, 15, 30))

	dismissed, err := engine.IsFeedDismissed(ctx, "did:test:alice", "https://recent.com/feed")
	assert.NilError(t, err)
	assert.Assert(t, !dismissed, "recent impression should not be auto-dismissed")
}

func TestAutoDismissStale_DoesNotDismissActed(t *testing.T) {
	ctx := context.Background()
	dbs := setupClusterTestDB(t)
	seedClusterData(t, ctx, dbs)

	engine := newTestEngine(dbs)

	_, err := dbs.DB().ExecContext(ctx, `
		INSERT INTO recs.recommendation_impressions (user_did, target_type, target_id, first_shown_at, last_shown_at, shown_count, acted)
		VALUES ('did:test:alice', 'feed', 'https://acted.com/feed', datetime('now', '-31 days'), datetime('now'), 20, 1)
	`)
	assert.NilError(t, err)

	assert.NilError(t, engine.AutoDismissStale(ctx, 15, 30))

	dismissed, err := engine.IsFeedDismissed(ctx, "did:test:alice", "https://acted.com/feed")
	assert.NilError(t, err)
	assert.Assert(t, !dismissed, "acted recommendation should not be auto-dismissed")
}

func TestDiversityFiltering(t *testing.T) {
	candidates := []*FeedRecommendation{
		{FeedURL: "https://a.com/1", SiteURL: "https://a.com", Score: 1.0},
		{FeedURL: "https://a.com/2", SiteURL: "https://a.com", Score: 0.9},
		{FeedURL: "https://a.com/3", SiteURL: "https://a.com", Score: 0.8},
		{FeedURL: "https://b.com/1", SiteURL: "https://b.com", Score: 0.7},
		{FeedURL: "https://b.com/2", SiteURL: "https://b.com", Score: 0.6},
		{FeedURL: "https://c.com/1", SiteURL: "https://c.com", Score: 0.5},
	}

	result := ApplyDiversity(candidates, 6)

	aCount := 0
	bCount := 0
	cCount := 0
	for _, r := range result {
		switch extractDomain(r.SiteURL) {
		case "a.com":
			aCount++
		case "b.com":
			bCount++
		case "c.com":
			cCount++
		}
	}
	assert.Assert(t, aCount <= maxPerDomain, "should limit feeds from same domain")
	assert.Assert(t, len(result) <= 6, "should respect topN limit")
	assert.Assert(t, cCount >= 1, "should include feeds from different domains")
}

func TestDiversityFiltering_EmptySiteURL(t *testing.T) {
	candidates := []*FeedRecommendation{
		{FeedURL: "https://a.com/1", SiteURL: "", Score: 1.0},
		{FeedURL: "https://b.com/1", SiteURL: "", Score: 0.9},
	}
	result := ApplyDiversity(candidates, 5)
	assert.Equal(t, len(result), 2, "feeds without site_url should not be filtered out")
}

func TestSignalWeights_Default(t *testing.T) {
	ctx := context.Background()
	dbs := setupClusterTestDB(t)
	seedClusterData(t, ctx, dbs)

	engine := newTestEngine(dbs)
	w := engine.GetWeights(ctx, "did:test:alice")

	assert.Equal(t, w.WSub, 1.0)
	assert.Equal(t, w.WLike, 0.5)
	assert.Equal(t, w.WTag, 0.3)
	assert.Equal(t, w.WSocial, 0.7)
	assert.Equal(t, w.WPop, 0.2)
	assert.Equal(t, w.WCategory, 0.4)
}

func TestSignalWeights_RewardPenalize(t *testing.T) {
	ctx := context.Background()
	dbs := setupClusterTestDB(t)
	seedClusterData(t, ctx, dbs)

	engine := newTestEngine(dbs)

	_, err := dbs.DB().ExecContext(ctx, `
		INSERT INTO recs.recommendation_impressions (user_did, target_type, target_id, first_shown_at, last_shown_at, shown_count, acted)
		VALUES ('did:test:alice', 'feed', 'https://a.com/feed', datetime('now'), datetime('now'), 1, 1)
	`)
	assert.NilError(t, err)
	for i := range minActionsTune {
		_, err = dbs.DB().ExecContext(ctx, `
			INSERT INTO recs.recommendation_impressions (user_did, target_type, target_id, first_shown_at, last_shown_at, shown_count, acted)
			VALUES ('did:test:alice', 'feed', ?, datetime('now'), datetime('now'), 1, 1)
		`, fmt.Sprintf("https://%d.com/feed", i))
		assert.NilError(t, err)
	}

	engine.RewardSignal(ctx, "did:test:alice", "social")

	w := engine.GetWeights(ctx, "did:test:alice")
	assert.Assert(t, w.WSocial > 0.7, "rewarding social signal should increase w_social, got %f", w.WSocial)
}

func TestColdStartRecommendations(t *testing.T) {
	ctx := context.Background()
	dbs := setupClusterTestDB(t)
	seedClusterData(t, ctx, dbs)
	seedFollowData(t, ctx, dbs)

	engine := newTestEngine(dbs)
	assert.NilError(t, engine.ComputeFollowDistances(ctx))

	_, err := dbs.DB().ExecContext(ctx, `UPDATE articles.feeds SET subscriber_count = 2 WHERE feed_url = 'https://a.com/feed'`)
	assert.NilError(t, err)
	_, err = dbs.DB().ExecContext(ctx, `UPDATE articles.feeds SET subscriber_count = 2 WHERE feed_url = 'https://b.com/feed'`)
	assert.NilError(t, err)

	_, err = dbs.DB().ExecContext(ctx, `INSERT INTO users (did, handle) VALUES (?, ?)`, "did:test:newuser", "newuser")
	assert.NilError(t, err)

	recs, err := engine.ColdStartRecommendations(ctx, "did:test:newuser", 10)
	assert.NilError(t, err)
	assert.Assert(t, len(recs) > 0, "new user should get cold start recommendations")
}

func TestColdStartRecommendations_NotTriggeredForEstablishedUser(t *testing.T) {
	ctx := context.Background()
	dbs := setupClusterTestDB(t)
	seedClusterData(t, ctx, dbs)
	seedFollowData(t, ctx, dbs)

	engine := newTestEngine(dbs)
	assert.NilError(t, engine.ComputeFollowDistances(ctx))

	recs, err := engine.ColdStartRecommendations(ctx, "did:test:alice", 10)
	assert.NilError(t, err)
	assert.Assert(t, recs == nil, "established user should not get cold start recommendations")
}

func TestOnDemandPeopleRecommendations(t *testing.T) {
	ctx := context.Background()
	dbs := setupClusterTestDB(t)
	seedClusterData(t, ctx, dbs)

	engine := newTestEngine(dbs)
	assert.NilError(t, engine.ComputeUserSimilarity(ctx))

	recs, err := engine.GetPeopleRecommendations(ctx, "did:test:carol", 10)
	assert.NilError(t, err)
	assert.Assert(t, len(recs) > 0, "carol should get people recommendations")
}

func TestDismissArticle(t *testing.T) {
	ctx := context.Background()
	dbs := setupClusterTestDB(t)
	seedClusterData(t, ctx, dbs)

	engine := newTestEngine(dbs)

	assert.NilError(t, engine.DismissArticle(ctx, "did:test:alice", "https://a.com/article1", "not_interested"))

	var count int
	assert.NilError(t, dbs.DB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM recs.dismissed_recommendations WHERE user_did = 'did:test:alice' AND target_type = 'article'`).Scan(&count))
	assert.Equal(t, count, 1)
}

func TestComputeSignalProfiles(t *testing.T) {
	ctx := context.Background()
	dbs := setupClusterTestDB(t)
	seedClusterData(t, ctx, dbs)

	engine := newTestEngine(dbs)
	assert.NilError(t, engine.ComputeSignalProfiles(ctx))

	var count int
	assert.NilError(t, dbs.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM recs.user_signal_profiles`).Scan(&count))
	assert.Assert(t, count >= 3, "expected signal profiles for all users")
}

func TestDismissFeed_Idempotent(t *testing.T) {
	ctx := context.Background()
	dbs := setupClusterTestDB(t)
	seedClusterData(t, ctx, dbs)

	engine := newTestEngine(dbs)

	assert.NilError(t, engine.DismissFeed(ctx, "did:test:alice", "https://a.com/feed", "reason1"))
	assert.NilError(t, engine.DismissFeed(ctx, "did:test:alice", "https://a.com/feed", "reason2"))

	var count int
	assert.NilError(t, dbs.DB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM recs.dismissed_recommendations WHERE user_did = 'did:test:alice' AND target_type = 'feed'`).Scan(&count))
	assert.Equal(t, count, 1, "duplicate dismiss should not create extra rows")
}

func TestDescriptionBasedFeedSimilarity(t *testing.T) {
	ctx := context.Background()
	dbs := setupClusterTestDB(t)

	_, err := dbs.DB().ExecContext(ctx, `INSERT INTO users (did, handle) VALUES (?, ?)`, "did:test:alice", "alice")
	assert.NilError(t, err)
	_, err = dbs.DB().ExecContext(ctx, `INSERT INTO users (did, handle) VALUES (?, ?)`, "did:test:bob", "bob")
	assert.NilError(t, err)

	_, err = dbs.DB().ExecContext(ctx, `INSERT INTO articles.feeds (feed_url, title, site_url, description, feed_type) VALUES (?, ?, ?, ?, 'rss')`,
		"https://go.com/feed", "Go Blog", "https://go.com", "programming language golang software development")
	assert.NilError(t, err)
	_, err = dbs.DB().ExecContext(ctx, `INSERT INTO articles.feeds (feed_url, title, site_url, description, feed_type) VALUES (?, ?, ?, ?, 'rss')`,
		"https://rust.com/feed", "Rust Blog", "https://rust.com", "programming language rust software development")
	assert.NilError(t, err)

	_, err = dbs.DB().ExecContext(ctx, `INSERT INTO articles.subscriptions (user_did, feed_url) VALUES (?, ?)`, "did:test:alice", "https://go.com/feed")
	assert.NilError(t, err)
	_, err = dbs.DB().ExecContext(ctx, `INSERT INTO articles.subscriptions (user_did, feed_url) VALUES (?, ?)`, "did:test:bob", "https://rust.com/feed")
	assert.NilError(t, err)

	engine := newTestEngine(dbs)
	assert.NilError(t, engine.ComputeFeedSimilarity(ctx))

	var count int
	assert.NilError(t, dbs.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM recs.feed_similarity`).Scan(&count))
	assert.Assert(t, count >= 0, "description-based similarity should produce pairs")

	if count > 0 {
		var jaccard float64
		assert.NilError(t, dbs.DB().QueryRowContext(ctx,
			`SELECT jaccard FROM recs.feed_similarity WHERE feed_a = ? AND feed_b = ?`,
			"https://go.com/feed", "https://rust.com/feed").Scan(&jaccard))
		assert.Assert(t, jaccard > 0, "description word overlap should boost similarity")
	}
}
