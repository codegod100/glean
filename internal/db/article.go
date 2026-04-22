package db

import (
	"context"
	"database/sql"
	"strings"
	"time"
	"unicode"

	"pkg.rbrt.fr/glean/internal/feed"
)

type Article struct {
	ID             int64
	FeedURL        string
	FeedTitle      string
	FeedFaviconURL sql.NullString
	GUID           string
	Title          string
	URL            sql.NullString
	Author         sql.NullString
	Summary        sql.NullString
	Content        sql.NullString
	FullContent    sql.NullString
	Published      sql.NullTime
	Updated        sql.NullTime
	FetchedAt      sql.NullTime
	IsRead         sql.NullBool
	LikeCount      int
	HasLiked       bool
}

type ReadState struct {
	UserDID   string
	ArticleID int64
	IsRead    bool
	ReadAt    sql.NullTime
}

func (db *DB) UpsertArticle(ctx context.Context, article *Article) (int64, error) {
	var id int64
	err := db.QueryRowContext(ctx, `
		INSERT INTO articles (feed_url, guid, title, url, author, summary, content, published, updated)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(feed_url, guid) DO NOTHING
		RETURNING id
	`, article.FeedURL, article.GUID, article.Title, article.URL, article.Author,
		article.Summary, article.Content, article.Published, article.Updated).Scan(&id)
	if err == sql.ErrNoRows {
		err = db.QueryRowContext(ctx, `
			SELECT id FROM articles WHERE feed_url = ? AND guid = ?
		`, article.FeedURL, article.GUID).Scan(&id)
	}
	return id, err
}

func (db *DB) UpsertArticlesBatch(ctx context.Context, articles []feed.Article) error {
	if len(articles) == 0 {
		return nil
	}

	err := upsertArticlesBatch(ctx, db, articles)
	if err != nil {
		err = upsertArticlesBatch(ctx, db, articles)
	}
	return err
}

