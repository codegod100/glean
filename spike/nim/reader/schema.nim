## The reader's schema.
##
## Two databases: `main` holds authenticated users and their web sessions,
## while `articles` holds feeds and their contents. They are separate files so the article store -- the
## only one that grows without bound -- can be vacuumed or discarded on its
## own.
##
## Originally generated from a Go server's schema and then trimmed to what the
## reader needs. OAuth protocol state lives in the official ATProto client's
## durable store; only verified DIDs and opaque application sessions live here.
##
## `read_state_history` is not redundant with `read_state`. Articles are
## purged on a retention window and can return from the feed later under a
## new surrogate id; the history is keyed on (feed_url, guid), which survives
## that, so a re-ingested article does not come back unread.

const readerSchema* = [
  """CREATE TABLE IF NOT EXISTS users (
		did TEXT PRIMARY KEY,
		indexed_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		follows_dirty BOOLEAN NOT NULL DEFAULT 1
	)""",
  """CREATE TABLE IF NOT EXISTS web_sessions (
		token TEXT PRIMARY KEY,
		user_did TEXT NOT NULL,
		expires_at INTEGER NOT NULL
	)""",
  """CREATE INDEX IF NOT EXISTS idx_web_sessions_expiry ON web_sessions(expires_at)""",
  """CREATE TABLE IF NOT EXISTS articles.feeds (
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
	)""",
  """CREATE TABLE IF NOT EXISTS articles.subscriptions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_did TEXT NOT NULL,
		feed_url TEXT NOT NULL,
		title TEXT,
		category TEXT,
		added_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		uri TEXT,
		cid TEXT,
		UNIQUE(user_did, feed_url)
	)""",
  """CREATE TABLE IF NOT EXISTS articles.articles (
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
		language TEXT NOT NULL DEFAULT '',
		UNIQUE(feed_url, guid)
	)""",
  """CREATE TABLE IF NOT EXISTS articles.read_state (
		user_did TEXT NOT NULL,
		article_id INTEGER NOT NULL,
		is_read BOOLEAN NOT NULL DEFAULT 0,
		read_at DATETIME,
		PRIMARY KEY (user_did, article_id)
	)""",
  """CREATE TABLE IF NOT EXISTS articles.read_state_history (
		user_did TEXT NOT NULL,
		feed_url TEXT NOT NULL,
		guid TEXT NOT NULL,
		is_read BOOLEAN NOT NULL DEFAULT 0,
		read_at DATETIME,
		PRIMARY KEY (user_did, feed_url, guid)
	)""",
  """CREATE INDEX IF NOT EXISTS articles.idx_subscriptions_feed_user ON subscriptions(feed_url, user_did)""",
  """CREATE INDEX IF NOT EXISTS articles.idx_subscriptions_uri ON subscriptions(uri)""",
  """CREATE INDEX IF NOT EXISTS articles.idx_articles_url ON articles(url)""",
  """CREATE INDEX IF NOT EXISTS articles.idx_read_state_unread ON read_state(user_did, is_read) WHERE is_read = 0""",
  """CREATE INDEX IF NOT EXISTS articles.idx_articles_language ON articles(language)""",
  """CREATE VIRTUAL TABLE IF NOT EXISTS articles.articles_fts USING fts5(title, summary, content, author, content=articles, content_rowid=id)""",
  """CREATE TRIGGER IF NOT EXISTS articles.articles_ai AFTER INSERT ON articles BEGIN
		INSERT INTO articles_fts(rowid, title, summary, content, author) VALUES (new.id, new.title, new.summary, new.content, new.author);
	END""",
  """CREATE TRIGGER IF NOT EXISTS articles.articles_ad AFTER DELETE ON articles BEGIN
		INSERT INTO articles_fts(articles_fts, rowid, title, summary, content, author) VALUES('delete', old.id, old.title, old.summary, old.content, old.author);
	END""",
  """CREATE TRIGGER IF NOT EXISTS articles.articles_au AFTER UPDATE ON articles BEGIN
		INSERT INTO articles_fts(articles_fts, rowid, title, summary, content, author) VALUES('delete', old.id, old.title, old.summary, old.content, old.author);
		INSERT INTO articles_fts(rowid, title, summary, content, author) VALUES (new.id, new.title, new.summary, new.content, new.author);
	END""",
]
