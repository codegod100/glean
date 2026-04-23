package db

import (
	"database/sql"
	"fmt"
	"math"
	"sync/atomic"
	"time"

	"github.com/mattn/go-sqlite3"
)

type Databases struct {
	Users    *UserStore
	Articles *ArticleStore

	db *DB
}

var multiDriverSeq int64

func OpenAll(basePath string) (*Databases, error) {
	articlesPath := basePath + "_articles"
	recsPath := basePath + "_recs"

	for _, p := range []string{articlesPath, recsPath} {
		f, err := sql.Open("sqlite3", p+"?"+DSN)
		if err != nil {
			return nil, err
		}
		f.Close()
	}

	seq := atomic.AddInt64(&multiDriverSeq, 1)
	driverName := fmt.Sprintf("sqlite3_glean_multi_%d", seq)

	sql.Register(driverName, &sqlite3.SQLiteDriver{
		ConnectHook: func(conn *sqlite3.SQLiteConn) error {
			if err := conn.RegisterFunc("exp", func(x float64) float64 { return math.Exp(x) }, true); err != nil {
				return err
			}
			if err := conn.RegisterFunc("log", func(x float64) float64 { return math.Log(x) }, true); err != nil {
				return err
			}
			for _, p := range []string{
				`PRAGMA wal_autocheckpoint = 1000`,
				`PRAGMA temp_store = MEMORY`,
				`PRAGMA mmap_size = 268435456`,
			} {
				if _, err := conn.Exec(p, nil); err != nil {
					return err
				}
			}
			if _, err := conn.Exec(fmt.Sprintf("ATTACH DATABASE '%s' AS articles", articlesPath), nil); err != nil {
				return err
			}
			if _, err := conn.Exec(fmt.Sprintf("ATTACH DATABASE '%s' AS recs", recsPath), nil); err != nil {
				return err
			}
			return nil
		},
	})

	db, err := sql.Open(driverName, basePath+"_users?cache=shared&"+DSN)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)
	d := &DB{db}

	if err := initUsersSchema(d); err != nil {
		d.Close()
		return nil, err
	}

	if err := initArticlesSchema(d); err != nil {
		d.Close()
		return nil, err
	}

	if err := initRecsSchema(d); err != nil {
		d.Close()
		return nil, err
	}

	return &Databases{
		Users:    NewUserStore(d),
		Articles: NewArticleStore(d),
		db:       d,
	}, nil
}

func (d *Databases) Close() error {
	if d.db != nil {
		_ = d.db.Close()
	}
	return nil
}

func (d *Databases) DB() *sql.DB {
	return d.db.DB
}

func initUsersSchema(db *DB) error {
	for _, s := range usersSchema {
		if _, err := db.Exec(s); err != nil {
			return err
		}
	}
	return nil
}

func initArticlesSchema(db *DB) error {
	for _, s := range articlesSchema {
		if _, err := db.Exec(s); err != nil {
			return err
		}
	}
	return nil
}

func initRecsSchema(db *DB) error {
	for _, s := range recsSchema {
		if _, err := db.Exec(s); err != nil {
			return err
		}
	}
	return nil
}