func upsertArticlesBatch(ctx context.Context, db *DB, articles []feed.Article) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO articles (feed_url, guid, title, url, author, summary, content, published, updated)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(feed_url, guid) DO NOTHING
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, a := range articles {
		url := sql.NullString{String: a.URL, Valid: a.URL != ""}
		author := sql.NullString{String: a.Author, Valid: a.Author != ""}
		summary := sql.NullString{String: a.Summary, Valid: a.Summary != ""}
		content := sql.NullString{String: a.Content, Valid: a.Content != ""}
		var published, updated sql.NullTime
		if !a.Published.IsZero() {
			published = sql.NullTime{Time: a.Published, Valid: true}
		}
		if !a.Updated.IsZero() {
			updated = sql.NullTime{Time: a.Updated, Valid: true}
		}

		if _, err := stmt.ExecContext(ctx, a.FeedURL, a.GUID, a.Title, url, author, summary, content, published, updated); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (db *DB) GetArticle(ctx context.Context, id int64) (*Article, error) {
	a := &Article{}
	err := db.QueryRowContext(ctx, `
		SELECT id, feed_url, guid, title, url, author, summary, content, full_content, published, updated, fetched_at
		FROM articles WHERE id = ?
	`, id).Scan(&a.ID, &a.FeedURL, &a.GUID, &a.Title, &a.URL, &a.Author,
		&a.Summary, &a.Content, &a.FullContent, &a.Published, &a.Updated, &a.FetchedAt)
	if err != nil {
		return nil, err
	}
	return a, nil
}

func (db *DB) ListArticles(ctx context.Context, userDID, feedURL string, limit, offset int) ([]*Article, error) {
	var query string
	var args []any

	if feedURL != "" {
		query = `
			SELECT a.id, a.feed_url, COALESCE(f.title, ''), f.favicon_url, a.guid, a.title, a.url, a.author, a.summary, a.content,
				a.published, a.updated, a.fetched_at,
				COALESCE(r.is_read, 0),
				COALESCE(lc.cnt, 0),
				COALESCE(ul.liked, 0)
			FROM articles a
			LEFT JOIN feeds f ON a.feed_url = f.feed_url
			LEFT JOIN read_state r ON r.user_did = ? AND r.article_id = a.id
			LEFT JOIN (SELECT feed_url, article_url, COUNT(*) as cnt FROM likes GROUP BY feed_url, article_url) lc
				ON lc.feed_url = a.feed_url AND lc.article_url = a.url
			LEFT JOIN (SELECT feed_url, article_url, 1 as liked FROM likes WHERE author_did = ?) ul
				ON ul.feed_url = a.feed_url AND ul.article_url = a.url
			WHERE a.feed_url = ?
		`
		args = []any{userDID, userDID, feedURL}
	} else {
		query = `
			SELECT a.id, a.feed_url, COALESCE(f.title, ''), f.favicon_url, a.guid, a.title, a.url, a.author, a.summary, a.content,
				a.published, a.updated, a.fetched_at,
				COALESCE(r.is_read, 0),
				COALESCE(lc.cnt, 0),
				COALESCE(ul.liked, 0)
			FROM articles a
			JOIN subscriptions s ON a.feed_url = s.feed_url AND s.user_did = ?
			LEFT JOIN feeds f ON a.feed_url = f.feed_url
			LEFT JOIN read_state r ON r.user_did = ? AND r.article_id = a.id
			LEFT JOIN (SELECT feed_url, article_url, COUNT(*) as cnt FROM likes GROUP BY feed_url, article_url) lc
				ON lc.feed_url = a.feed_url AND lc.article_url = a.url
			LEFT JOIN (SELECT feed_url, article_url, 1 as liked FROM likes WHERE author_did = ?) ul
				ON ul.feed_url = a.feed_url AND ul.article_url = a.url
			WHERE 1=1
		`
		args = []any{userDID, userDID, userDID}
	}

	// Future-published articles (e.g., scheduled) sort last
	query += ` ORDER BY (CASE WHEN a.published > 'now' THEN 1 ELSE 0 END), a.published DESC LIMIT ? OFFSET ?`
	args = append(args, limit, offset)

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var articles []*Article
	for rows.Next() {
		a := &Article{}
		if err := rows.Scan(&a.ID, &a.FeedURL, &a.FeedTitle, &a.FeedFaviconURL, &a.GUID, &a.Title, &a.URL, &a.Author,
			&a.Summary, &a.Content, &a.Published, &a.Updated, &a.FetchedAt,
			&a.IsRead, &a.LikeCount, &a.HasLiked); err != nil {
			return nil, err
		}
		articles = append(articles, a)
	}
	return articles, rows.Err()
}

func (db *DB) ListUnreadArticles(ctx context.Context, userDID, feedURL string, limit, offset int) ([]*Article, error) {
	var query string
	var args []any

	if feedURL != "" {
		query = `
			SELECT a.id, a.feed_url, COALESCE(f.title, ''), f.favicon_url, a.guid, a.title, a.url, a.author, a.summary, a.content,
				a.published, a.updated, a.fetched_at,
				COALESCE(r.is_read, 0),
				COALESCE(lc.cnt, 0),
				COALESCE(ul.liked, 0)
			FROM articles a
			LEFT JOIN feeds f ON a.feed_url = f.feed_url
			LEFT JOIN read_state r ON r.user_did = ? AND r.article_id = a.id
			LEFT JOIN (SELECT feed_url, article_url, COUNT(*) as cnt FROM likes GROUP BY feed_url, article_url) lc
				ON lc.feed_url = a.feed_url AND lc.article_url = a.url
			LEFT JOIN (SELECT feed_url, article_url, 1 as liked FROM likes WHERE author_did = ?) ul
				ON ul.feed_url = a.feed_url AND ul.article_url = a.url
			WHERE a.feed_url = ? AND (r.is_read = 0 OR r.is_read IS NULL)
		`
		args = []any{userDID, userDID, feedURL}
	} else {
		query = `
			SELECT a.id, a.feed_url, COALESCE(f.title, ''), f.favicon_url, a.guid, a.title, a.url, a.author, a.summary, a.content,
				a.published, a.updated, a.fetched_at,
				COALESCE(r.is_read, 0),
				COALESCE(lc.cnt, 0),
				COALESCE(ul.liked, 0)
			FROM articles a
			JOIN subscriptions s ON a.feed_url = s.feed_url AND s.user_did = ?
			LEFT JOIN feeds f ON a.feed_url = f.feed_url
			LEFT JOIN read_state r ON r.user_did = ? AND r.article_id = a.id
			LEFT JOIN (SELECT feed_url, article_url, COUNT(*) as cnt FROM likes GROUP BY feed_url, article_url) lc
				ON lc.feed_url = a.feed_url AND lc.article_url = a.url
			LEFT JOIN (SELECT feed_url, article_url, 1 as liked FROM likes WHERE author_did = ?) ul
				ON ul.feed_url = a.feed_url AND ul.article_url = a.url
			WHERE (r.is_read = 0 OR r.is_read IS NULL)
		`
		args = []any{userDID, userDID, userDID}
	}

	// Future-published articles (e.g., scheduled) sort last
	query += ` ORDER BY (CASE WHEN a.published > 'now' THEN 1 ELSE 0 END), a.published DESC LIMIT ? OFFSET ?`
	args = append(args, limit, offset)

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var articles []*Article
	for rows.Next() {
		a := &Article{}
		if err := rows.Scan(&a.ID, &a.FeedURL, &a.FeedTitle, &a.FeedFaviconURL, &a.GUID, &a.Title, &a.URL, &a.Author,
			&a.Summary, &a.Content, &a.Published, &a.Updated, &a.FetchedAt,
			&a.IsRead, &a.LikeCount, &a.HasLiked); err != nil {
			return nil, err
		}
		articles = append(articles, a)
	}
	return articles, rows.Err()
}

func (db *DB) ListReadArticles(ctx context.Context, userDID, feedURL string, limit, offset int) ([]*Article, error) {
	var query string
	var args []any

	if feedURL != "" {
		query = `
			SELECT a.id, a.feed_url, COALESCE(f.title, ''), f.favicon_url, a.guid, a.title, a.url, a.author, a.summary, a.content,
				a.published, a.updated, a.fetched_at,
				COALESCE(r.is_read, 0),
				COALESCE(lc.cnt, 0),
				COALESCE(ul.liked, 0)
			FROM articles a
			LEFT JOIN feeds f ON a.feed_url = f.feed_url
			JOIN read_state r ON r.user_did = ? AND r.article_id = a.id
			LEFT JOIN (SELECT feed_url, article_url, COUNT(*) as cnt FROM likes GROUP BY feed_url, article_url) lc
				ON lc.feed_url = a.feed_url AND lc.article_url = a.url
			LEFT JOIN (SELECT feed_url, article_url, 1 as liked FROM likes WHERE author_did = ?) ul
				ON ul.feed_url = a.feed_url AND ul.article_url = a.url
			WHERE r.is_read = 1 AND a.feed_url = ?
		`
		args = []any{userDID, userDID, feedURL}
	} else {
		query = `
			SELECT a.id, a.feed_url, COALESCE(f.title, ''), f.favicon_url, a.guid, a.title, a.url, a.author, a.summary, a.content,
				a.published, a.updated, a.fetched_at,
				COALESCE(r.is_read, 0),
				COALESCE(lc.cnt, 0),
				COALESCE(ul.liked, 0)
			FROM articles a
			JOIN subscriptions s ON a.feed_url = s.feed_url AND s.user_did = ?
			LEFT JOIN feeds f ON a.feed_url = f.feed_url
			JOIN read_state r ON r.user_did = ? AND r.article_id = a.id
			LEFT JOIN (SELECT feed_url, article_url, COUNT(*) as cnt FROM likes GROUP BY feed_url, article_url) lc
				ON lc.feed_url = a.feed_url AND lc.article_url = a.url
			LEFT JOIN (SELECT feed_url, article_url, 1 as liked FROM likes WHERE author_did = ?) ul
				ON ul.feed_url = a.feed_url AND ul.article_url = a.url
			WHERE r.is_read = 1
		`
		args = []any{userDID, userDID, userDID}
	}

	// Future-published articles (e.g., scheduled) sort last
	query += ` ORDER BY (CASE WHEN a.published > 'now' THEN 1 ELSE 0 END), a.published DESC LIMIT ? OFFSET ?`
	args = append(args, limit, offset)

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var articles []*Article
	for rows.Next() {
		a := &Article{}
		if err := rows.Scan(&a.ID, &a.FeedURL, &a.FeedTitle, &a.FeedFaviconURL, &a.GUID, &a.Title, &a.URL, &a.Author,
			&a.Summary, &a.Content, &a.Published, &a.Updated, &a.FetchedAt,
			&a.IsRead, &a.LikeCount, &a.HasLiked); err != nil {
			return nil, err
		}
		articles = append(articles, a)
	}
	return articles, rows.Err()
}

func (db *DB) MarkArticleRead(ctx context.Context, userDID string, articleID int64) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO read_state (user_did, article_id, is_read, read_at)
		VALUES (?, ?, 1, CURRENT_TIMESTAMP)
		ON CONFLICT(user_did, article_id) DO UPDATE SET
			is_read = 1, read_at = CURRENT_TIMESTAMP
	`, userDID, articleID)
	return err
}

func (db *DB) MarkArticleUnread(ctx context.Context, userDID string, articleID int64) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO read_state (user_did, article_id, is_read)
		VALUES (?, ?, 0)
		ON CONFLICT(user_did, article_id) DO UPDATE SET
			is_read = 0, read_at = NULL
	`, userDID, articleID)
	return err
}

