package db

import (
	"database/sql"
	"fmt"
	"strings"
)

func init() {
	if len(migrations) != SchemaVersion {
		panic(fmt.Sprintf("SchemaVersion (%d) does not match number of migrations (%d)", SchemaVersion, len(migrations)))
	}
}

// SchemaVersion must be incremented each time a migration is added to the migrations slice (used so that fresh dbs skip running migrations).
const SchemaVersion = 7

type migration struct {
	id   int
	name string
	run  func(db *DB) error
}

var migrations = []migration{
	{
		id:   1,
		name: "add_person_target_type",
		run:  migrateAddPersonTargetType,
	},
	{
		id:   2,
		name: "feed_type_atproto",
		run:  migrateFeedTypeATProto,
	},
	{
		id:   3,
		name: "article_language_user_languages",
		run:  migrateArticleLanguageUserLanguages,
	},
	{
		id:   4,
		name: "jetstream_cursor",
		run:  migrateJetstreamCursor,
	},
	{
		id:   5,
		name: "user_settings_expanded_view",
		run:  migrateUserSettingsExpandedView,
	},
	{
		id:   6,
		name: "drop_redundant_follows_user_index",
		run:  migrateDropFollowsUserIndex,
	},
	{
		id:   7,
		name: "user_settings_digest_enabled",
		run:  migrateUserSettingsDigestEnabled,
	},
}

func runMigrations(db *DB) error {
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		id   INTEGER PRIMARY KEY,
		name TEXT NOT NULL,
		run_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`); err != nil {
		return fmt.Errorf("create schema_migrations table: %w", err)
	}

	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM schema_migrations").Scan(&count); err != nil {
		return fmt.Errorf("count migrations: %w", err)
	}
	if count == 0 {
		for _, m := range migrations {
			if _, err := db.Exec("INSERT INTO schema_migrations (id, name) VALUES (?, ?)", m.id, m.name); err != nil {
				return fmt.Errorf("record migration %d: %w", m.id, err)
			}
		}
		return nil
	}

	for _, m := range migrations {
		var id int
		err := db.QueryRow("SELECT id FROM schema_migrations WHERE id = ?", m.id).Scan(&id)
		if err == nil {
			continue
		}
		if err != sql.ErrNoRows {
			return fmt.Errorf("check migration %d: %w", m.id, err)
		}

		if err := m.run(db); err != nil {
			return fmt.Errorf("migration %d (%s): %w", m.id, m.name, err)
		}

		if _, err := db.Exec("INSERT INTO schema_migrations (id, name) VALUES (?, ?)", m.id, m.name); err != nil {
			return fmt.Errorf("record migration %d: %w", m.id, err)
		}
	}

	return nil
}

func migrateFeedTypeATProto(db *DB) error {
	var checkClause string
	err := db.QueryRow("SELECT sql FROM articles.sqlite_master WHERE type='table' AND name='feeds'").Scan(&checkClause)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil
		}
		return fmt.Errorf("read feeds schema: %w", err)
	}
	if strings.Contains(checkClause, "'atproto'") {
		return nil
	}

	for _, stmt := range []string{
		`CREATE TABLE articles.feeds_new (
			feed_url TEXT PRIMARY KEY,
			title TEXT,
			site_url TEXT,
			description TEXT,
			feed_type TEXT CHECK(feed_type IN ('rss', 'atom', 'json', 'atproto')),
			last_fetched_at DATETIME,
			last_error TEXT,
			subscriber_count INTEGER NOT NULL DEFAULT 0,
			consecutive_empty_fetches INTEGER NOT NULL DEFAULT 0,
			error_count INTEGER NOT NULL DEFAULT 0,
			favicon_url TEXT
		)`,
		`INSERT INTO articles.feeds_new (feed_url, title, site_url, description, feed_type, last_fetched_at, last_error, subscriber_count, consecutive_empty_fetches, error_count, favicon_url)
		 SELECT feed_url, title, site_url, description, feed_type, last_fetched_at, last_error, subscriber_count, consecutive_empty_fetches, error_count, favicon_url FROM articles.feeds`,
		`DROP TABLE articles.feeds`,
		`ALTER TABLE articles.feeds_new RENAME TO feeds`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("migrate feed_type atproto: %w", err)
		}
	}

	return nil
}

func migrateAddPersonTargetType(db *DB) error {
	for _, m := range []struct {
		table   string
		create  string
		columns string
		index   string
	}{
		{
			table: "dismissed_recommendations",
			create: `CREATE TABLE dismissed_recommendations_new (
				user_did     TEXT NOT NULL,
				target_type  TEXT NOT NULL CHECK(target_type IN ('feed', 'article', 'person')),
				target_id    TEXT NOT NULL,
				reason       TEXT,
				dismissed_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
				PRIMARY KEY (user_did, target_type, target_id)
			)`,
			columns: "user_did, target_type, target_id, reason, dismissed_at",
			index:   "idx_dismissed_user_type",
		},
		{
			table: "recommendation_impressions",
			create: `CREATE TABLE recommendation_impressions_new (
				user_did       TEXT NOT NULL,
				target_type    TEXT NOT NULL CHECK(target_type IN ('feed', 'article', 'person')),
				target_id      TEXT NOT NULL,
				first_shown_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
				last_shown_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
				shown_count    INTEGER NOT NULL DEFAULT 1,
				acted          BOOLEAN NOT NULL DEFAULT 0,
				PRIMARY KEY (user_did, target_type, target_id)
			)`,
			columns: "user_did, target_type, target_id, first_shown_at, last_shown_at, shown_count, acted",
			index:   "idx_impressions_user_unacted",
		},
	} {
		var schema string
		_ = db.QueryRow("SELECT sql FROM sqlite_master WHERE type='table' AND name=?", m.table).Scan(&schema)
		if strings.Contains(schema, "'person'") {
			continue
		}

		db.Exec(fmt.Sprintf("DROP TABLE IF EXISTS %s_new", m.table))

		for _, stmt := range []string{
			m.create,
			fmt.Sprintf("INSERT INTO %s_new (%s) SELECT %s FROM %s", m.table, m.columns, m.columns, m.table),
			fmt.Sprintf("DROP TABLE %s", m.table),
			fmt.Sprintf("ALTER TABLE %s_new RENAME TO %s", m.table, m.table),
			fmt.Sprintf("CREATE INDEX IF NOT EXISTS %s ON %s(user_did, target_type)", m.index, m.table),
		} {
			if _, err := db.Exec(stmt); err != nil {
				return fmt.Errorf("migrating %s: %w", m.table, err)
			}
		}
	}

	if _, err := db.Exec("CREATE INDEX IF NOT EXISTS idx_impressions_last_shown ON recommendation_impressions(last_shown_at)"); err != nil {
		return err
	}

	return nil
}

func migrateArticleLanguageUserLanguages(db *DB) error {
	var colCount int
	err := db.QueryRow("SELECT COUNT(*) FROM pragma_table_info('articles.articles') WHERE name='language'").Scan(&colCount)
	if err != nil {
		return fmt.Errorf("check articles.language column: %w", err)
	}
	if colCount == 0 {
		if _, err := db.Exec("ALTER TABLE articles.articles ADD COLUMN language TEXT NOT NULL DEFAULT ''"); err != nil {
			return fmt.Errorf("add articles.language: %w", err)
		}
		if _, err := db.Exec("CREATE INDEX IF NOT EXISTS articles.idx_articles_language ON articles(language)"); err != nil {
			return fmt.Errorf("create articles.language index: %w", err)
		}
	}

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS user_settings (
		did TEXT PRIMARY KEY,
		languages TEXT,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`)
	if err != nil {
		return fmt.Errorf("create user_settings: %w", err)
	}

	return nil
}

