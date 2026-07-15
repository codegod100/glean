package db

import (
	"context"
	"fmt"
	"testing"

	"gotest.tools/v3/assert"
)

func TestIndexSchema(t *testing.T) {
	cases := []struct {
		idx  string
		want string
	}{
		{"idx_subscriptions_feed", "articles"},
		{"idx_articles_published", "articles"},
		{"idx_likes_article", "articles"},
		{"idx_follow_distances_b", "recs"},
		{"idx_user_similarity_a", "recs"},
		{"idx_follows_user", ""},
		{"idx_dismissed_user_type", ""},
	}
	for _, tc := range cases {
		t.Run(tc.idx, func(t *testing.T) {
			if got := indexSchema(tc.idx); got != tc.want {
				t.Fatalf("indexSchema(%q) = %q, want %q", tc.idx, got, tc.want)
			}
		})
	}
}

func TestIndexExistsAndDropIndex(t *testing.T) {
	ctx := context.Background()
	store := setupTestDB(t)
	db := &DB{DB: store.SQLDB()}

	// Create a real index in the main schema to exercise the empty-schema path.
	_, err := db.ExecContext(ctx, `CREATE TABLE t (x)`)
	assert.NilError(t, err)
	_, err = db.ExecContext(ctx, `CREATE INDEX idx_follows_user ON t(x)`)
	assert.NilError(t, err)

	if !indexExists(db, "", "idx_follows_user") {
		t.Fatal("indexExists should report true for existing main-schema index")
	}
	if indexExists(db, "", "idx_does_not_exist") {
		t.Fatal("indexExists should report false for missing index")
	}

	// articles-attached index: schema prefix goes on the index name, not the table.
	_, err = db.ExecContext(ctx, `CREATE INDEX articles.idx_articles_feed ON subscriptions(feed_url)`)
	assert.NilError(t, err)
	if !indexExists(db, "articles", "idx_articles_feed") {
		t.Fatal("indexExists should report true for existing articles-schema index")
	}

	assert.NilError(t, dropIndex(db, "articles", "idx_articles_feed"))
	if indexExists(db, "articles", "idx_articles_feed") {
		t.Fatal("dropIndex should have removed the articles index")
	}

	assert.NilError(t, dropIndex(db, "", "idx_follows_user"))
	if indexExists(db, "", "idx_follows_user") {
		t.Fatal("dropIndex should have removed the main-schema index")
	}
}

func TestMigrateDropFollowsUserIndex_Idempotent(t *testing.T) {
	store := setupTestDB(t)
	db := &DB{DB: store.SQLDB()}

	// Should run cleanly against a fresh schema where none of the legacy indexes exist.
	assert.NilError(t, migrateDropFollowsUserIndex(db))
	// Running twice must not error (DROP INDEX IF EXISTS is idempotent).
	assert.NilError(t, migrateDropFollowsUserIndex(db))
}

func TestMigrateDropFollowsUserIndex_DropsLegacyIndexes(t *testing.T) {
	ctx := context.Background()
	store := setupTestDB(t)
	db := &DB{DB: store.SQLDB()}

	_, err := db.ExecContext(ctx, `CREATE TABLE t (x)`)
	assert.NilError(t, err)

	legacy := []string{
		"idx_follows_user",      // main
		"idx_articles_feed",     // articles
		"idx_user_similarity_a", // recs
	}
	for _, idx := range legacy {
		schema := indexSchema(idx)
		switch schema {
		case "articles":
			_, err = db.ExecContext(ctx, fmt.Sprintf(`CREATE INDEX articles.%s ON articles(feed_url)`, idx))
		case "recs":
			_, err = db.ExecContext(ctx, fmt.Sprintf(`CREATE INDEX recs.%s ON user_similarity(user_b)`, idx))
		default:
			_, err = db.ExecContext(ctx, fmt.Sprintf(`CREATE INDEX %s ON t(x)`, idx))
		}
		assert.NilError(t, err, idx)
	}

	assert.NilError(t, migrateDropFollowsUserIndex(db))

	for _, idx := range legacy {
		schema := indexSchema(idx)
		if indexExists(db, schema, idx) {
			t.Fatalf("index %q still present after migration", idx)
		}
	}
}