func (db *DB) MarkAllRead(ctx context.Context, userDID, feedURL string) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO read_state (user_did, article_id, is_read, read_at)
		SELECT ?, a.id, 1, CURRENT_TIMESTAMP
		FROM articles a
		WHERE a.feed_url = ?
		ON CONFLICT(user_did, article_id) DO UPDATE SET
			is_read = 1, read_at = CURRENT_TIMESTAMP
	`, userDID, feedURL)
	return err
}

func (db *DB) MarkAllSubscribedRead(ctx context.Context, userDID string) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO read_state (user_did, article_id, is_read, read_at)
		SELECT ?, a.id, 1, CURRENT_TIMESTAMP
		FROM articles a
		JOIN subscriptions s ON a.feed_url = s.feed_url AND s.user_did = ?
		ON CONFLICT(user_did, article_id) DO UPDATE SET
			is_read = 1, read_at = CURRENT_TIMESTAMP
	`, userDID, userDID)
	return err
}

func (db *DB) GetReadState(ctx context.Context, userDID string, articleID int64) (*ReadState, error) {
	rs := &ReadState{}
	err := db.QueryRowContext(ctx, `
		SELECT user_did, article_id, is_read, read_at
		FROM read_state WHERE user_did = ? AND article_id = ?
	`, userDID, articleID).Scan(&rs.UserDID, &rs.ArticleID, &rs.IsRead, &rs.ReadAt)
	if err == sql.ErrNoRows {
		return &ReadState{UserDID: userDID, ArticleID: articleID}, nil
	}
	if err != nil {
		return nil, err
	}
	return rs, nil
}