func migrateJetstreamCursor(db *DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS jetstream_cursor (
		id INTEGER PRIMARY KEY CHECK(id = 1),
		cursor_us INTEGER NOT NULL
	)`)
	if err != nil {
		return fmt.Errorf("create jetstream_cursor: %w", err)
	}
	return nil
}

func migrateUserSettingsExpandedView(db *DB) error {
	var colCount int
	err := db.QueryRow("SELECT COUNT(*) FROM pragma_table_info('user_settings') WHERE name='expanded_view'").Scan(&colCount)
	if err != nil {
		return fmt.Errorf("check user_settings.expanded_view column: %w", err)
	}
	if colCount == 0 {
		if _, err := db.Exec("ALTER TABLE user_settings ADD COLUMN expanded_view BOOLEAN NOT NULL DEFAULT 0"); err != nil {
			return fmt.Errorf("add user_settings.expanded_view: %w", err)
		}
	}
	return nil
}

func migrateUserSettingsDigestEnabled(db *DB) error {
	var colCount int
	err := db.QueryRow("SELECT COUNT(*) FROM pragma_table_info('user_settings') WHERE name='digest_enabled'").Scan(&colCount)
	if err != nil {
		return fmt.Errorf("check user_settings.digest_enabled column: %w", err)
	}
	if colCount == 0 {
		if _, err := db.Exec("ALTER TABLE user_settings ADD COLUMN digest_enabled BOOLEAN NOT NULL DEFAULT 0"); err != nil {
			return fmt.Errorf("add user_settings.digest_enabled: %w", err)
		}
	}
	return nil
}

// indexSchema maps an index name to the schema (and attached db prefix) it lives in.
// Empty prefix means the main database.
func indexSchema(idx string) string {
	switch {
	case strings.HasPrefix(idx, "idx_subscriptions_"),
		strings.HasPrefix(idx, "idx_articles_"),
		strings.HasPrefix(idx, "idx_likes_"):
		return "articles"
	case strings.HasPrefix(idx, "idx_follow_distances_"),
		strings.HasPrefix(idx, "idx_user_similarity_"):
		return "recs"
	default:
		return ""
	}
}

// indexExists reports whether the given index exists in its schema's sqlite_master.
func indexExists(db *DB, schema, idx string) bool {
	table := "sqlite_master"
	if schema != "" {
		table = schema + ".sqlite_master"
	}
	var name string
	_ = db.QueryRow(fmt.Sprintf("SELECT name FROM %s WHERE type='index' AND name=?", table), idx).Scan(&name)
	return name != ""
}

// dropIndex drops an index from its schema. Empty schema means the main database.
func dropIndex(db *DB, schema, idx string) error {
	target := idx
	if schema != "" {
		target = schema + "." + idx
	}
	if _, err := db.Exec(fmt.Sprintf("DROP INDEX IF EXISTS %s", target)); err != nil {
		return fmt.Errorf("drop %s: %w", idx, err)
	}
	return nil
}

func migrateDropFollowsUserIndex(db *DB) error {
	for _, idx := range []string{
		"idx_follows_user",
		"idx_follows_followed_at",
		"idx_dismissed_user_type",
		"idx_impressions_last_shown",
		"idx_subscriptions_feed",
		"idx_subscriptions_user",
		"idx_subscriptions_user_feed",
		"idx_articles_feed",
		"idx_articles_published",
		"idx_likes_article",
		"idx_likes_author",
		"idx_follow_distances_b",
		"idx_user_similarity_a",
	} {
		schema := indexSchema(idx)
		if !indexExists(db, schema, idx) {
			continue
		}
		if err := dropIndex(db, schema, idx); err != nil {
			return err
		}
	}
	return nil
}
