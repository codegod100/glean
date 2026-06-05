package cluster

import (
	"context"
	"fmt"
	"hash/fnv"
	"log/slog"
	"os"
	"strings"
	"testing"

	vec "github.com/asg017/sqlite-vec-go-bindings/cgo"
	"pkg.rbrt.fr/glean/internal/db"
	"pkg.rbrt.fr/glean/internal/feedback"

	"gotest.tools/v3/assert"
)

func setupClusterTestDB(t *testing.T) *db.Store {
	t.Helper()
	vec.Auto()
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

	dbs, err := db.Open(path)
	assert.NilError(t, err)
	t.Cleanup(func() { _ = dbs.Close() })
	assert.NilError(t, dbs.InitVecTables(8))
	return dbs
}

func seedClusterData(t *testing.T, ctx context.Context, dbs *db.Store) {
	t.Helper()

	users := []string{"did:test:alice", "did:test:bob", "did:test:carol", "did:test:dave"}
	for _, did := range users {
		_, err := dbs.SQLDB().ExecContext(ctx, `INSERT INTO users (did) VALUES (?)`, did)
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
		_, err := dbs.SQLDB().ExecContext(ctx, `INSERT INTO articles.feeds (feed_url, title, site_url, description, feed_type, subscriber_count) VALUES (?, ?, ?, '', 'rss', 2)`, f.url, f.title, f.url)
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
		_, err := dbs.SQLDB().ExecContext(ctx, `INSERT INTO articles.subscriptions (user_did, feed_url) VALUES (?, ?)`, s.user, s.feed)
		assert.NilError(t, err)
	}
}

func seedFollowData(t *testing.T, ctx context.Context, dbs *db.Store) {
	t.Helper()
	follows := []struct{ user, target string }{
		{"did:test:alice", "did:test:bob"},
		{"did:test:bob", "did:test:carol"},
		{"did:test:carol", "did:test:dave"},
	}
	for _, f := range follows {
		_, err := dbs.SQLDB().ExecContext(ctx, `INSERT OR IGNORE INTO follows (user_did, target_did) VALUES (?, ?)`, f.user, f.target)
		assert.NilError(t, err)
	}
}

func newTestEngine(dbs *db.Store) *Engine {
	return NewEngine(dbs.SQLDB(), dbs.Articles, NewMockEmbedder(8), nil, feedback.NewService(dbs.SQLDB()), slog.Default(), DefaultConfig())
}

func TestComputeFeedSimilarity(t *testing.T) {
	ctx := context.Background()
	dbs := setupClusterTestDB(t)
	seedClusterData(t, ctx, dbs)

	engine := newTestEngine(dbs)
	err := engine.ComputeFeedSimilarity(ctx)
	assert.NilError(t, err)

	var count int
	err = dbs.SQLDB().QueryRowContext(ctx, `SELECT COUNT(*) FROM recs.feed_similarity`).Scan(&count)
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
	err = dbs.SQLDB().QueryRowContext(ctx, `SELECT COUNT(*) FROM recs.user_similarity`).Scan(&count)
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

	assert.NilError(t, engine.feedback.DismissFeed(ctx, "did:test:carol", "https://a.com/feed", "not_interested"))

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

	dismissed, err := engine.feedback.IsFeedDismissed(ctx, "did:test:alice", "https://a.com/feed")
	assert.NilError(t, err)
	assert.Assert(t, !dismissed, "feed should not be dismissed initially")

	assert.NilError(t, engine.feedback.DismissFeed(ctx, "did:test:alice", "https://a.com/feed", "not_interested"))

	dismissed, err = engine.feedback.IsFeedDismissed(ctx, "did:test:alice", "https://a.com/feed")
	assert.NilError(t, err)
	assert.Assert(t, dismissed, "feed should be dismissed after dismiss call")
}