func (db *DB) GetUnreadCount(ctx context.Context, userDID, feedURL string) (int, error) {
	var count int
	if feedURL != "" {
		err := db.QueryRowContext(ctx, `
			SELECT COUNT(*)
			FROM articles a
			LEFT JOIN read_state r ON r.user_did = ? AND r.article_id = a.id
			WHERE a.feed_url = ? AND (r.is_read = 0 OR r.is_read IS NULL)
		`, userDID, feedURL).Scan(&count)
		return count, err
	}
	err := db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM articles a
		JOIN subscriptions s ON a.feed_url = s.feed_url AND s.user_did = ?
		LEFT JOIN read_state r ON r.user_did = ? AND r.article_id = a.id
		WHERE r.is_read = 0 OR r.is_read IS NULL
	`, userDID, userDID).Scan(&count)
	return count, err
}

func (db *DB) UpdateArticleFullContent(ctx context.Context, id int64, fullContent string) error {
	_, err := db.ExecContext(ctx, `
		UPDATE articles SET full_content = ? WHERE id = ?
	`, fullContent, id)
	return err
}

func (db *DB) GetArticleByURL(ctx context.Context, url string) (*Article, error) {
	a := &Article{}
	err := db.QueryRowContext(ctx, `
		SELECT id, feed_url, guid, title, url, author, summary, content, full_content, published, updated, fetched_at
		FROM articles WHERE url = ?
		LIMIT 1
	`, url).Scan(&a.ID, &a.FeedURL, &a.GUID, &a.Title, &a.URL, &a.Author,
		&a.Summary, &a.Content, &a.FullContent, &a.Published, &a.Updated, &a.FetchedAt)
	if err != nil {
		return nil, err
	}
	return a, nil
}

func (db *DB) CountNewArticles(ctx context.Context, userDID string, since time.Time) (int, error) {
	var count int
	err := db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM articles a
		JOIN subscriptions s ON a.feed_url = s.feed_url AND s.user_did = ?
		WHERE a.fetched_at > ?
	`, userDID, since).Scan(&count)
	return count, err
}