var usersSchema = []string{
	`CREATE TABLE IF NOT EXISTS users (
		did TEXT PRIMARY KEY,
		handle TEXT NOT NULL,
		display_name TEXT,
		avatar_url TEXT,
		indexed_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`,

	`CREATE TABLE IF NOT EXISTS follows (
		user_did TEXT NOT NULL,
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

	`CREATE INDEX IF NOT EXISTS idx_follows_user ON follows(user_did)`,
	`CREATE INDEX IF NOT EXISTS idx_follows_target ON follows(target_did)`,
	`CREATE INDEX IF NOT EXISTS idx_follows_uri ON follows(uri)`,
	`CREATE INDEX IF NOT EXISTS idx_follows_followed_at ON follows(followed_at)`,
	`CREATE INDEX IF NOT EXISTS idx_users_handle ON users(handle)`,
}

var articlesSchema = []string{
	`CREATE TABLE IF NOT EXISTS articles.feeds (
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
		consecutive_empty_fetches INTEGER NOT NULL DEFAULT 0,
		error_count INTEGER NOT NULL DEFAULT 0,
		favicon_url TEXT
	)`,

	`CREATE TABLE IF NOT EXISTS articles.subscriptions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_did TEXT NOT NULL,
		feed_url TEXT NOT NULL,
		title TEXT,
		category TEXT,
		added_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		uri TEXT,
		cid TEXT,
		UNIQUE(user_did, feed_url)
	)`,

	`CREATE TABLE IF NOT EXISTS articles.articles (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		feed_url TEXT NOT NULL,
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

	`CREATE TABLE IF NOT EXISTS articles.read_state (
		user_did TEXT NOT NULL,
		article_id INTEGER NOT NULL,
		is_read BOOLEAN NOT NULL DEFAULT 0,
		read_at DATETIME,
		PRIMARY KEY (user_did, article_id)
	)`,

	`CREATE TABLE IF NOT EXISTS articles.annotations (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		uri TEXT NOT NULL UNIQUE,
		author_did TEXT NOT NULL,
		feed_url TEXT NOT NULL,
		article_url TEXT NOT NULL,
		quote TEXT,
		note TEXT,
		tags TEXT,
		rating INTEGER,
		created_at DATETIME NOT NULL,
		cid TEXT
	)`,

	`CREATE TABLE IF NOT EXISTS articles.likes (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		uri TEXT NOT NULL UNIQUE,
		author_did TEXT NOT NULL,
		feed_url TEXT NOT NULL,
		article_url TEXT NOT NULL,
		created_at DATETIME NOT NULL,
		cid TEXT,
		UNIQUE(author_did, feed_url, article_url)
	)`,

	`CREATE INDEX IF NOT EXISTS articles.idx_subscriptions_feed ON subscriptions(feed_url)`,
	`CREATE INDEX IF NOT EXISTS articles.idx_subscriptions_feed_user ON subscriptions(feed_url, user_did)`,
	`CREATE INDEX IF NOT EXISTS articles.idx_subscriptions_user ON subscriptions(user_did)`,
	`CREATE INDEX IF NOT EXISTS articles.idx_subscriptions_uri ON subscriptions(uri)`,
	`CREATE INDEX IF NOT EXISTS articles.idx_likes_author_feed ON likes(author_did, feed_url, created_at)`,
	`CREATE INDEX IF NOT EXISTS articles.idx_articles_feed ON articles(feed_url)`,
	`CREATE INDEX IF NOT EXISTS articles.idx_articles_published ON articles(published DESC)`,
	`CREATE INDEX IF NOT EXISTS articles.idx_articles_url ON articles(url)`,
	`CREATE INDEX IF NOT EXISTS articles.idx_read_state_unread ON read_state(user_did, is_read) WHERE is_read = 0`,
	`CREATE INDEX IF NOT EXISTS articles.idx_annotations_article ON annotations(article_url)`,
	`CREATE INDEX IF NOT EXISTS articles.idx_annotations_author ON annotations(author_did)`,
	`CREATE INDEX IF NOT EXISTS articles.idx_annotations_created_at ON annotations(created_at DESC)`,
	`CREATE INDEX IF NOT EXISTS articles.idx_likes_article ON likes(feed_url, article_url)`,
	`CREATE INDEX IF NOT EXISTS articles.idx_likes_author ON likes(author_did)`,
	`CREATE INDEX IF NOT EXISTS articles.idx_likes_created_at ON likes(created_at DESC)`,

	`CREATE VIRTUAL TABLE IF NOT EXISTS articles.articles_fts USING fts5(title, summary, content, author, content=articles, content_rowid=id)`,
	`CREATE TRIGGER IF NOT EXISTS articles.articles_ai AFTER INSERT ON articles BEGIN
		INSERT INTO articles_fts(rowid, title, summary, content, author) VALUES (new.id, new.title, new.summary, new.content, new.author);
	END`,
	`CREATE TRIGGER IF NOT EXISTS articles.articles_ad AFTER DELETE ON articles BEGIN
		INSERT INTO articles_fts(articles_fts, rowid, title, summary, content, author) VALUES('delete', old.id, old.title, old.summary, old.content, old.author);
	END`,
	`CREATE TRIGGER IF NOT EXISTS articles.articles_au AFTER UPDATE ON articles BEGIN
		INSERT INTO articles_fts(articles_fts, rowid, title, summary, content, author) VALUES('delete', old.id, old.title, old.summary, old.content, old.author);
		INSERT INTO articles_fts(rowid, title, summary, content, author) VALUES (new.id, new.title, new.summary, new.content, new.author);
	END`,
}