func TestRecordImpressions(t *testing.T) {
	ctx := context.Background()
	dbs := setupClusterTestDB(t)
	seedClusterData(t, ctx, dbs)

	engine := newTestEngine(dbs)

	impressions := []feedback.Impression{
		{TargetType: "feed", TargetID: "https://a.com/feed"},
		{TargetType: "feed", TargetID: "https://b.com/feed"},
	}
	assert.NilError(t, engine.feedback.RecordImpressions(ctx, "did:test:alice", impressions))

	var count int
	assert.NilError(t, dbs.SQLDB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM main.recommendation_impressions WHERE user_did = 'did:test:alice'`).Scan(&count))
	assert.Equal(t, count, 2)

	assert.NilError(t, engine.feedback.RecordImpressions(ctx, "did:test:alice", impressions))

	var shownCount int
	assert.NilError(t, dbs.SQLDB().QueryRowContext(ctx,
		`SELECT shown_count FROM main.recommendation_impressions WHERE user_did = 'did:test:alice' AND target_id = 'https://a.com/feed'`).Scan(&shownCount))
	assert.Equal(t, shownCount, 2, "shown_count should increment on repeated impression")
}

func TestMarkImpressionActed(t *testing.T) {
	ctx := context.Background()
	dbs := setupClusterTestDB(t)
	seedClusterData(t, ctx, dbs)

	engine := newTestEngine(dbs)

	impressions := []feedback.Impression{{TargetType: "feed", TargetID: "https://a.com/feed"}}
	assert.NilError(t, engine.feedback.RecordImpressions(ctx, "did:test:alice", impressions))

	assert.NilError(t, engine.feedback.MarkImpressionActed(ctx, "did:test:alice", "feed", "https://a.com/feed"))

	var acted bool
	assert.NilError(t, dbs.SQLDB().QueryRowContext(ctx,
		`SELECT acted FROM main.recommendation_impressions WHERE user_did = 'did:test:alice' AND target_id = 'https://a.com/feed'`).Scan(&acted))
	assert.Assert(t, acted, "impression should be marked as acted")
}

func TestComputeFollowDistances(t *testing.T) {
	ctx := context.Background()
	dbs := setupClusterTestDB(t)
	seedClusterData(t, ctx, dbs)
	seedFollowData(t, ctx, dbs)

	engine := newTestEngine(dbs)
	assert.NilError(t, engine.ComputeFollowDistances(ctx))

	var d1, d2, d3 int
	assert.NilError(t, dbs.SQLDB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM recs.follow_distances WHERE distance = 1`).Scan(&d1))
	assert.NilError(t, dbs.SQLDB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM recs.follow_distances WHERE distance = 2`).Scan(&d2))
	assert.NilError(t, dbs.SQLDB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM recs.follow_distances WHERE distance = 3`).Scan(&d3))
	assert.Assert(t, d1 >= 3, "expected at least 3 direct follow distances")
	assert.Assert(t, d2 >= 1, "expected at least 1 two-hop distance (alice -> bob -> carol)")
	assert.Assert(t, d3 >= 1, "expected at least 1 three-hop distance (alice -> bob -> carol -> dave)")

	var exists int
	assert.NilError(t, dbs.SQLDB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM recs.follow_distances WHERE user_a = 'did:test:alice' AND user_b = 'did:test:carol'`).Scan(&exists))
	assert.Assert(t, exists == 1, "alice should reach carol")

	assert.NilError(t, dbs.SQLDB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM recs.follow_distances WHERE user_a = 'did:test:alice' AND user_b = 'did:test:dave'`).Scan(&exists))
	assert.Assert(t, exists == 1, "alice should reach dave via 3 hops")
}

func TestComputeFollowDistances_WritesPairsToDB(t *testing.T) {
	ctx := context.Background()
	dbs := setupClusterTestDB(t)
	seedClusterData(t, ctx, dbs)
	seedFollowData(t, ctx, dbs)

	engine := newTestEngine(dbs)

	assert.NilError(t, engine.ComputeFollowDistances(ctx))

	var count int
	assert.NilError(t, dbs.SQLDB().QueryRowContext(ctx, `SELECT COUNT(*) FROM recs.follow_distances`).Scan(&count))
	assert.Assert(t, count > 0, "expected follow distance pairs")
}

func TestAutoDismissStale(t *testing.T) {
	ctx := context.Background()
	dbs := setupClusterTestDB(t)
	seedClusterData(t, ctx, dbs)

	engine := newTestEngine(dbs)

	_, err := dbs.SQLDB().ExecContext(ctx, `
		INSERT INTO main.recommendation_impressions (user_did, target_type, target_id, first_shown_at, last_shown_at, shown_count, acted)
		VALUES ('did:test:alice', 'feed', 'https://stale.com/feed', datetime('now', '-6 days'), datetime('now'), 6, 0)
	`)
	assert.NilError(t, err)

	assert.NilError(t, engine.feedback.AutoDismissStale(ctx, 5, 5))

	dismissed, err := engine.feedback.IsFeedDismissed(ctx, "did:test:alice", "https://stale.com/feed")
	assert.NilError(t, err)
	assert.Assert(t, dismissed, "stale recommendation should be auto-dismissed")
}