func escapeFTS5(query string) string {
	var b strings.Builder
	b.Grow(len(query))
	for _, r := range query {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsSpace(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func (db *DB) SearchArticles(ctx context.Context, userDID, query string, limit, offset int) ([]*Article, error) {
	if strings.TrimSpace(query) == "" {
		return nil, nil
	}

	safeQuery := escapeFTS5(query)
	if strings.TrimSpace(safeQuery) == "" {
		return nil, nil
	}

	columnQuery := "{title summary} : " + safeQuery

	rows, err := db.QueryContext(ctx, `
		SELECT a.id, a.feed_url, COALESCE(f.title, ''), f.favicon_url, a.guid, a.title, a.url, a.author, a.summary, a.content,
			a.published, a.updated, a.fetched_at,
			COALESCE(r.is_read, 0),
			COALESCE(lc.cnt, 0),
			COALESCE(ul.liked, 0)
		FROM articles_fts ft
		JOIN articles a ON a.id = ft.rowid
		JOIN subscriptions s ON a.feed_url = s.feed_url AND s.user_did = ?
		LEFT JOIN feeds f ON a.feed_url = f.feed_url
		LEFT JOIN read_state r ON r.user_did = ? AND r.article_id = a.id
		LEFT JOIN (SELECT feed_url, article_url, COUNT(*) as cnt FROM likes GROUP BY feed_url, article_url) lc
			ON lc.feed_url = a.feed_url AND lc.article_url = a.url
		LEFT JOIN (SELECT feed_url, article_url, 1 as liked FROM likes WHERE author_did = ?) ul
			ON ul.feed_url = a.feed_url AND ul.article_url = a.url
		WHERE articles_fts MATCH ?
		ORDER BY ft.rank
		LIMIT ? OFFSET ?
	`, userDID, userDID, userDID, columnQuery, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var articles []*Article
	for rows.Next() {
		a := &Article{}
		if err := rows.Scan(&a.ID, &a.FeedURL, &a.FeedTitle, &a.FeedFaviconURL, &a.GUID, &a.Title, &a.URL, &a.Author,
			&a.Summary, &a.Content, &a.Published, &a.Updated, &a.FetchedAt,
			&a.IsRead, &a.LikeCount, &a.HasLiked); err != nil {
			return nil, err
		}
		articles = append(articles, a)
	}
	return articles, rows.Err()
}
