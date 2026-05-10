# Glean - Design Document

## 1. Overview

Glean is a social RSS reader built on the AT Protocol. It operates as an **AppView** for the `at.glean.*` lexicon namespace: it indexes records from Jetstream, serves XRPC query endpoints, and provides the web UI at [glean.at](https://glean.at).

Users store their RSS feed subscriptions as individual lexicon records on their PDS (one record per feed). Glean's AppView consumes Jetstream, indexes those records, fetches the referenced RSS feeds, and serves both the reader UI and public XRPC APIs for the `at.glean.*` namespace.

The core idea: your RSS subscriptions are a strong signal about your interests. When enough people expose theirs, you can discover both **people** (who reads the same things) and **content** (what similar readers follow that you don't).

## 2. Stack

| Layer            | Technology                                                                                      |
| ---------------- | ----------------------------------------------------------------------------------------------- |
| Backend          | Go                                                                                              |
| Database         | SQLite (3 files: users, articles, recs via `mattn/go-sqlite3` + `sqlite-vec` for vector search) |
| Frontend         | htmx + TailwindCSS                                                                              |
| Auth             | AT Protocol OAuth / DID resolution (configurable PLC directory)                                 |
| AT Protocol role | AppView for `at.glean.*` lexicons                                                               |
| Data source      | AT Protocol Jetstream → SQLite index                                                            |

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

| Skyreader field | Glean field | Notes                               |
| --------------- | ----------- | ----------------------------------- |
| `feedUrl`       | `feed_url`  | Direct mapping                      |
| `title`         | `title`     | Direct mapping                      |
| `siteUrl`       | `site_url`  | Stored on the feed record           |
| `createdAt`     | `added_at`  | Direct mapping                      |
| _(none)_        | `category`  | Empty (Skyreader has no categories) |

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
  people: [{ did, handle, displayName, avatar, jaccard, commonFeeds, isFollowed }]
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

A background scheduler polls subscribed feeds on a configurable tick. Feeds are fetched at most once per cycle regardless of how many users share them.

### 4.2 Fetch Schedule

The scheduler uses a configurable tick interval with in-flight deduplication:

- **Tick interval**: The scheduler checks for stale feeds every `GLEAN_FETCH_INTERVAL` (default 15 minutes)
- **Staleness threshold**: Feeds not fetched in the last 30 minutes are eligible
- **Subscriber filter**: Only feeds with `subscriber_count > 0` are fetched
- **In-flight dedup**: If a feed is already being fetched (e.g., manual refresh and background scheduler overlap), the second caller waits for the first to complete rather than fetching again
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
    feed_url    TEXT NOT NULL,
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
    language    TEXT NOT NULL DEFAULT '',
    UNIQUE(feed_url, guid)
);

CREATE INDEX idx_articles_feed ON articles(feed_url);
CREATE INDEX idx_articles_published ON articles(published DESC);
```

Content is stored as raw HTML from the feed's `<content:encoded>`, `<summary>`, or JSON Feed `content_html`. `full_content` stores scraped article content fetched from the original URL. The server renders it in a sanitized view (strip `<script>`, `<iframe>`, etc.).

The `language` column stores the ISO 639-1 code detected by the LLM (e.g. `en`, `fr`, `ja`). It defaults to empty (`''`) and is populated by the cron job when `GLEAN_LLM_BASE_URL` is configured.

### 4.5 Read State

Read/unread state is tracked per user per article:

```sql
CREATE TABLE read_state (
    user_did    TEXT NOT NULL,
    article_id  INTEGER NOT NULL,
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
- **Dead feed detection**: If a feed fails for 7 consecutive fetches, mark it as dead. Notify the user and offer to remove it.

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

Glean uses three separate SQLite database files to reduce write-lock contention. Each is opened with its own connection pool:

| File              | Contents                                                       | ATTACH alias     |
| ----------------- | -------------------------------------------------------------- | ---------------- |
| `<base>_users`    | Users, follows, OAuth                                          | `main` (primary) |
| `<base>_articles` | Feeds, subscriptions, articles, read state, likes, annotations | `articles`       |
| `<base>_recs`     | Similarity scores, impressions, dismissals, signal weights     | `recs`           |

The users connection pool uses a custom SQLite driver with a `ConnectHook` that ATTACHes the articles and recs databases on every new connection. This allows the cluster engine to run cross-database queries using schema prefixes (`articles.subscriptions`, `recs.user_similarity`, `main.follows`).

Foreign key constraints are not used because SQLite does not support foreign keys across ATTACHed databases. Referential integrity is enforced by the application layer.

### 6.1 Users (`<base>_users`)

Profile data (handle, display name, avatar) is resolved on-the-fly via AT Protocol identity resolution rather than stored locally.

```sql
CREATE TABLE users (
    did            TEXT PRIMARY KEY,
    indexed_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    follows_dirty  BOOLEAN NOT NULL DEFAULT 1,
    languages      TEXT
);
```

### 6.2 Feed Subscriptions (`<base>_articles`)

Indexed from `at.glean.subscription` records on user PDS.

```sql
CREATE TABLE subscriptions (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    user_did    TEXT NOT NULL,
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

### 6.3 Feeds (`<base>_articles`)

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
    consecutive_empty_fetches INTEGER NOT NULL DEFAULT 0,
    error_count     INTEGER NOT NULL DEFAULT 0,
    favicon_url     TEXT
);
```

### 6.4 Articles (`<base>_articles`)

Fetched from RSS feeds. Only fetched for feeds that have local subscribers.

```sql
CREATE TABLE articles (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    feed_url    TEXT NOT NULL,
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
    language    TEXT NOT NULL DEFAULT '',
    UNIQUE(feed_url, guid)
);

CREATE INDEX idx_articles_feed ON articles(feed_url);
CREATE INDEX idx_articles_published ON articles(published DESC);
CREATE INDEX idx_articles_language ON articles(language);
```

### 6.5 Read State (`<base>_articles`)

```sql
CREATE TABLE read_state (
    user_did    TEXT NOT NULL,
    article_id  INTEGER NOT NULL,
    is_read     BOOLEAN NOT NULL DEFAULT 0,
    read_at     DATETIME,
    PRIMARY KEY (user_did, article_id)
);

CREATE INDEX idx_read_state_unread ON read_state(user_did, is_read) WHERE is_read = 0;
```

### 6.6 Annotations, Likes (`<base>_articles`)

Local mirror of AT Protocol lexicon records for fast querying.

```sql
CREATE TABLE annotations (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    uri         TEXT NOT NULL UNIQUE,
    author_did  TEXT NOT NULL,
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
    author_did  TEXT NOT NULL,
    feed_url    TEXT NOT NULL,
    article_url TEXT NOT NULL,
    created_at  DATETIME NOT NULL,
    cid         TEXT,
    UNIQUE(author_did, feed_url, article_url)
);
```

### 6.7 Cluster Precomputation (`<base>_recs`)

Stores precomputed similarity data to avoid recalculating on every request.

```sql
CREATE TABLE feed_similarity (
    feed_a     TEXT NOT NULL,
    feed_b     TEXT NOT NULL,
    jaccard    REAL NOT NULL,
    computed_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (feed_a, feed_b),
    CHECK(feed_a < feed_b)
);

CREATE TABLE user_similarity (
    user_a     TEXT NOT NULL,
    user_b     TEXT NOT NULL,
    jaccard    REAL NOT NULL,
    common_feeds INTEGER NOT NULL,
    common_likes INTEGER NOT NULL DEFAULT 0,
    common_tags INTEGER NOT NULL DEFAULT 0,
    computed_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_a, user_b),
    CHECK(user_a < user_b)
);
```

### 6.8 Follows (`<base>_users`)

Tracks follow relationships between users (from `app.bsky.graph.follow` and `sh.tangled.graph.follow` records).

```sql
CREATE TABLE follows (
    user_did    TEXT NOT NULL,
    target_did  TEXT NOT NULL,
    uri         TEXT,
    cid         TEXT,
    followed_at DATETIME,
    PRIMARY KEY (user_did, target_did)
);

CREATE INDEX idx_follows_user ON follows(user_did);
CREATE INDEX idx_follows_target ON follows(target_did);
```

### 6.9 OAuth Storage (`<base>_users`)

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

## 7. Recommendations

Glean uses a multi-signal recommendation system that combines subscription overlap, like patterns, social graph distance, and user behavior feedback.

### 7.1 Signals

| Signal       | Source                   | Weight (default) | Description                                             |
| ------------ | ------------------------ | ---------------- | ------------------------------------------------------- |
| Subscription | `subscriptions`          | 1.0              | Jaccard over subscriber sets between similar users      |
| Like         | `likes`                  | 0.5              | Time-decayed like co-occurrence (30-day half-life)      |
| Tag          | `annotations.tags`       | 0.3              | Jaccard over annotation tag sets                        |
| Social       | `follow_distances`       | 0.7              | Follow distance: 1-hop=1.0, 2-hop=0.3, 3-hop=0.1        |
| Popularity   | `feeds.subscriber_count` | 0.2              | `log(1 + subscribers) / log(1 + max)`                   |
| Category     | `subscriptions.category` | 0.4              | Boost feeds matching user's existing categories         |
| Content      | `article_embeddings`     | 0.4              | Cosine similarity via embedding KNN (requires embedder) |

### 7.2 Feed Co-occurrence (Jaccard Similarity)

For any two feeds, the similarity is the Jaccard index of their subscriber sets:

```
J(A, B) = |subscribers(A) ∩ subscribers(B)| / |subscribers(A) ∪ subscribers(B)|
```

Feed description similarity is also computed via embedding cosine similarity (requires embedder) and added as a boost.

### 7.3 User Similarity

For any two users, compute Jaccard over their subscription sets, plus like co-occurrence (time-decayed) and tag overlap:

```
J(U1, U2) = jaccard_subscriptions + 0.3 * jaccard_likes + 0.2 * jaccard_tags + 0.5 * follow_boost
```

Like overlap uses exponential time decay: `EXP(-0.023 * age_days)` (30-day half-life).

### 7.4 On-Demand Scoring

Recommendations are computed **on-demand** at query time, not pre-materialized. This avoids write amplification on every cron run.

**Feed recommendation score** (computed in SQL):

```
score = sub_signal * w_sub
      + like_signal * w_like
      + social_signal * w_social
      + pop_signal * w_pop
      + category_signal * w_category
```

Where:

- `sub_signal = SUM(jaccard(target, U))` for similar users U subscribed to feed
- `like_signal = SUM(jaccard(target, U) * time_decay)` for likes in that feed by similar users
- `social_signal = SUM(distance_weight)` from follow_distances
- `pop_signal = log(1 + subscriber_count) / log(1 + max_subscribers)`
- `category_signal = 1` if feed description matches user's top categories

**Article recommendation score**:

```
score = like_signal * w_like
      + social_signal * w_social
      + content_signal * w_content
      + recency_signal * 0.2
```

Content signal uses embedding vectors: the user's liked article embeddings are averaged into a single interest vector, then a KNN query against the `article_embeddings` vec0 table finds semantically similar articles. This requires an embedder to be configured; without it, the content signal is 0.

**Language filtering**: Users can set preferred languages on their profile (stored in the `user_settings` table as a JSON array of ISO 639-1 codes). When set, article recommendations are filtered to only include articles whose `language` column matches one of the selected codes **or** whose `language` is empty (not yet classified). This ensures users with language preferences still see all recommendations when the LLM hasn't run or hasn't classified certain articles yet. If no languages are set (empty/nil), all articles are shown regardless of language.

### 7.5 User Feedback (Dismiss)

Users can dismiss recommendations they don't want to see again:

- `POST /feeds/dismiss` — dismiss a feed recommendation
- `POST /articles/dismiss` — dismiss an article recommendation
- Dismissals are stored locally in `dismissed_recommendations` (not on PDS)
- Dismissed items are excluded from all future recommendation queries
- Auto-dismiss: items shown ≥5 times over >5 days without action are auto-dismissed

Impression tracking (`recommendation_impressions`) records how many times each recommendation was shown and whether the user acted on it.

### 7.6 Auto-Tuned Signal Weights

Each user has a row in `user_signal_weights` with per-signal weights. When a user acts on a recommendation (subscribes, likes), the dominant signal that produced that recommendation is rewarded:

```
new_weight = MAX(0.1, MIN(3.0, old_weight * (1 + learning_rate * delta)))
```

- `learning_rate = 0.1`, `delta = +1` for reward, `-1` for penalty
- Only activates after `minActionsTune = 5` positive actions
- Defaults are used when no row exists for a user

### 7.7 Social Graph

Follow distances (1-hop through 3-hop) are computed incrementally. A `follows_dirty` column on `users` tracks whose follow graph changed since the last cron run. Only dirty users are reprocessed — their existing rows in `follow_distances` are deleted and recomputed via BFS, then the dirty flag is cleared.

- 1-hop: direct follows (weight 1.0)
- 2-hop: friends-of-friends (weight 0.3)
- 3-hop: third-degree connections (weight 0.1)

### 7.8 Diversity & Freshness

After scoring, diversity filtering is applied in Go (not SQL):

- **Domain diversity**: max 2 feeds from the same domain in results
- **Category diversity**: max 3 feeds from the same category in results
- This prevents recommendation clustering on a single source

### 7.9 Cold Start

New users with <5 subscriptions get a fallback strategy:

1. Feeds from 1-hop followed users (70% weight)
2. Globally popular feeds by subscriber count (30% weight)

### 7.10 Clustering Engine (Cron)

A background goroutine runs on a configurable schedule (`GLEAN_CLUSTER_INTERVAL`, default 1h):

1. **Compute feed embeddings**: Embed new feed descriptions via embedding API into `feed_embeddings` table (skipped if no embedder configured)
2. **Compute feed similarity**: Batch-update `feed_similarity` table (Jaccard over subscriber sets + embedding cosine similarity)
3. **Compute user similarity**: Batch-update `user_similarity` table (subscription Jaccard + time-decayed likes + tags + follow boost)
4. **Compute article embeddings**: Embed new articles (`title + summary + content`, excluding `full_content` to stay within embedding model token limits) via embedding API into `article_embeddings` vec0 table (skipped if no embedder configured)
5. **Detect article languages**: Batch-classify article languages via LLM, updating the `language` column (skipped if no LLM configured)
6. **Compute follow distances**: Incremental BFS for dirty users (1-hop through 3-hop from `follows` table)
7. **Compute signal profiles**: Per-user category/tag/like summaries
8. **Auto-dismiss stale**: Dismiss items shown >=5 times over >5 days without action

Jetstream ingestion and record indexing happen in a separate persistent goroutine (the Jetstream consumer), not in the cron.

### 7.11 User Interaction Tables (`<base>_users`)

Per-user interaction state lives in the users database so that real-time writes (impressions, dismissals) never contend with cron batch writes to the recs database.

```sql
CREATE TABLE dismissed_recommendations (
    user_did     TEXT NOT NULL,
    target_type  TEXT NOT NULL CHECK(target_type IN ('feed', 'article')),
    target_id    TEXT NOT NULL,
    reason       TEXT,
    dismissed_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_did, target_type, target_id)
);

CREATE TABLE recommendation_impressions (
    user_did       TEXT NOT NULL,
    target_type    TEXT NOT NULL CHECK(target_type IN ('feed', 'article')),
    target_id      TEXT NOT NULL,
    first_shown_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_shown_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    shown_count    INTEGER NOT NULL DEFAULT 1,
    acted          BOOLEAN NOT NULL DEFAULT 0,
    PRIMARY KEY (user_did, target_type, target_id)
);
```

### 7.12 Computed Recommendation Tables (`<base>_recs`)

Written exclusively by the cron. No user-facing writes — only reads during on-demand scoring.

```sql
CREATE TABLE feed_similarity (
    feed_a     TEXT NOT NULL,
    feed_b     TEXT NOT NULL,
    jaccard    REAL NOT NULL,
    computed_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (feed_a, feed_b),
    CHECK(feed_a < feed_b)
);

CREATE TABLE user_similarity (
    user_a     TEXT NOT NULL,
    user_b     TEXT NOT NULL,
    jaccard    REAL NOT NULL,
    common_feeds INTEGER NOT NULL,
    common_likes INTEGER NOT NULL DEFAULT 0,
    common_tags INTEGER NOT NULL DEFAULT 0,
    computed_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_a, user_b),
    CHECK(user_a < user_b)
);

CREATE TABLE follow_distances (
    user_a   TEXT NOT NULL,
    user_b   TEXT NOT NULL,
    distance INTEGER NOT NULL CHECK(distance IN (1, 2, 3)),
    PRIMARY KEY (user_a, user_b)
);

CREATE TABLE user_signal_weights (
    user_did   TEXT PRIMARY KEY,
    w_sub      REAL NOT NULL DEFAULT 1.0,
    w_like     REAL NOT NULL DEFAULT 0.5,
    w_tag      REAL NOT NULL DEFAULT 0.3,
    w_social   REAL NOT NULL DEFAULT 0.7,
    w_pop      REAL NOT NULL DEFAULT 0.2,
    w_category REAL NOT NULL DEFAULT 0.4,
    w_content  REAL NOT NULL DEFAULT 0.4,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE user_signal_profiles (
    user_did       TEXT PRIMARY KEY,
    total_likes     INTEGER NOT NULL DEFAULT 0,
    total_tags      INTEGER NOT NULL DEFAULT 0,
    top_categories  TEXT,
    top_tags        TEXT,
    updated_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
```

### 7.13 Embeddings (recommended)

When `GLEAN_EMBED_BASE_URL` is configured, article text and feed descriptions are embedded into vectors stored in `sqlite-vec` virtual tables (`recs.feed_embeddings`, `recs.article_embeddings`). The vec0 extension provides native KNN vector search via `WHERE embedding MATCH ? AND k = ?`, replacing Go-side cosine similarity for large-scale lookups. Without embeddings, recommendations rely only on subscription overlap, like patterns, and social graph — no content-based signals.

The embedder uses the official `github.com/openai/openai-go` SDK with `option.WithBaseURL()`, so any OpenAI-compatible `/v1/embeddings` endpoint works (OpenAI, Gemini, Ollama, local inference servers).

### 7.14 LLM Client (optional)

When `GLEAN_LLM_BASE_URL` is configured, an LLM client is available for text classification tasks. It uses the same `github.com/openai/openai-go` SDK pointed at any OpenAI-compatible `/v1/chat/completions` endpoint.

**Language detection**: The cron job calls `DetectLanguages` in batches of up to 100 articles per request. For each article, the title and summary (truncated to 500 characters) are sent with a prompt asking for ISO 639-1 codes. Results are written to `articles.language`. Articles that are already classified (non-empty `language`) are skipped. An empty response from the LLM defaults to English (`en`).

Without an LLM, all articles remain at the default empty language value and language-based filtering is unavailable.

vec0 tables are created dynamically at startup with the configured dimension (`GLEAN_EMBED_DIMENSION`, default 1536):

```sql
CREATE VIRTUAL TABLE recs.feed_embeddings USING vec0(
    feed_url TEXT PRIMARY KEY,
    embedding float[1536]
);

CREATE VIRTUAL TABLE recs.article_embeddings USING vec0(
    article_id INTEGER PRIMARY KEY,
    embedding float[1536]
);
```

Since vec0 virtual tables cannot hold metadata columns, a side table tracks the source text for re-embedding on description changes:

```sql
CREATE TABLE recs.feed_embedding_meta (
    feed_url TEXT PRIMARY KEY,
    source_text TEXT NOT NULL DEFAULT ''
);
```

During cron, `ComputeArticleEmbeddings` embeds new articles in batches using `title + summary + content` (the scraped `full_content` is excluded to stay within model token limits — most embedding models cap at ~8k tokens). Text is truncated to 8000 characters as a safety net. Batches are capped at 100 inputs per API call. `ComputeFeedEmbeddings` embeds feed descriptions (`title || description`) and re-embeds when the source text changes (detected via `feed_embedding_meta`). During on-demand article recommendations, the user's liked article embeddings are averaged into an interest vector, then a vec0 KNN query finds the top-200 most semantically similar articles. For cold-start users (<5 subscriptions), their subscribed feed embeddings are averaged and a KNN query finds similar feeds.

## 8. HTTP API / htmx Endpoints

The server renders HTML fragments that htmx swaps into the page. No JSON API needed for the frontend.

### 8.1 Pages

| Route                          | Method | Description                                                         |
| ------------------------------ | ------ | ------------------------------------------------------------------- |
| `/`                            | GET    | Landing page / auth redirect                                        |
| `/dashboard`                   | GET    | Main dashboard: article recs, unread articles, trending, people, feeds |
| `/feeds`                       | GET    | Manage RSS subscriptions (OPML import for onboarding)               |
| `/feeds/list`                  | GET    | Feed list fragment (htmx partial)                                   |
| `/feeds/opml/upload`           | POST   | Upload OPML file to bulk-import subscriptions (redirects to /feeds) |
| `/feeds/opml/download`         | GET    | Export subscriptions as OPML (offboarding)                          |
| `/feeds/add`                   | POST   | Add a single feed URL                                               |
| `/feeds/remove`                | DELETE | Remove a feed                                                       |
| `/feeds/refresh`               | POST   | Refresh all subscribed feeds                                        |
| `/feeds/retry`                 | POST   | Retry a failed feed                                                 |
| `/feeds/clear`                 | POST   | Clear all subscriptions                                             |
| `/feeds/dismiss`               | POST   | Dismiss a feed recommendation                                       |
| `/articles`                    | GET    | Read articles (paginated, filterable by feed)                       |
| `/articles/new-count`          | GET    | Get count of new articles (for badge updates)                       |
| `/articles/{id}`               | GET    | Article detail view                                                 |
| `/articles/{id}/read`          | POST   | Mark article as read                                                |
| `/articles/{id}/unread`        | POST   | Mark article as unread                                              |
| `/articles/{id}/like`          | POST   | Like an article                                                     |
| `/articles/{id}/fetch-content` | POST   | Fetch full article content from original URL                        |
| `/articles/mark-all-read`      | POST   | Mark all articles as read                                           |
| `/articles/dismiss`            | POST   | Dismiss an article recommendation                                   |
| `/trending`                    | GET    | Community feed: articles ranked by likes (public)                   |
| `/library`                     | GET    | Liked articles and annotations                                      |
| `/library/create`              | POST   | Create annotation on an article                                     |
| `/library/{id}/delete`         | POST   | Delete an annotation                                                |
| `/stats`                       | GET    | Application metrics and performance data (Prometheus, public)       |
| `/profile/{did}`               | GET    | Public profile: their feeds, likes, annotations                     |
| `/settings/languages`          | POST   | Save preferred recommendation languages (htmx, requires auth)       |
| `/auth/login`                  | GET    | Login page                                                          |
| `/auth/register`               | GET    | Register with Eurosky (OAuth flow with hardcoded PDS)               |
| `/auth/resolve`                | GET    | Resolve handle to DID                                               |
| `/auth/start`                  | POST   | Start OAuth authorization flow                                      |
| `/auth/callback`               | GET    | OAuth callback                                                      |
| `/terms`                       | GET    | Terms of service                                                    |

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
│   │   ├── db.go                  # SQLite connection with ATTACH for cross-database queries
│   │   ├── user.go                # User queries
│   │   ├── feed.go                # Feed + subscription queries
│   │   ├── article.go             # Article queries
│   │   ├── social.go              # Like, annotation queries
│   │   ├── follow.go              # Follow queries
│   │   ├── oauth_store.go         # OAuth session storage
│   │   └── store.go               # FeedStore adapter for scheduler
│   ├── feed/
│   │   ├── parser.go              # RSS/Atom/RDF/JSON feed parser
│   │   ├── fetcher.go             # Scheduler with dedup + Fetcher
│   │   ├── discover.go            # Feed auto-discovery from URLs
│   │   └── opml.go                # OPML import/export
│   ├── httpclient/
│   │   └── httpclient.go         # Shared HTTP transport, retry logic, User-Agent
│   ├── scraper/
│   │   └── scraper.go             # Full article content scraper
│   ├── metrics/
│   │   └── metrics.go             # Prometheus metrics definitions
│   ├── ai/
│   │   ├── embed.go               # Embedder interface + OpenAI-compatible implementation + vector helpers
│   │   └── llm.go                 # TextModel interface + OpenAI-compatible LLM implementation
│   ├── cluster/
│   │   ├── jaccard.go             # Jaccard similarity computation
│   │   ├── article.go             # Article + feed embedding computation, vec0 KNN content boost, language detection
│   │   ├── scoring.go             # Feed + people + article recommendation queries (on-demand)
│   │   ├── social.go              # Incremental follow-distance computation (1-3 hop, dirty-flag)
│   │   ├── weights.go             # Bandit-style signal weight auto-tuning
│   │   ├── diversity.go           # Post-query domain/category diversity filtering
│   │   └── cron.go                # Background recomputation scheduler
│   ├── feedback/
│   │   └── feedback.go            # Dismiss + impression tracking service
│   ├── server/
│   │   ├── server.go              # HTTP server, router setup
│   │   ├── auth_handler.go        # OAuth login/callback/register
│   │   ├── feeds_handler.go       # Feed management handlers
│   │   ├── articles_handler.go    # Article reading handlers
│   │   ├── annotations_handler.go # Annotation handlers
│   │   ├── dashboard_handler.go   # Dashboard handler
│   │   ├── trending_handler.go    # Trending handler
│   │   ├── stats_handler.go       # Stats handler (Prometheus metrics display)
│   │   ├── index_handler.go       # Landing page handler
│   │   ├── profile_handler.go     # Public profile handler
│   │   ├── settings_handler.go    # User settings (language preferences)
│   │   ├── terms_handler.go       # Terms of service handler
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
│       ├── stats.html             # Application metrics
│       ├── library.html           # Liked articles + annotations
│       ├── profile.html           # User profile
│       ├── error.html             # Error page
│       ├── 404.html               # Not found page
│       ├── terms.html             # Terms of service
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

Session cookies are HMAC-signed using `GLEAN_SESSION_KEY` (required, must be set to a random string). The server refuses to start without it.

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
                                       └─◄ Redirect to `/feeds`
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
Cron (every 1h) ──► Cluster Engine
                           │
                           ├─► Compute feed similarity
                           ├─► Compute user similarity
                           ├─► Compute article embeddings (if embedder configured)
                           ├─► Detect article languages (if LLM configured)
                           ├─► Compute follow distances
                           ├─► Compute signal profiles
                           └─► Auto-dismiss stale recommendations

Browser ──GET /dashboard──► Server
                                │
                                ├─► Compute recommendations on-demand (filtered by user's language preferences)
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

- **`glean_feeds_fetched_total`** — Total feed fetch attempts (counter)
- **`glean_feeds_fetched_last_timestamp_seconds`** — Unix timestamp of last feed fetch (gauge)
- **`glean_feed_fetch_duration_seconds`** — Histogram of feed fetch latency
- **`glean_articles_upserted_total`** — Counter of articles stored from feeds
- **`glean_jetstream_events_total`** — Jetstream events labeled by collection and action
- **`glean_jetstream_errors_total`** — Jetstream handler errors
- **`glean_jetstream_reconnects_total`** — Jetstream reconnection count
- **`glean_http_requests_total`** — HTTP request counts labeled by method, path, and status
- **`glean_http_request_duration_seconds`** — HTTP request duration labeled by method and path
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

- **Email digest**: Periodic email with top articles from subscribed feeds