func TestAutoDismissStale_DoesNotDismissRecent(t *testing.T) {
	ctx := context.Background()
	dbs := setupClusterTestDB(t)
	seedClusterData(t, ctx, dbs)

	engine := newTestEngine(dbs)

	_, err := dbs.SQLDB().ExecContext(ctx, `
		INSERT INTO main.recommendation_impressions (user_did, target_type, target_id, first_shown_at, last_shown_at, shown_count, acted)
		VALUES ('did:test:alice', 'feed', 'https://recent.com/feed', datetime('now'), datetime('now'), 5, 0)
	`)
	assert.NilError(t, err)

	assert.NilError(t, engine.feedback.AutoDismissStale(ctx, 5, 5))

	dismissed, err := engine.feedback.IsFeedDismissed(ctx, "did:test:alice", "https://recent.com/feed")
	assert.NilError(t, err)
	assert.Assert(t, !dismissed, "recent impression should not be auto-dismissed")
}

func TestAutoDismissStale_DoesNotDismissActed(t *testing.T) {
	ctx := context.Background()
	dbs := setupClusterTestDB(t)
	seedClusterData(t, ctx, dbs)

	engine := newTestEngine(dbs)

	_, err := dbs.SQLDB().ExecContext(ctx, `
		INSERT INTO main.recommendation_impressions (user_did, target_type, target_id, first_shown_at, last_shown_at, shown_count, acted)
		VALUES ('did:test:alice', 'feed', 'https://acted.com/feed', datetime('now', '-6 days'), datetime('now'), 6, 1)
	`)
	assert.NilError(t, err)

	assert.NilError(t, engine.feedback.AutoDismissStale(ctx, 5, 5))

	dismissed, err := engine.feedback.IsFeedDismissed(ctx, "did:test:alice", "https://acted.com/feed")
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
	assert.Equal(t, w.WContent, 0.4)
}

func TestSignalWeights_RewardPenalize(t *testing.T) {
	ctx := context.Background()
	dbs := setupClusterTestDB(t)
	seedClusterData(t, ctx, dbs)

	engine := newTestEngine(dbs)

	_, err := dbs.SQLDB().ExecContext(ctx, `
		INSERT INTO main.recommendation_impressions (user_did, target_type, target_id, first_shown_at, last_shown_at, shown_count, acted)
		VALUES ('did:test:alice', 'feed', 'https://a.com/feed', datetime('now'), datetime('now'), 1, 1)
	`)
	assert.NilError(t, err)
	for i := range minActionsTune {
		_, err = dbs.SQLDB().ExecContext(ctx, `
			INSERT INTO main.recommendation_impressions (user_did, target_type, target_id, first_shown_at, last_shown_at, shown_count, acted)
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

	_, err := dbs.SQLDB().ExecContext(ctx, `INSERT INTO users (did) VALUES (?)`, "did:test:newuser")
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
	seedFollowData(t, ctx, dbs)

	engine := newTestEngine(dbs)
	assert.NilError(t, engine.ComputeUserSimilarity(ctx))

	recs, err := engine.GetPeopleRecommendations(ctx, "did:test:carol", 10)
	assert.NilError(t, err)
	assert.Assert(t, len(recs) > 0, "carol should get people recommendations")

	for _, r := range recs {
		if r.DID == "did:test:dave" {
			assert.Assert(t, r.IsFollowed, "carol follows dave, should be marked as followed")
		}
		if r.DID == "did:test:alice" || r.DID == "did:test:bob" {
			assert.Assert(t, !r.IsFollowed, "carol does not follow %s, should not be marked as followed", r.DID)
		}
	}
}

func TestDismissArticle(t *testing.T) {
	ctx := context.Background()
	dbs := setupClusterTestDB(t)
	seedClusterData(t, ctx, dbs)

	engine := newTestEngine(dbs)

	assert.NilError(t, engine.feedback.DismissArticle(ctx, "did:test:alice", "https://a.com/article1", "not_interested"))

	var count int
	assert.NilError(t, dbs.SQLDB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM main.dismissed_recommendations WHERE user_did = 'did:test:alice' AND target_type = 'article'`).Scan(&count))
	assert.Equal(t, count, 1)
}

