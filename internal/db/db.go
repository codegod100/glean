package db

import (
	"database/sql"
	_ "github.com/mattn/go-sqlite3"
	"strings"
	"time"
)

func NullStr(s string) sql.NullString {
	return sql.NullString{String: s, Valid: s != ""}
}

func NullTime(t time.Time) sql.NullTime {
	return sql.NullTime{Time: t, Valid: !t.IsZero()}
}

func NullInt(n int64) sql.NullInt64 {
	return sql.NullInt64{Int64: n, Valid: true}
}

func NullStrTags(tags []string) sql.NullString {
	if len(tags) == 0 {
		return sql.NullString{}
	}
	return sql.NullString{String: strings.Join(tags, ","), Valid: true}
}

type DB struct {
	*sql.DB
}

func Open(path string) (*DB, error) {
	db, err := sql.Open("sqlite3", path+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(30 * time.Minute)

	if err := initSchema(db); err != nil {
		db.Close()
		return nil, err
	}

	return &DB{db}, nil
}

func initSchema(db *sql.DB) error {
	for _, s := range schema {
		if _, err := db.Exec(s); err != nil {
			return err
		}
	}
	return nil
}

var schema = []string{
	`CREATE TABLE IF NOT EXISTS users (
		did TEXT PRIMARY KEY,
		handle TEXT NOT NULL,
		display_name TEXT,
		avatar_url TEXT,
		indexed_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`,
	`CREATE TABLE IF NOT EXISTS feeds (
		feed_url TEXT PRIMARY KEY,
		title TEXT,
		site_url TEXT,
		description TEXT,
		feed_type TEXT CHECK(feed_type IN ('rss', 'atom', 'json')),
		last_fetched_at DATETIME,
		last_error TEXT,
		subscriber_count INTEGER NOT NULL DEFAULT 0,
		etag TEXT,
		last_modified TEXT,
		fetch_interval_minutes INTEGER NOT NULL DEFAULT 30,
		next_fetch_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		consecutive_empty_fetches INTEGER NOT NULL DEFAULT 0,
		error_count INTEGER NOT NULL DEFAULT 0,
		favicon_url TEXT
	)`,
	`CREATE TABLE IF NOT EXISTS subscriptions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_did TEXT NOT NULL REFERENCES users(did),
		feed_url TEXT NOT NULL REFERENCES feeds(feed_url),
		title TEXT,
		category TEXT,
		added_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		uri TEXT,
		cid TEXT,
		UNIQUE(user_did, feed_url)
	)`,
	`CREATE TABLE IF NOT EXISTS articles (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		feed_url TEXT NOT NULL REFERENCES feeds(feed_url),
		guid TEXT NOT NULL,
		title TEXT NOT NULL DEFAULT '',
		url TEXT,
		author TEXT,
		summary TEXT,
		content TEXT,
		full_content TEXT,
		published DATETIME,
		updated DATETIME,
		fetched_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(feed_url, guid)
	)`,
	`CREATE TABLE IF NOT EXISTS read_state (
		user_did TEXT NOT NULL REFERENCES users(did),
		article_id INTEGER NOT NULL REFERENCES articles(id),
		is_read BOOLEAN NOT NULL DEFAULT 0,
		read_at DATETIME,
		PRIMARY KEY (user_did, article_id)
	)`,
	`CREATE TABLE IF NOT EXISTS annotations (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		uri TEXT NOT NULL UNIQUE,
		author_did TEXT NOT NULL REFERENCES users(did),
		feed_url TEXT NOT NULL,
		article_url TEXT NOT NULL,
		quote TEXT,
		note TEXT,
		tags TEXT,
		rating INTEGER,
		created_at DATETIME NOT NULL,
		cid TEXT
	)`,
	`CREATE TABLE IF NOT EXISTS likes (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		uri TEXT NOT NULL UNIQUE,
		author_did TEXT NOT NULL REFERENCES users(did),
		feed_url TEXT NOT NULL,
		article_url TEXT NOT NULL,
		created_at DATETIME NOT NULL,
		cid TEXT,
		UNIQUE(author_did, feed_url, article_url)
	)`,
	`CREATE TABLE IF NOT EXISTS feed_similarity (
		feed_a TEXT NOT NULL REFERENCES feeds(feed_url),
		feed_b TEXT NOT NULL REFERENCES feeds(feed_url),
		jaccard REAL NOT NULL,
		computed_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (feed_a, feed_b),
		CHECK(feed_a < feed_b)
	)`,
	`CREATE TABLE IF NOT EXISTS user_similarity (
		user_a TEXT NOT NULL REFERENCES users(did),
		user_b TEXT NOT NULL REFERENCES users(did),
		jaccard REAL NOT NULL,
		common_feeds INTEGER NOT NULL,
		common_likes INTEGER NOT NULL DEFAULT 0,
		common_tags INTEGER NOT NULL DEFAULT 0,
		computed_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (user_a, user_b),
		CHECK(user_a < user_b)
	)`,
	`CREATE TABLE IF NOT EXISTS user_feed_recommendations (
		user_did TEXT NOT NULL REFERENCES users(did),
		feed_url TEXT NOT NULL REFERENCES feeds(feed_url),
		score REAL NOT NULL,
		computed_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (user_did, feed_url)
	)`,
	`CREATE TABLE IF NOT EXISTS user_article_recommendations (
		user_did TEXT NOT NULL REFERENCES users(did),
		feed_url TEXT NOT NULL,
		article_url TEXT NOT NULL,
		score REAL NOT NULL,
		computed_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (user_did, feed_url, article_url)
	)`,
	`CREATE TABLE IF NOT EXISTS follows (
		user_did TEXT NOT NULL REFERENCES users(did),
		target_did TEXT NOT NULL,
		uri TEXT,
		cid TEXT,
		followed_at DATETIME,
		PRIMARY KEY (user_did, target_did)
	)`,
	`CREATE TABLE IF NOT EXISTS oauth_auth_requests (
		state TEXT PRIMARY KEY,
		data TEXT NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS oauth_sessions (
		account_did TEXT NOT NULL,
		session_id TEXT NOT NULL,
		data TEXT NOT NULL,
		PRIMARY KEY (account_did, session_id)
	)`,
	`CREATE INDEX IF NOT EXISTS idx_subscriptions_feed ON subscriptions(feed_url)`,
	`CREATE INDEX IF NOT EXISTS idx_subscriptions_user ON subscriptions(user_did)`,
	`CREATE INDEX IF NOT EXISTS idx_subscriptions_uri ON subscriptions(uri)`,
	`CREATE INDEX IF NOT EXISTS idx_articles_feed ON articles(feed_url)`,
	`CREATE INDEX IF NOT EXISTS idx_articles_published ON articles(published DESC)`,
	`CREATE INDEX IF NOT EXISTS idx_articles_url ON articles(url)`,
	`CREATE INDEX IF NOT EXISTS idx_read_state_unread ON read_state(user_did, is_read) WHERE is_read = 0`,
	`CREATE INDEX IF NOT EXISTS idx_annotations_article ON annotations(article_url)`,
	`CREATE INDEX IF NOT EXISTS idx_annotations_author ON annotations(author_did)`,
	`CREATE INDEX IF NOT EXISTS idx_annotations_created_at ON annotations(created_at DESC)`,
	`CREATE INDEX IF NOT EXISTS idx_likes_article ON likes(feed_url, article_url)`,
	`CREATE INDEX IF NOT EXISTS idx_likes_author ON likes(author_did)`,
	`CREATE INDEX IF NOT EXISTS idx_likes_created_at ON likes(created_at DESC)`,
	`CREATE INDEX IF NOT EXISTS idx_follows_user ON follows(user_did)`,
	`CREATE INDEX IF NOT EXISTS idx_follows_target ON follows(target_did)`,
	`CREATE INDEX IF NOT EXISTS idx_follows_uri ON follows(uri)`,
	`CREATE INDEX IF NOT EXISTS idx_user_similarity_b ON user_similarity(user_b)`,
	`CREATE INDEX IF NOT EXISTS idx_users_handle ON users(handle)`,
	`CREATE VIRTUAL TABLE IF NOT EXISTS articles_fts USING fts5(title, summary, content, author, content=articles, content_rowid=id)`,
	`CREATE TRIGGER IF NOT EXISTS articles_ai AFTER INSERT ON articles BEGIN
		INSERT INTO articles_fts(rowid, title, summary, content, author) VALUES (new.id, new.title, new.summary, new.content, new.author);
	END`,
	`CREATE TRIGGER IF NOT EXISTS articles_ad AFTER DELETE ON articles BEGIN
		INSERT INTO articles_fts(articles_fts, rowid, title, summary, content, author) VALUES('delete', old.id, old.title, old.summary, old.content, old.author);
	END`,
	`CREATE TRIGGER IF NOT EXISTS articles_au AFTER UPDATE ON articles BEGIN
		INSERT INTO articles_fts(articles_fts, rowid, title, summary, content, author) VALUES('delete', old.id, old.title, old.summary, old.content, old.author);
		INSERT INTO articles_fts(rowid, title, summary, content, author) VALUES (new.id, new.title, new.summary, new.content, new.author);
	END`,
}