var recsSchema = []string{
	`CREATE TABLE IF NOT EXISTS recs.feed_similarity (
		feed_a TEXT NOT NULL,
		feed_b TEXT NOT NULL,
		jaccard REAL NOT NULL,
		computed_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (feed_a, feed_b),
		CHECK(feed_a < feed_b)
	)`,

	`CREATE TABLE IF NOT EXISTS recs.user_similarity (
		user_a TEXT NOT NULL,
		user_b TEXT NOT NULL,
		jaccard REAL NOT NULL,
		common_feeds INTEGER NOT NULL,
		common_likes INTEGER NOT NULL DEFAULT 0,
		common_tags INTEGER NOT NULL DEFAULT 0,
		computed_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (user_a, user_b),
		CHECK(user_a < user_b)
	)`,

	`CREATE TABLE IF NOT EXISTS recs.dismissed_recommendations (
		user_did     TEXT NOT NULL,
		target_type  TEXT NOT NULL CHECK(target_type IN ('feed', 'article')),
		target_id    TEXT NOT NULL,
		reason       TEXT,
		dismissed_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (user_did, target_type, target_id)
	)`,

	`CREATE TABLE IF NOT EXISTS recs.recommendation_impressions (
		user_did       TEXT NOT NULL,
		target_type    TEXT NOT NULL CHECK(target_type IN ('feed', 'article')),
		target_id      TEXT NOT NULL,
		first_shown_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		last_shown_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		shown_count    INTEGER NOT NULL DEFAULT 1,
		acted          BOOLEAN NOT NULL DEFAULT 0,
		PRIMARY KEY (user_did, target_type, target_id)
	)`,

	`CREATE TABLE IF NOT EXISTS recs.follow_distances (
		user_a   TEXT NOT NULL,
		user_b   TEXT NOT NULL,
		distance INTEGER NOT NULL CHECK(distance IN (1, 2)),
		PRIMARY KEY (user_a, user_b)
	)`,

	`CREATE TABLE IF NOT EXISTS recs.user_signal_weights (
		user_did   TEXT PRIMARY KEY,
		w_sub      REAL NOT NULL DEFAULT 1.0,
		w_like     REAL NOT NULL DEFAULT 0.5,
		w_tag      REAL NOT NULL DEFAULT 0.3,
		w_social   REAL NOT NULL DEFAULT 0.7,
		w_pop      REAL NOT NULL DEFAULT 0.2,
		w_category REAL NOT NULL DEFAULT 0.4,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`,

	`CREATE TABLE IF NOT EXISTS recs.user_signal_profiles (
		user_did       TEXT PRIMARY KEY,
		total_likes     INTEGER NOT NULL DEFAULT 0,
		total_tags      INTEGER NOT NULL DEFAULT 0,
		top_categories  TEXT,
		updated_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`,

	`CREATE INDEX IF NOT EXISTS recs.idx_dismissed_user_type ON dismissed_recommendations(user_did, target_type)`,
	`CREATE INDEX IF NOT EXISTS recs.idx_impressions_user_unacted ON recommendation_impressions(user_did, acted, shown_count)`,
	`CREATE INDEX IF NOT EXISTS recs.idx_impressions_last_shown ON recommendation_impressions(last_shown_at)`,
	`CREATE INDEX IF NOT EXISTS recs.idx_follow_distances_b ON follow_distances(user_b)`,
	`CREATE INDEX IF NOT EXISTS recs.idx_follow_distances_a_dist ON follow_distances(user_a, distance)`,
	`CREATE INDEX IF NOT EXISTS recs.idx_user_similarity_b ON user_similarity(user_b)`,
	`CREATE INDEX IF NOT EXISTS recs.idx_user_similarity_a ON user_similarity(user_a)`,
}