func TestComputeSignalProfiles(t *testing.T) {
	ctx := context.Background()
	dbs := setupClusterTestDB(t)
	seedClusterData(t, ctx, dbs)

	engine := newTestEngine(dbs)
	assert.NilError(t, engine.ComputeSignalProfiles(ctx))

	var count int
	assert.NilError(t, dbs.SQLDB().QueryRowContext(ctx, `SELECT COUNT(*) FROM recs.user_signal_profiles`).Scan(&count))
	assert.Assert(t, count >= 3, "expected signal profiles for all users")
}

func TestDismissFeed_Idempotent(t *testing.T) {
	ctx := context.Background()
	dbs := setupClusterTestDB(t)
	seedClusterData(t, ctx, dbs)

	engine := newTestEngine(dbs)

	assert.NilError(t, engine.feedback.DismissFeed(ctx, "did:test:alice", "https://a.com/feed", "reason1"))
	assert.NilError(t, engine.feedback.DismissFeed(ctx, "did:test:alice", "https://a.com/feed", "reason2"))

	var count int
	assert.NilError(t, dbs.SQLDB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM main.dismissed_recommendations WHERE user_did = 'did:test:alice' AND target_type = 'feed'`).Scan(&count))
	assert.Equal(t, count, 1, "duplicate dismiss should not create extra rows")
}

func TestEmbeddingBasedFeedSimilarity(t *testing.T) {
	ctx := context.Background()
	dbs := setupClusterTestDB(t)

	_, err := dbs.SQLDB().ExecContext(ctx, `INSERT INTO users (did) VALUES (?)`, "did:test:alice")
	assert.NilError(t, err)
	_, err = dbs.SQLDB().ExecContext(ctx, `INSERT INTO users (did) VALUES (?)`, "did:test:bob")
	assert.NilError(t, err)

	_, err = dbs.SQLDB().ExecContext(ctx, `INSERT INTO articles.feeds (feed_url, title, site_url, description, feed_type, subscriber_count) VALUES (?, ?, ?, ?, 'rss', 2)`,
		"https://go.com/feed", "Go Blog", "https://go.com", "programming language golang software development")
	assert.NilError(t, err)
	_, err = dbs.SQLDB().ExecContext(ctx, `INSERT INTO articles.feeds (feed_url, title, site_url, description, feed_type, subscriber_count) VALUES (?, ?, ?, ?, 'rss', 2)`,
		"https://rust.com/feed", "Rust Blog", "https://rust.com", "programming language rust software development")
	assert.NilError(t, err)

	_, err = dbs.SQLDB().ExecContext(ctx, `INSERT INTO articles.subscriptions (user_did, feed_url, added_at) VALUES (?, ?, CURRENT_TIMESTAMP)`, "did:test:alice", "https://go.com/feed")
	assert.NilError(t, err)
	_, err = dbs.SQLDB().ExecContext(ctx, `INSERT INTO articles.subscriptions (user_did, feed_url, added_at) VALUES (?, ?, CURRENT_TIMESTAMP)`, "did:test:alice", "https://rust.com/feed")
	assert.NilError(t, err)
	_, err = dbs.SQLDB().ExecContext(ctx, `INSERT INTO articles.subscriptions (user_did, feed_url, added_at) VALUES (?, ?, CURRENT_TIMESTAMP)`, "did:test:bob", "https://go.com/feed")
	assert.NilError(t, err)
	_, err = dbs.SQLDB().ExecContext(ctx, `INSERT INTO articles.subscriptions (user_did, feed_url, added_at) VALUES (?, ?, CURRENT_TIMESTAMP)`, "did:test:bob", "https://rust.com/feed")
	assert.NilError(t, err)

	engine := newTestEngine(dbs)

	assert.NilError(t, engine.ComputeFeedEmbeddings(ctx))
	assert.NilError(t, engine.ComputeFeedSimilarity(ctx))

	var jaccard float64
	assert.NilError(t, dbs.SQLDB().QueryRowContext(ctx,
		`SELECT jaccard FROM recs.feed_similarity WHERE feed_a = ? AND feed_b = ?`,
		"https://go.com/feed", "https://rust.com/feed").Scan(&jaccard))
	assert.Assert(t, jaccard > 1.0, "embedding cosine similarity should boost feed similarity above pure Jaccard")
}

type MockEmbedder struct {
	dimension int
}

func NewMockEmbedder(dimension int) *MockEmbedder {
	return &MockEmbedder{dimension: dimension}
}

func (m *MockEmbedder) Embed(_ context.Context, texts []string, _ string) ([][]float32, error) {
	result := make([][]float32, len(texts))
	for i, text := range texts {
		vec := make([]float32, m.dimension)
		for word := range strings.FieldsSeq(strings.ToLower(text)) {
			if len(word) < 2 {
				continue
			}
			h := fnv.New32a()
			h.Write([]byte(word))
			idx := h.Sum32() % uint32(m.dimension)
			vec[idx] += 1.0
		}
		result[i] = vec
	}
	return result, nil
}

func (m *MockEmbedder) Dimension() int {
	return m.dimension
}

func TestComputeArticleEmbeddings(t *testing.T) {
	ctx := context.Background()
	dbs := setupClusterTestDB(t)

	_, err := dbs.SQLDB().ExecContext(ctx, `INSERT INTO articles.feeds (feed_url, title, site_url, feed_type, subscriber_count) VALUES (?, ?, ?, 'rss', 1)`,
		"https://tech.com/feed", "Tech Feed", "https://tech.com")
	assert.NilError(t, err)

	_, err = dbs.SQLDB().ExecContext(ctx, `INSERT INTO articles.articles (feed_url, guid, title, summary, url) VALUES (?, ?, ?, ?, ?)`,
		"https://tech.com/feed", "1", "golang programming language tutorial", "learn the go programming language for backend development", "https://tech.com/go")
	assert.NilError(t, err)
	_, err = dbs.SQLDB().ExecContext(ctx, `INSERT INTO articles.articles (feed_url, guid, title, summary, url) VALUES (?, ?, ?, ?, ?)`,
		"https://tech.com/feed", "2", "rust programming language guide", "learn the rust programming language for systems development", "https://tech.com/rust")
	assert.NilError(t, err)
	_, err = dbs.SQLDB().ExecContext(ctx, `INSERT INTO articles.articles (feed_url, guid, title, summary, url) VALUES (?, ?, ?, ?, ?)`,
		"https://tech.com/feed", "3", "cooking recipes for dinner", "easy dinner recipes for the whole family", "https://tech.com/cook")
	assert.NilError(t, err)

	engine := newTestEngine(dbs)

	assert.NilError(t, engine.ComputeArticleEmbeddings(ctx))

	var count int
	assert.NilError(t, dbs.SQLDB().QueryRowContext(ctx, `SELECT COUNT(*) FROM recs.article_embeddings`).Scan(&count))
	assert.Equal(t, count, 3, "expected 3 article embeddings")
}

func seedArticleRecData(t *testing.T, ctx context.Context, dbs *db.Store) {
	t.Helper()

	for _, did := range []string{"did:test:alice", "did:test:bob"} {
		_, err := dbs.SQLDB().ExecContext(ctx, `INSERT INTO users (did) VALUES (?)`, did)
		assert.NilError(t, err)
	}

	for _, f := range []struct{ url, title string }{
		{"https://tech.com/feed", "Tech Feed"},
		{"https://dev.com/feed", "Dev Feed"},
		{"https://shared.com/feed", "Shared Feed"},
	} {
		_, err := dbs.SQLDB().ExecContext(ctx, `INSERT INTO articles.feeds (feed_url, title, site_url, feed_type, subscriber_count) VALUES (?, ?, ?, 'rss', 2)`, f.url, f.title, f.url)
		assert.NilError(t, err)
	}

	subs := []struct{ user, feed string }{
		{"did:test:alice", "https://tech.com/feed"},
		{"did:test:alice", "https://shared.com/feed"},
		{"did:test:bob", "https://dev.com/feed"},
		{"did:test:bob", "https://shared.com/feed"},
	}
	for _, s := range subs {
		_, err := dbs.SQLDB().ExecContext(ctx, `INSERT INTO articles.subscriptions (user_did, feed_url) VALUES (?, ?)`, s.user, s.feed)
		assert.NilError(t, err)
	}

	articles := []struct{ feed, guid, title, summary, url, lang string }{
		{"https://tech.com/feed", "1", "golang programming tutorial", "learn go programming", "https://tech.com/go", "en"},
		{"https://dev.com/feed", "2", "rust programming tutorial", "learn rust programming", "https://dev.com/rust", "fr"},
		{"https://dev.com/feed", "3", "cooking dinner recipes", "easy dinner recipes", "https://dev.com/cook", ""},
		{"https://dev.com/feed", "4", "python data science", "python for data analysis", "https://dev.com/python", "de"},
	}
	for _, a := range articles {
		_, err := dbs.SQLDB().ExecContext(ctx, `INSERT INTO articles.articles (feed_url, guid, title, summary, url, published, language) VALUES (?, ?, ?, ?, ?, datetime('now'), ?)`,
			a.feed, a.guid, a.title, a.summary, a.url, a.lang)
		assert.NilError(t, err)
	}

	likes := []struct{ uri, author, feed, article string }{
		{"at://bob/like/1", "did:test:bob", "https://tech.com/feed", "https://tech.com/go"},
		{"at://bob/like/2", "did:test:bob", "https://dev.com/feed", "https://dev.com/rust"},
		{"at://bob/like/3", "did:test:bob", "https://dev.com/feed", "https://dev.com/cook"},
		{"at://bob/like/4", "did:test:bob", "https://dev.com/feed", "https://dev.com/python"},
	}
	for _, l := range likes {
		_, err := dbs.SQLDB().ExecContext(ctx, `INSERT INTO articles.likes (uri, author_did, feed_url, article_url, created_at) VALUES (?, ?, ?, ?, datetime('now'))`,
			l.uri, l.author, l.feed, l.article)
		assert.NilError(t, err)
	}
}

func TestArticleRecommendations_LanguageFilter_ShowsUnclassified(t *testing.T) {
	ctx := context.Background()
	dbs := setupClusterTestDB(t)
	seedArticleRecData(t, ctx, dbs)

	engine := newTestEngine(dbs)
	assert.NilError(t, engine.ComputeFeedSimilarity(ctx))
	assert.NilError(t, engine.ComputeUserSimilarity(ctx))
	assert.NilError(t, engine.ComputeArticleEmbeddings(ctx))

	recs, err := engine.GetArticleRecommendations(ctx, "did:test:alice", []string{"en"}, 10)
	assert.NilError(t, err)
	assert.Assert(t, len(recs) > 0, "alice should get recommendations with language filter")

	urls := make(map[string]bool)
	for _, r := range recs {
		urls[r.URL] = true
	}
	assert.Assert(t, urls["https://tech.com/go"], "should include article with language 'en'")
	assert.Assert(t, urls["https://dev.com/cook"], "should include article with empty language (unclassified)")
	assert.Assert(t, !urls["https://dev.com/rust"], "should exclude article with language 'fr'")
	assert.Assert(t, !urls["https://dev.com/python"], "should exclude article with language 'de'")
}

func TestArticleRecommendations_LanguageFilter_MultipleLanguages(t *testing.T) {
	ctx := context.Background()
	dbs := setupClusterTestDB(t)
	seedArticleRecData(t, ctx, dbs)

	engine := newTestEngine(dbs)
	assert.NilError(t, engine.ComputeFeedSimilarity(ctx))
	assert.NilError(t, engine.ComputeUserSimilarity(ctx))
	assert.NilError(t, engine.ComputeArticleEmbeddings(ctx))

	recs, err := engine.GetArticleRecommendations(ctx, "did:test:alice", []string{"en", "fr"}, 10)
	assert.NilError(t, err)
	assert.Assert(t, len(recs) > 0, "alice should get recommendations with multi-language filter")

	urls := make(map[string]bool)
	for _, r := range recs {
		urls[r.URL] = true
	}
	assert.Assert(t, urls["https://tech.com/go"], "should include article with language 'en'")
	assert.Assert(t, urls["https://dev.com/rust"], "should include article with language 'fr'")
	assert.Assert(t, urls["https://dev.com/cook"], "should include article with empty language (unclassified)")
	assert.Assert(t, !urls["https://dev.com/python"], "should exclude article with language 'de'")
}

func TestArticleRecommendations_LanguageFilter_NoPreferences(t *testing.T) {
	ctx := context.Background()
	dbs := setupClusterTestDB(t)
	seedArticleRecData(t, ctx, dbs)

	engine := newTestEngine(dbs)
	assert.NilError(t, engine.ComputeFeedSimilarity(ctx))
	assert.NilError(t, engine.ComputeUserSimilarity(ctx))
	assert.NilError(t, engine.ComputeArticleEmbeddings(ctx))

	recs, err := engine.GetArticleRecommendations(ctx, "did:test:alice", nil, 10)
	assert.NilError(t, err)
	assert.Assert(t, len(recs) > 0, "alice should get all recommendations without language filter")

	urls := make(map[string]bool)
	for _, r := range recs {
		urls[r.URL] = true
	}
	assert.Assert(t, urls["https://tech.com/go"], "should include 'en' article")
	assert.Assert(t, urls["https://dev.com/rust"], "should include 'fr' article")
	assert.Assert(t, urls["https://dev.com/cook"], "should include unclassified article")
	assert.Assert(t, urls["https://dev.com/python"], "should include 'de' article")
}

func TestArticleRecommendations_LanguageFilter_EmptyPreferences(t *testing.T) {
	ctx := context.Background()
	dbs := setupClusterTestDB(t)
	seedArticleRecData(t, ctx, dbs)

	engine := newTestEngine(dbs)
	assert.NilError(t, engine.ComputeFeedSimilarity(ctx))
	assert.NilError(t, engine.ComputeUserSimilarity(ctx))
	assert.NilError(t, engine.ComputeArticleEmbeddings(ctx))

	recs, err := engine.GetArticleRecommendations(ctx, "did:test:alice", []string{}, 10)
	assert.NilError(t, err)
	assert.Assert(t, len(recs) > 0, "empty language list should show all articles")

	urls := make(map[string]bool)
	for _, r := range recs {
		urls[r.URL] = true
	}
	assert.Assert(t, urls["https://tech.com/go"], "should include 'en' article")
	assert.Assert(t, urls["https://dev.com/rust"], "should include 'fr' article")
	assert.Assert(t, urls["https://dev.com/cook"], "should include unclassified article")
	assert.Assert(t, urls["https://dev.com/python"], "should include 'de' article")
}

func TestFeedEmbeddingRecomputedOnDescriptionChange(t *testing.T) {
	ctx := context.Background()
	dbs := setupClusterTestDB(t)

	_, err := dbs.SQLDB().ExecContext(ctx, `INSERT INTO articles.feeds (feed_url, title, site_url, description, feed_type) VALUES (?, ?, ?, ?, 'rss')`,
		"https://go.com/feed", "Go Blog", "https://go.com", "old description")
	assert.NilError(t, err)

	engine := newTestEngine(dbs)

	assert.NilError(t, engine.ComputeFeedEmbeddings(ctx))

	var count int
	assert.NilError(t, dbs.SQLDB().QueryRowContext(ctx, `SELECT COUNT(*) FROM recs.feed_embeddings`).Scan(&count))
	assert.Equal(t, count, 1)

	_, err = dbs.SQLDB().ExecContext(ctx, `UPDATE articles.feeds SET description = 'new description' WHERE feed_url = 'https://go.com/feed'`)
	assert.NilError(t, err)

	assert.NilError(t, engine.ComputeFeedEmbeddings(ctx))

	var sourceText string
	assert.NilError(t, dbs.SQLDB().QueryRowContext(ctx, `SELECT source_text FROM recs.feed_embedding_meta WHERE feed_url = 'https://go.com/feed'`).Scan(&sourceText))
	assert.Assert(t, sourceText == "Go Blog new description", "embedding should be recomputed when description changes, got: %s", sourceText)
}

func TestTimeDecayedFeedSimilarity(t *testing.T) {
	ctx := context.Background()
	dbs := setupClusterTestDB(t)

	_, err := dbs.SQLDB().ExecContext(ctx, `INSERT INTO users (did) VALUES (?)`, "did:test:alice")
	assert.NilError(t, err)
	_, err = dbs.SQLDB().ExecContext(ctx, `INSERT INTO users (did) VALUES (?)`, "did:test:bob")
	assert.NilError(t, err)

	_, err = dbs.SQLDB().ExecContext(ctx, `INSERT INTO articles.feeds (feed_url, title, site_url, feed_type, subscriber_count) VALUES (?, ?, ?, 'rss', 2)`,
		"https://a.com/feed", "Feed A", "https://a.com")
	assert.NilError(t, err)
	_, err = dbs.SQLDB().ExecContext(ctx, `INSERT INTO articles.feeds (feed_url, title, site_url, feed_type, subscriber_count) VALUES (?, ?, ?, 'rss', 2)`,
		"https://b.com/feed", "Feed B", "https://b.com")
	assert.NilError(t, err)

	_, err = dbs.SQLDB().ExecContext(ctx, `INSERT INTO articles.subscriptions (user_did, feed_url, added_at) VALUES (?, ?, datetime('now', '-60 days'))`,
		"did:test:alice", "https://a.com/feed")
	assert.NilError(t, err)
	_, err = dbs.SQLDB().ExecContext(ctx, `INSERT INTO articles.subscriptions (user_did, feed_url, added_at) VALUES (?, ?, datetime('now'))`,
		"did:test:bob", "https://a.com/feed")
	assert.NilError(t, err)
	_, err = dbs.SQLDB().ExecContext(ctx, `INSERT INTO articles.subscriptions (user_did, feed_url, added_at) VALUES (?, ?, datetime('now'))`,
		"did:test:alice", "https://b.com/feed")
	assert.NilError(t, err)
	_, err = dbs.SQLDB().ExecContext(ctx, `INSERT INTO articles.subscriptions (user_did, feed_url, added_at) VALUES (?, ?, datetime('now'))`,
		"did:test:bob", "https://b.com/feed")
	assert.NilError(t, err)

	engine := newTestEngine(dbs)
	assert.NilError(t, engine.ComputeFeedSimilarity(ctx))

	var jaccard float64
	assert.NilError(t, dbs.SQLDB().QueryRowContext(ctx,
		`SELECT jaccard FROM recs.feed_similarity WHERE feed_a = 'https://a.com/feed' AND feed_b = 'https://b.com/feed'`).Scan(&jaccard))
	assert.Assert(t, jaccard > 0, "time-decayed feed similarity should be positive")
	assert.Assert(t, jaccard < 1.0, "time decay should reduce similarity below raw Jaccard")
}

func TestColdStartFromEmbeddings(t *testing.T) {
	ctx := context.Background()
	dbs := setupClusterTestDB(t)

	_, err := dbs.SQLDB().ExecContext(ctx, `INSERT INTO users (did) VALUES (?)`, "did:test:newuser")
	assert.NilError(t, err)

	_, err = dbs.SQLDB().ExecContext(ctx, `INSERT INTO articles.feeds (feed_url, title, site_url, description, feed_type, subscriber_count) VALUES (?, ?, ?, ?, 'rss', 2)`,
		"https://go.com/feed", "Go Blog", "https://go.com", "golang programming language")
	assert.NilError(t, err)
	_, err = dbs.SQLDB().ExecContext(ctx, `INSERT INTO articles.feeds (feed_url, title, site_url, description, feed_type, subscriber_count) VALUES (?, ?, ?, ?, 'rss', 2)`,
		"https://godev.com/feed", "Go Dev", "https://godev.com", "golang development tutorials")
	assert.NilError(t, err)
	_, err = dbs.SQLDB().ExecContext(ctx, `INSERT INTO articles.feeds (feed_url, title, site_url, description, feed_type, subscriber_count) VALUES (?, ?, ?, ?, 'rss', 2)`,
		"https://cooking.com/feed", "Cooking", "https://cooking.com", "recipes for dinner")
	assert.NilError(t, err)

	_, err = dbs.SQLDB().ExecContext(ctx, `INSERT INTO articles.subscriptions (user_did, feed_url) VALUES (?, ?)`,
		"did:test:newuser", "https://go.com/feed")
	assert.NilError(t, err)

	engine := newTestEngine(dbs)
	assert.NilError(t, engine.ComputeFeedEmbeddings(ctx))

	recs, err := engine.ColdStartRecommendations(ctx, "did:test:newuser", 10)
	assert.NilError(t, err)
	assert.Assert(t, len(recs) > 0, "embedding cold start should return recommendations")

	for _, r := range recs {
		assert.Assert(t, r.FeedURL != "https://go.com/feed", "should not recommend already subscribed feed")
	}
}

func TestNormalizeFeedScores(t *testing.T) {
	recs := []*FeedRecommendation{
		{FeedURL: "a", Score: 10.0},
		{FeedURL: "b", Score: 5.0},
		{FeedURL: "c", Score: 1.0},
	}
	normalizeFeedScores(recs)
	assert.Equal(t, recs[0].Score, 1.0)
	assert.Equal(t, recs[1].Score, 4.0/9.0)
	assert.Equal(t, recs[2].Score, 0.0)
}
