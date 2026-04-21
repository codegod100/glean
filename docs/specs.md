# Glean - Design Document

## 1. Overview

Glean is a social RSS reader built on the AT Protocol. It operates as an **AppView** for the `at.glean.*` lexicon namespace: it indexes records from Jetstream, serves XRPC query endpoints, and provides the web UI at [glean.at](https://glean.at).

Users store their RSS feed subscriptions as individual lexicon records on their PDS (one record per feed). Glean's AppView consumes Jetstream, indexes those records, fetches the referenced RSS feeds, and serves both the reader UI and public XRPC APIs for the `at.glean.*` namespace.

The core idea: your RSS subscriptions are a strong signal about your interests. When enough people expose theirs, you can discover both **people** (who reads the same things) and **content** (what similar readers follow that you don't).

## 2. Stack

| Layer            | Technology                                                      |
| ---------------- | --------------------------------------------------------------- |
| Backend          | Go                                                              |
| Database         | SQLite (via `mattn/go-sqlite3`)                                 |
| Frontend         | htmx + TailwindCSS                                              |
| Auth             | AT Protocol OAuth / DID resolution (configurable PLC directory) |
| AT Protocol role | AppView for `at.glean.*` lexicons                               |
| Data source      | AT Protocol Jetstream → SQLite index                            |

## 3. AT Protocol Lexicons

All user data lives on their PDS. The server does not own user data — it indexes and aggregates it.

### 3.1 `at.glean.subscription`

A single RSS feed subscription. One record per feed per user. Created automatically when a user subscribes to a feed. During onboarding, existing subscriptions can be bulk-imported from an OPML file — the OPML is parsed and individual subscription records are created.

```json
{
  "lexicon": 1,
  "id": "at.glean.subscription",
  "defs": {
    "main": {
      "type": "record",
      "key": "tid",
      "description": "A single RSS feed subscription.",
      "record": {
        "type": "object",
        "required": ["feedUrl"],
        "properties": {
          "createdAt": { "type": "string", "format": "datetime" },
          "feedUrl": { "type": "string" },
          "title": { "type": "string" },
          "category": { "type": "string" }
        }
      }
    }
  }
}
```

#### OPML Import/Export

OPML is **not** part of the lexicon. It is only used as a transport format:

- **Import (onboarding)**: User uploads an OPML file. Glean parses it, validates each feed URL, creates individual `at.glean.subscription` records on the user's PDS, and indexes them locally.
- **Export (offboarding)**: Glean reads the user's `at.glean.subscription` records from their PDS and generates an OPML file for download. The user's repository remains the canonical source.

### 3.2 `at.glean.annotation`

Reading notes on an article: quote a passage, tag it, rate it, or write a note. A user can have many annotations per article. These are public records on the PDS — users can always share an annotation on Bluesky if they want discussion.

```json
{
  "lexicon": 1,
  "id": "at.glean.annotation",
  "defs": {
    "main": {
      "type": "record",
      "key": "tid",
      "description": "Reading note on a specific RSS article.",
      "record": {
        "type": "object",
        "required": ["feedUrl", "articleUrl"],
        "properties": {
          "createdAt": { "type": "string", "format": "datetime" },
          "feedUrl": { "type": "string" },
          "articleUrl": { "type": "string" },
          "quote": { "type": "string", "maxGraphemes": 5000 },
          "note": { "type": "string", "maxGraphemes": 500 },
          "tags": {
            "type": "array",
            "items": { "type": "string", "maxGraphemes": 50 },
            "maxLength": 10
          },
          "rating": { "type": "integer", "minimum": 1, "maximum": 5 }
        }
      }
    }
  }
}
```

### 3.3 `at.glean.like`

A user likes an article. The liked feed surfaces popular articles and feeds into discovery. Likes also feed into the recommendation system.

```json
{
  "lexicon": 1,
  "id": "at.glean.like",
  "defs": {
    "main": {
      "type": "record",
      "key": "tid",
      "description": "Like an RSS article.",
      "record": {
        "type": "object",
        "required": ["feedUrl", "articleUrl"],
        "properties": {
          "createdAt": { "type": "string", "format": "datetime" },
          "feedUrl": { "type": "string" },
          "articleUrl": { "type": "string" }
        }
      }
    }
  }
}
```

### 3.4 `at.margin.note` (External)

Glean also indexes records from the `at.margin.note` lexicon (owned by [margin.at](https://margin.at)). These are displayed in the UI as if they were `at.glean.annotation` records — margin notes appear alongside glean annotations on article detail pages.

The mapping from margin note to glean annotation:

| Margin note field              | Annotation field | Notes                                                           |
| ------------------------------ | ---------------- | --------------------------------------------------------------- |
| `target.source`                | `article_url`    | W3C SpecificResource source URL                                 |
| `body.value`                   | `note`           | Text content of the annotation                                  |
| `target.selector.exact`        | `quote`          | TextQuoteSelector exact text                                    |
| `tags`                         | `tags`           | Direct mapping                                                  |
| `createdAt`                    | `created_at`     | Direct mapping                                                  |
| _(looked up from articles DB)_ | `feed_url`       | Resolved by matching `target.source` against known article URLs |

When no matching article exists in the local DB, the annotation is stored with an empty `feed_url`. Margin notes are indexed from both Jetstream and PDS sync, same as glean records.

### 3.5 `app.skyreader.feed.subscription` (External)

Glean also indexes records from the Skyreader lexicon (`app.skyreader.feed.subscription`). When a user has subscriptions in Skyreader, they are imported as Glean subscriptions during PDS sync and via Jetstream events. This lets users who previously used Skyreader seamlessly transition to Glean without re-subscribing to their feeds.

The mapping from Skyreader subscription to Glean subscription:

| Skyreader field | Glean field   | Notes                               |
| --------------- | ------------- | ------------------------------------ |
| `feedUrl`       | `feed_url`    | Direct mapping                      |
| `title`         | `title`       | Direct mapping                      |
| `siteUrl`       | `site_url`    | Stored on the feed record           |
| `createdAt`     | `added_at`    | Direct mapping                      |
| _(none)_        | `category`    | Empty (Skyreader has no categories) |

If a Glean subscription already exists for the same `feed_url`, the existing one is kept. If the existing subscription has no URI (was created locally without PDS sync), the Skyreader URI/CID is backfilled.

### 3.6 `app.bsky.graph.follow` (External)

Follow relationships are tracked from Bluesky and Tangled follow records. The `FollowRecord` struct is validated against the lexicon at `lexicons/app/bsky/graph/follow.json`. The optional `via` field (a strong ref) is preserved as raw JSON but not used by Glean.

### 3.7 Lexicon Constants

All collection NSIDs are defined as constants in `lexicon.go` and used throughout the codebase:

```go
const (
    CollectionSubscription          = "at.glean.subscription"
    CollectionAnnotation            = "at.glean.annotation"
    CollectionLike                  = "at.glean.like"
    CollectionMarginNote            = "at.margin.note"
    CollectionSkyreaderSubscription = "app.skyreader.feed.subscription"
    CollectionBskyFollow            = "app.bsky.graph.follow"
    CollectionTangledFollow         = "sh.tangled.graph.follow"
)
```

### 3.8 AppView Query Lexicons

As an AppView, Glean serves the following XRPC query endpoints. Other AT Protocol applications can call these to access indexed `at.glean.*` data without implementing their own indexer.

#### `at.glean.listSubscriptions`

List subscriptions from a repo, with optional filtering.

```
Input:
  repo: string (DID of the user)
  category?: string
  limit?: integer (default 50, max 100)
  cursor?: string

Output:
  cursor?: string
  subscriptions: [{ uri, cid, value: at.glean.subscription#main, indexedAt }]
```

#### `at.glean.listFeedLists`

List subscription lists from multiple repos, with optional filtering.

```
Input:
  actors?: string[] (filter by DIDs)
  limit?: integer (default 50, max 100)
  cursor?: string

Output:
  cursor?: string
  feeds: [{ did, subscriptionCount, subscriptions: [{ feedUrl, title, category }] }]
```

#### `at.glean.listAnnotations`

List annotations for an article, a feed, or by a user.

```
Input:
  feedUrl?: string
  articleUrl?: string
  author?: string (DID)
  limit?: integer (default 50, max 100)
  cursor?: string

Output:
  cursor?: string
  annotations: [{ uri, cid, author: { did, handle }, value, indexedAt }]
```

#### `at.glean.listLikes`

List liked articles, optionally filtered by user or feed.

```
Input:
  author?: string (DID)
  feedUrl?: string
  limit?: integer (default 50, max 100)
  cursor?: string

Output:
  cursor?: string
  likes: [{ uri, cid, author: { did, handle }, value: at.glean.like#main, indexedAt }]
```

#### `at.glean.getTrending`

Articles with the most likes, forming the community feed.

```
Input:
  limit?: integer (default 50, max 100)
  cursor?: string
  since?: string (datetime)

Output:
  cursor?: string
  articles: [{ feedUrl, articleUrl, title, likeCount, annotations: [...] }]
```

#### `at.glean.getRecommendations`

Get feed recommendations for a user based on clustering.

```
Input:
  repo: string (DID of the user, query parameter)
  limit?: integer (default 20, max 50)

Output:
  feeds: [{ feedUrl, title, siteUrl, description, subscriberCount, score }]
  people: [{ did, handle, displayName, avatar, jaccard, commonFeeds }]
```

### 3.9 AppView Jetstream Consumption

Glean subscribes to a Jetstream endpoint (`GLEAN_JETSTREAM`, default `wss://jetstream.glean.at`) for all `at.glean.*` records:

```
SUBSCRIBE collections: ["at.glean.subscription", "at.glean.annotation", "at.glean.like", "app.bsky.graph.follow", "sh.tangled.graph.follow", "at.margin.note", "app.skyreader.feed.subscription"]
```

On each event:

- **create**: Insert record into local SQLite, update materialized counts
- **delete**: Tombstone the record (soft delete to preserve foreign key integrity)
- **update**: Replace the record's CID and value

The AppView does not handle writes. Users write records to their own PDS. Glean only reads them from Jetstream.

## 4. RSS Reader

Glean is first and foremost an RSS reader. It fetches, parses, and stores articles from RSS, Atom, and JSON feeds so users can read them in a clean interface.

### 4.1 Feed Fetching

A background scheduler polls subscribed feeds on a fixed 5-minute tick. Feeds are fetched at most once per cycle regardless of how many users share them.

```
                    ┌─────────────────────────┐
                    │     Feed Scheduler      │
                    │  (background goroutine) │
                    └────────┬────────────────┘
                             │ every 5 min
                    ┌────────▼────────────────┐
                    │     Feed Fetcher        │
                    │                         │
                    │  1. SELECT feeds where   │
                    │     subscriber_count > 0 │
                    │     AND not fetched in   │
                    │     last 30 min         │
                    │  2. Dedup in-flight:    │
                    │     skip if already     │
                    │     being fetched       │
                    │  3. Respect ETag/If-None│
                    │     Match / Last-Modified│
                    │  4. GET feed URL        │
                    │  5. Parse XML/JSON      │
                    │  6. Upsert articles     │
                    │  7. Update feed metadata│
                    └────────┬────────────────┘
                             │
              ┌──────────────┼──────────────┐
              │              │              │
     ┌────▼────┐  ┌─────▼─────┐  ┌─────▼─────┐  ┌─────▼─────┐
     │RSS/XML  │  │RSS1/RDF   │  │Atom/XML   │  │JSON Feed  │
     │Parser   │  │Parser     │  │Parser     │  │Parser     │
     └─────────┘  └───────────┘  └───────────┘  └───────────┘
```

### 4.2 Fetch Schedule

The scheduler uses a single fixed interval with in-flight deduplication:

- **Tick interval**: The scheduler checks for stale feeds every 5 minutes
- **Staleness threshold**: Feeds not fetched in the last 30 minutes are eligible
- **Subscriber filter**: Only feeds with `subscriber_count > 0` are fetched
- **In-flight dedup**: If a feed is already being fetched (e.g., manual refresh and background scheduler overlap), the second caller waits for the first to complete rather than fetching again
- **HTTP cache**: Honor `ETag` and `Last-Modified` headers to skip parsing when nothing changed (304 Not Modified)
- **Error tracking**: `error_count` increments on failure, resets to 0 on success. Feeds with high error counts are surfaced as "dead feeds" to the user.

```sql
-- Feeds are fetched once regardless of subscriber count
SELECT ... FROM feeds
WHERE subscriber_count > 0
  AND (last_fetched_at IS NULL OR last_fetched_at <= :cutoff)
ORDER BY last_fetched_at ASC NULLS FIRST
```

### 4.3 Feed Parsing

Go's `encoding/xml` for RSS and Atom. A simple `encoding/json` for JSON Feed.

Each parser returns a normalized `Feed` and a slice of `Article` structs:

```go
type Article struct {
    FeedURL   string
    GUID      string
    Title     string
    URL       string
    Author    string
    Content   string
    Summary   string
    Published time.Time
    Updated   time.Time
}
```

Articles are deduplicated by `(feed_url, guid)`. On upsert, only the article metadata changes — reading state is preserved.

### 4.4 Article Content

Glean stores article content locally so the reading experience is fast and consistent:

```sql
CREATE TABLE articles (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    feed_url    TEXT NOT NULL REFERENCES feeds(feed_url),
    guid        TEXT NOT NULL,
    title       TEXT NOT NULL DEFAULT '',
    url         TEXT,
    author      TEXT,
    summary     TEXT,
    content     TEXT,
    full_content TEXT,
    published   DATETIME,
    updated     DATETIME,
    fetched_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(feed_url, guid)
);

CREATE INDEX idx_articles_feed ON articles(feed_url);
CREATE INDEX idx_articles_published ON articles(published DESC);
```

Content is stored as raw HTML from the feed's `<content:encoded>`, `<summary>`, or JSON Feed `content_html`. `full_content` stores scraped article content fetched from the original URL. The server renders it in a sanitized view (strip `<script>`, `<iframe>`, etc.).

### 4.5 Read State

Read/unread state is tracked per user per article:

```sql
CREATE TABLE read_state (
    user_did    TEXT NOT NULL REFERENCES users(did),
    article_id  INTEGER NOT NULL REFERENCES articles(id),
    is_read     BOOLEAN NOT NULL DEFAULT 0,
    read_at     DATETIME,
    PRIMARY KEY (user_did, article_id)
);

CREATE INDEX idx_read_state_unread ON read_state(user_did, is_read) WHERE is_read = 0;
```

### 4.6 Reading Experience

The `/articles` page is the main reading view:

- **River of news**: Chronological list of all unread articles across all subscriptions
- **Feed filter**: Narrow to a single feed or category
- **Mark as read**: Individual or "mark all above" / "mark all"
- **Like**: Endorse an article (public, synced to Bluesky PDS)
- **Open original**: Title links to the source article
- **Share**: Share to Bluesky
- **Keyboard navigation**: `j`/`k` to navigate, `l` to like, `m` to mark read (progressive enhancement via a small `<script>` block)

### 4.7 Feed Discovery from Content

Beyond the clustering system, Glean also discovers new feeds from article content:

- **Auto-discovery**: When fetching a feed, parse `<link rel="alternate" type="application/rss+xml">` from the feed's site URL to discover related feeds
- **Feedfavicon**: Fetch `favicon.ico` or `/apple-touch-icon.png` from the feed's site URL for display
- **Dead feed detection**: If a feed fails for 7 consecutive fetches (14 days at base interval), mark it as dead. Notify the user and offer to remove it.

## 5. System Architecture

Glean runs as a single Go binary that fills three roles: **AppView** (indexing `at.glean.*` records from Jetstream, serving XRPC queries), **RSS reader** (fetching and storing feed content), and **web UI** (htmx frontend).

```
                          Jetstream (GLEAN_JETSTREAM)
                                │ subscribe
                                ▼
                     ┌─────────────────────┐
                     │   Go Server (glean.at)│
                     │                      │
   Browser ──HTTP──► │  ┌────────────────┐  │ ──XRPC queries──► Other AT apps
   (htmx + TW)      │  │    Router      │  │
                     │  │  ┌───────────┐ │  │
                     │  │  │ Handlers  │ │  │
                     │  │  │ (UI + XRPC)│ │  │
                     │  │  └─────┬─────┘ │  │
                     │  └────────┼────────┘  │
                     │           │           │
                     │  ┌────────▼────────┐  │         ┌──────────────────┐
                     │  │  Service Layer  │  │         │  Feed Scheduler  │
                     │  │                 │──┼──sync──►│  (goroutine)     │
                     │  └────────┬────────┘  │         │  Fetcher + Parser│
                     │           │           │         └────────┬─────────┘
                     │  ┌────────▼────────┐  │                  │
                     │  │     SQLite      │  │           RSS/Atom/JSON feeds
                     │  │  (jetstream idx, │  │
                     │  │   articles,     │  │         ┌──────────────────┐
                     │  │   read state,   │  │         │  Cluster Engine  │
                     │  │   clustering)   │◄─┼────────►│  (periodic cron) │
                     │  └─────────────────┘  │         └──────────────────┘
                     └──────────────────────┘

                       AppView responsibilities:
                       • Subscribe to Jetstream for at.glean.subscription, at.glean.annotation, at.glean.like, at.margin.note, app.skyreader.feed.subscription
                       • Index records into SQLite
                       • Convert at.margin.note records to annotations (displayed alongside glean.at annotations)
                       • Import app.skyreader.feed.subscription records as Glean subscriptions
                     • Serve XRPC query endpoints (at.glean.listSubscriptions, etc.)
                     • Host the web UI at glean.at
                     • Write to user PDS on behalf of user (when user acts through UI)
```

## 6. Database Schema (SQLite)

### 6.1 Users

```sql
CREATE TABLE users (
    did         TEXT PRIMARY KEY,
    handle      TEXT NOT NULL,
    display_name TEXT,
    avatar_url  TEXT,
    indexed_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
```

### 6.2 Feed Subscriptions

Indexed from `at.glean.subscription` records on user PDS.

```sql
CREATE TABLE subscriptions (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    user_did    TEXT NOT NULL REFERENCES users(did),
    feed_url    TEXT NOT NULL,
    title       TEXT,
    category    TEXT,
    added_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    uri         TEXT,
    cid         TEXT,
    UNIQUE(user_did, feed_url)
);

CREATE INDEX idx_subscriptions_feed ON subscriptions(feed_url);
CREATE INDEX idx_subscriptions_user ON subscriptions(user_did);
```

### 6.3 Feeds

Master list of all known RSS feeds.

```sql
CREATE TABLE feeds (
    feed_url        TEXT PRIMARY KEY,
    title           TEXT,
    site_url        TEXT,
    description     TEXT,
    feed_type       TEXT CHECK(feed_type IN ('rss', 'atom', 'json')),
    last_fetched_at DATETIME,
    last_error      TEXT,
    subscriber_count INTEGER NOT NULL DEFAULT 0,
    etag            TEXT,
    last_modified   TEXT,
    fetch_interval_minutes INTEGER NOT NULL DEFAULT 30,
    next_fetch_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    consecutive_empty_fetches INTEGER NOT NULL DEFAULT 0,
    error_count     INTEGER NOT NULL DEFAULT 0,
    favicon_url     TEXT
);
```

### 6.4 Articles

Fetched from RSS feeds. Only fetched for feeds that have local subscribers.

```sql
CREATE TABLE articles (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    feed_url    TEXT NOT NULL REFERENCES feeds(feed_url),
    guid        TEXT NOT NULL,
    title       TEXT NOT NULL DEFAULT '',
    url         TEXT,
    author      TEXT,
    summary     TEXT,
    content     TEXT,
    published   DATETIME,
    updated     DATETIME,
    fetched_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(feed_url, guid)
);

CREATE INDEX idx_articles_feed ON articles(feed_url);
CREATE INDEX idx_articles_published ON articles(published DESC);
```

### 6.5 Read State

```sql
CREATE TABLE read_state (
    user_did    TEXT NOT NULL REFERENCES users(did),
    article_id  INTEGER NOT NULL REFERENCES articles(id),
    is_read     BOOLEAN NOT NULL DEFAULT 0,
    read_at     DATETIME,
    PRIMARY KEY (user_did, article_id)
);

CREATE INDEX idx_read_state_unread ON read_state(user_did, is_read) WHERE is_read = 0;
```

### 6.6 Annotations, Likes

Local mirror of AT Protocol lexicon records for fast querying.

```sql
CREATE TABLE annotations (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    uri         TEXT NOT NULL UNIQUE,
    author_did  TEXT NOT NULL REFERENCES users(did),
    feed_url    TEXT NOT NULL,
    article_url TEXT NOT NULL,
    quote       TEXT,
    note        TEXT,
    tags        TEXT,
    rating      INTEGER,
    created_at  DATETIME NOT NULL,
    cid         TEXT
);

CREATE TABLE likes (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    uri         TEXT NOT NULL UNIQUE,
    author_did  TEXT NOT NULL REFERENCES users(did),
    feed_url    TEXT NOT NULL,
    article_url TEXT NOT NULL,
    created_at  DATETIME NOT NULL,
    cid         TEXT,
    UNIQUE(author_did, feed_url, article_url)
);
```

### 6.7 Cluster Precomputation

Stores precomputed similarity data to avoid recalculating on every request.

```sql
CREATE TABLE feed_similarity (
    feed_a     TEXT NOT NULL REFERENCES feeds(feed_url),
    feed_b     TEXT NOT NULL REFERENCES feeds(feed_url),
    jaccard    REAL NOT NULL,
    computed_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (feed_a, feed_b),
    CHECK(feed_a < feed_b)
);

CREATE TABLE user_similarity (
    user_a     TEXT NOT NULL REFERENCES users(did),
    user_b     TEXT NOT NULL REFERENCES users(did),
    jaccard    REAL NOT NULL,
    common_feeds INTEGER NOT NULL,
    computed_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_a, user_b),
    CHECK(user_a < user_b)
);
```

### 6.8 Follows

Tracks follow relationships between users (from `app.bsky.graph.follow` and `sh.tangled.graph.follow` records).

```sql
CREATE TABLE follows (
    user_did    TEXT NOT NULL REFERENCES users(did),
    target_did  TEXT NOT NULL,
    uri         TEXT,
    cid         TEXT,
    followed_at DATETIME,
    PRIMARY KEY (user_did, target_did)
);

CREATE INDEX idx_follows_user ON follows(user_did);
CREATE INDEX idx_follows_target ON follows(target_did);
```

### 6.9 OAuth Storage

```sql
CREATE TABLE oauth_auth_requests (
    state TEXT PRIMARY KEY,
    data  TEXT NOT NULL
);

CREATE TABLE oauth_sessions (
    account_did TEXT NOT NULL,
    session_id  TEXT NOT NULL,
    data        TEXT NOT NULL,
    PRIMARY KEY (account_did, session_id)
);
```

Glean has two complementary recommendation signals:

- **Subscriptions** (Jaccard similarity): "Who reads the same feeds?" → feed and people discovery
- **Likes** (co-occurrence): "Who likes the same articles?" → article and feed discovery

### 7.1 Feed Co-occurrence (Jaccard Similarity)

For any two feeds, the similarity is the Jaccard index of their subscriber sets:

```
J(A, B) = |subscribers(A) ∩ subscribers(B)| / |subscribers(A) ∪ subscribers(B)|
```

This is recomputed periodically (cron job) or incrementally when subscriptions change.

### 7.2 User Similarity

For any two users, compute Jaccard over their subscription sets:

```
J(U1, U2) = |feeds(U1) ∩ feeds(U2)| / |feeds(U1) ∪ feeds(U2)|
```

### 7.3 Recommendation Algorithms

**Feed recommendations (on glean.at):**

1. Find users with Jaccard > 0.2 (similar readers)
2. Collect feeds those users subscribe to that the target user does not
3. Rank by frequency (how many similar users subscribe) and average similarity
4. Return top N feeds as recommendations

```
score(feed) = Σ J(target, U)  for each user U subscribed to feed
```

**Article recommendations (on glean.at, from likes):**

1. Find users who liked articles that the target user also liked
2. Collect articles those users liked that the target has not
3. Rank by frequency and recency
4. Return top N articles as recommendations

```
score(article) = Σ J(target, U)  for each similar user U who liked the article
```

**People recommendations (to follow on Bluesky):**

1. Compute user similarity for all pairs
2. Return users with highest Jaccard, linking to their Bluesky profile for follow

### 7.4 Implementation

For the initial version, brute-force Jaccard with SQLite is sufficient (scale: ~10k users, ~100k subscriptions). The query is:

```sql
SELECT s2.feed_url, COUNT(*) as overlap_count
FROM subscriptions s1
JOIN subscriptions s2 ON s1.feed_url = s2.feed_url
WHERE s1.user_did = ? AND s2.user_did != ?
AND s2.feed_url NOT IN (SELECT feed_url FROM subscriptions WHERE user_did = ?)
GROUP BY s2.feed_url
ORDER BY overlap_count DESC
LIMIT 20;
```

For larger scale, move to MinHash + LSH (banded hashing) to approximate Jaccard in sub-linear time.

### 7.5 Clustering Engine (Cron)

A background goroutine runs on a configurable schedule (`GLEAN_CLUSTER_INTERVAL`, default 10m):

1. **Compute feed similarity**: Batch-update the `feed_similarity` table (Jaccard over subscriber sets)
2. **Compute user similarity**: Batch-update the `user_similarity` table (Jaccard over subscription sets, boosted by follow relationships)
3. **Generate feed recommendations**: Materialize top feed recommendations per user into `user_feed_recommendations`
4. **Generate article recommendations**: Materialize top article recommendations per user into `user_article_recommendations`

Jetstream ingestion and record indexing happen in a separate persistent goroutine (the Jetstream consumer), not in the cron.

```sql
CREATE TABLE user_feed_recommendations (
    user_did    TEXT NOT NULL REFERENCES users(did),
    feed_url    TEXT NOT NULL REFERENCES feeds(feed_url),
    score       REAL NOT NULL,
    computed_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_did, feed_url)
);

CREATE TABLE user_article_recommendations (
    user_did    TEXT NOT NULL REFERENCES users(did),
    feed_url    TEXT NOT NULL,
    article_url TEXT NOT NULL,
    score       REAL NOT NULL,
    computed_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_did, feed_url, article_url)
);
```

## 8. HTTP API / htmx Endpoints

The server renders HTML fragments that htmx swaps into the page. No JSON API needed for the frontend.

### 8.1 Pages

| Route                          | Method | Description                                              |
| ------------------------------ | ------ | -------------------------------------------------------- |
| `/`                            | GET    | Landing page / auth redirect                             |
| `/dashboard`                   | GET    | Main dashboard: unread articles, recommendations sidebar |
| `/feeds`                       | GET    | Manage RSS subscriptions (OPML import for onboarding)    |
| `/feeds/list`                  | GET    | Feed list fragment (htmx partial)                        |
| `/feeds/discover-url`          | GET    | Discover feed URL from a website                         |
| `/feeds/opml/upload`           | POST   | Upload OPML file to bulk-import subscriptions            |
| `/feeds/opml/download`         | GET    | Export subscriptions as OPML (offboarding)               |
| `/feeds/add`                   | POST   | Add a single feed URL                                    |
| `/feeds/remove`                | DELETE | Remove a feed                                            |
| `/feeds/refresh`               | POST   | Refresh all subscribed feeds                             |
| `/feeds/clear`                 | POST   | Clear all subscriptions                                  |
| `/articles`                    | GET    | Read articles (paginated, filterable by feed)            |
| `/articles/{id}`               | GET    | Article detail view                                      |
| `/articles/{id}/read`          | POST   | Mark article as read                                     |
| `/articles/{id}/unread`        | POST   | Mark article as unread                                   |
| `/articles/{id}/like`          | POST   | Like an article                                          |
| `/articles/{id}/fetch-content` | POST   | Fetch full article content from original URL             |
| `/articles/mark-all-read`      | POST   | Mark all articles as read                                |
| `/trending`                    | GET    | Community feed: articles ranked by likes                 |
| `/library`                     | GET    | Liked articles and annotations                           |
| `/library/create`              | POST   | Create annotation on an article                          |
| `/library/{id}/delete`         | POST   | Delete an annotation                                     |
| `/profile/{did}`               | GET    | Public profile: their feeds, likes, annotations          |

### 8.2 htmx Patterns

- **Feed list**: `<div hx-get="/feeds/list" hx-trigger="load">` renders the subscription list as a fragment
- **Infinite scroll articles**: `<div hx-get="/articles?page=2" hx-trigger="intersect">` for pagination
- **Like button**: `<button hx-post="/articles/{id}/like" hx-swap="outerHTML">` self-updates the button state
- **OPML upload**: `<form hx-post="/feeds/opml/upload" hx-encoding="multipart/form-data" hx-target="#feed-list">`

## 9. Project Structure

```
glean/
├── main.go                        # Entry point, wire everything
├── go.mod
├── go.sum
├── Dockerfile
├── Makefile
├── lexicons/
│   └── at/
│       ├── glean/                  # Glean lexicon JSON schemas (subscription, annotation, like)
│       └── margin/                 # External: at.margin.note W3C Web Annotation schema
│   └── app/bsky/graph/             # External: app.bsky.graph.follow schema
├── internal/
│   ├── atproto/
│   │   ├── auth.go                # DID resolution, OAuth flow
│   │   ├── client.go              # XRPC client (write to user PDS)
│   │   ├── collectiondir.go       # Collection directory backfill (startup)
│   │   ├── jetstream.go           # Subscribe to Jetstream via official client
│   │   ├── stream_handler.go      # Stream event → DB handler
│   │   ├── lexicon.go             # Lexicon record types (at.glean.*, maintained by hand)
│   │   ├── lexicon_external.go    # External lexicon record types (FollowRecord, MarginNoteRecord, SkyreaderSubscriptionRecord)
│   │   ├── lexicon_test.go        # Test: Go structs match lexicon JSON schemas
│   │   ├── sync.go                # PDS record reconciliation
│   │   └── xrpc.go                # XRPC query handlers (AppView endpoints)
│   ├── db/
│   │   ├── db.go                  # SQLite connection, migrations
│   │   ├── user.go                # User queries
│   │   ├── feed.go                # Feed + subscription queries
│   │   ├── article.go             # Article queries
│   │   ├── social.go              # Like, annotation queries
│   │   ├── follow.go              # Follow queries
│   │   ├── cluster.go             # Similarity + recommendation queries
│   │   ├── oauth_store.go         # OAuth session storage
│   │   └── store.go               # FeedStore adapter for scheduler
│   ├── feed/
│   │   ├── parser.go              # RSS/Atom/RDF/JSON feed parser
│   │   ├── fetcher.go             # Scheduler with dedup + Fetcher
│   │   ├── discover.go            # Feed auto-discovery from URLs
│   │   └── opml.go                # OPML import/export
│   ├── scraper/
│   │   └── scraper.go             # Full article content scraper
│   ├── metrics/
│   │   └── metrics.go             # Prometheus metrics definitions
│   ├── cluster/
│   │   ├── jaccard.go             # Jaccard similarity computation
│   │   ├── recommender.go         # Feed + people recommendation queries
│   │   └── cron.go                # Background recomputation scheduler
│   ├── server/
│   │   ├── server.go              # HTTP server, router setup
│   │   ├── auth_handler.go        # OAuth login/callback
│   │   ├── feeds_handler.go       # Feed management handlers
│   │   ├── articles_handler.go    # Article reading handlers
│   │   ├── annotations_handler.go # Annotation handlers
│   │   ├── dashboard_handler.go   # Dashboard handler
│   │   ├── trending_handler.go    # Trending handler
│   │   ├── index_handler.go       # Landing page handler
│   │   ├── profile_handler.go     # Public profile handler
│   │   ├── pagination.go          # Pagination helpers
│   │   ├── middleware.go          # Auth, logging, CSRF middleware
│   │   └── session.go             # Session management
│   ├── sanitize/
│   │   └── sanitize.go            # HTML sanitization for article content
│   └── tmpl/
│       ├── base.html              # Base template with htmx + Tailwind
│       ├── index.html             # Landing page
│       ├── login.html             # Login page
│       ├── dashboard.html         # Dashboard
│       ├── feeds.html             # Feed management
│       ├── articles.html          # Article listing
│       ├── article_detail.html    # Article detail
│       ├── trending.html          # Trending articles
│       ├── library.html           # Liked articles + annotations
│       ├── profile.html           # User profile
│       ├── 404.html               # Not found page
│       └── partials/              # Reusable template fragments
├── static/
│   ├── input.css                  # Tailwind input
│   └── output.css                 # Tailwind compiled output
├── docs/
│   ├── specs.md                   # Technical specification (this document)
│   └── design.md                  # Design system
└── tailwind.config.js
```

## 10. Auth Flow

DID resolution uses a configurable PLC directory (`GLEAN_PLC_URL`, defaults to `https://didplc.glean.at`). The identity directory is initialized once at startup via `InitIdentity()` with a caching layer (250k entries, 24h TTL).

1. User visits `/`, clicks "Sign in with Bluesky" (or any AT Proto PDS)
2. Server redirects to AT Protocol OAuth authorization endpoint
3. User authorizes on their PDS
4. PDS redirects back with authorization code
5. Server exchanges code for access token + refresh token
6. Server resolves DID and creates a session (cookie with encrypted DID)
7. On each request, middleware decrypts session, loads user from DB (or creates if new)

The server never stores the user's AT Protocol password. It stores the session tokens for XRPC calls to the user's PDS (to read/write their lexicon records).

## 11. Data Flow

### 11.1 User Imports OPML

```
Browser ──POST /feeds/opml/upload──► Server
                                      │
                                      ├─► Parse OPML, extract feed URLs
                                      ├─► Fetch each feed, validate + store in `feeds` table
                                      ├─► For each feed, create an `at.glean.subscription` record
                                      │   via XRPC write to user's PDS
                                      ├─► Insert subscriptions in local `subscriptions` table
                                      └─◄ Return updated feed list fragment (htmx)
```

### 11.2 Reading the Feed

```
Browser ──GET /articles──► Server
                              │
                              ├─► Query articles for user's subscriptions
                              ├─► Render article-list.html partial
                              └─◄ Return HTML fragment (htmx)
```

### 11.3 Recommendations

```
Cron (every 6h) ──► Cluster Engine
                        │
                        ├─► SELECT user similarity pairs
                        ├─► Compute recommendation scores
                        └─► INSERT into user_feed_recommendations

Browser ──GET /discover/feeds──► Server
                                  │
                                  ├─► SELECT from user_feed_recommendations
                                  ├─► Fetch feed metadata
                                  └─◄ Render recommendation cards (htmx)
```

## 12. Key Design Decisions

### 12.1 Why use lexicon records for feed subscriptions?

- **User sovereignty**: Each feed subscription lives as a record on the user's PDS. They can export them, move PDS, or revoke access at any time.
- **Interoperability**: Any AT Protocol app can read the lexicon and integrate with Glean data.
- **No lock-in**: If Glean shuts down, the user's data is intact on their PDS.
- **OPML as transport only**: OPML is a common interchange format used solely for import (onboarding from existing readers) and export (offboarding). The canonical representation is always the lexicon — individual `at.glean.subscription` records, one per feed.

### 12.2 Why SQLite?

- Single-binary deployment, no external database dependency
- More than sufficient for the expected scale (tens of thousands of users)
- Go's `database/sql` interface makes it easy to swap later if needed
- Matches the project's philosophy of simplicity

### 12.3 Prometheus Metrics

Glean exposes a `/metrics` endpoint for monitoring. Key metrics:

- **`glean_feeds_fetched_total`** — Feed fetch attempts labeled by status (`success`, `error`, `not_modified`)
- **`glean_feed_fetch_duration_seconds`** — Histogram of feed fetch latency
- **`glean_articles_upserted_total`** — Counter of articles stored from feeds
- **`glean_jetstream_events_total`** — Jetstream events labeled by collection and action
- **`glean_jetstream_errors_total`** — Jetstream handler errors
- **`glean_jetstream_reconnects_total`** — Jetstream reconnection count
- **`glean_http_requests_total`** — HTTP request counts labeled by method, path, and status
- **`glean_pds_sync_runs_total`** / **`glean_pds_sync_errors_total`** — PDS sync runs and errors
- **`glean_cluster_runs_total`** / **`glean_cluster_duration_seconds`** — Recommendation engine runs and timing

### 12.4 Why htmx?

- No JavaScript build pipeline
- Server renders everything — simpler mental model
- Progressive enhancement works naturally
- Perfect fit for a read-centric application
- TailwindCSS handles styling without writing custom CSS

### 12.5 AppView Architecture

Glean operates as an AT Protocol AppView. This means:

- **Read path**: All `at.glean.*` data is consumed from Jetstream, not by polling individual PDS instances. The Jetstream consumer runs as a persistent goroutine, upserting records into SQLite as they arrive.
- **Write path**: Users write records to their own PDS (via standard AT Protocol `com.atproto.repo.createRecord` / `deleteRecord`). Glean never stores user data directly — it only indexes what Jetstream delivers.
- **Query path**: Other AT Protocol apps can query Glean's XRPC endpoints to access indexed data (subscriptions, annotations, likes, recommendations) without building their own indexer.
- **Trade-off**: Article content (fetched from RSS feeds) is local-only and not part of the AT Protocol layer. Only individual feed subscription records (`at.glean.subscription`) live on the PDS.

### 12.6 Privacy Model

All PDS records are public. There is no notion of private data on the AT Protocol — everything stored on the repo is visible. Users should be aware that annotations, subscriptions, and likes are all public records.

## 13. Future Considerations

- **MinHash/LSH**: Replace brute-force Jaccard when user count exceeds ~50k
- **Full-text search**: Add FTS5 virtual table on articles for search
- **Email digest**: Periodic email with top articles from subscribed feeds
