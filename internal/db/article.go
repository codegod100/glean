package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
	"unicode"

	"pkg.rbrt.fr/glean/internal/feed"
)

const articlesOrderByDesc = ` ORDER BY (CASE WHEN a.published > 'now' THEN 1 ELSE 0 END), a.published DESC LIMIT ? OFFSET ?`
const articlesOrderByAsc = ` ORDER BY (CASE WHEN a.published > 'now' THEN 1 ELSE 0 END), a.published ASC LIMIT ? OFFSET ?`

type ArticleStore struct {
	db *DB
	// retentionDays mirrors the article retention window. Ingest drops entries
	// already older than it, so the purge is not undone by the next fetch of a
	// feed that still serves its whole history. Zero disables the cutoff.
	retentionDays int
}

func NewArticleStore(db *DB) *ArticleStore {
	return &ArticleStore{db: db}
}

// SetRetentionDays sets the ingest age cutoff. It must match the window passed
// to PurgeExpiredArticles.
func (s *ArticleStore) SetRetentionDays(days int) {
	s.retentionDays = days
}

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
	// NavSuffix holds the query string appended to article detail links to preserve
	// listing context (feed scope, liked) for next-article navigation.
	NavSuffix string
	// DismissURL, when non-empty, shows a dismiss button that POSTs to this URL.
	DismissURL   string
	DismissField string
	DismissValue string
}

type ReadState struct {
	UserDID   string
	ArticleID int64
	IsRead    bool
	ReadAt    sql.NullTime
}

func (s *ArticleStore) BatchUpsertArticles(ctx context.Context, articles []feed.Article) error {
	if len(articles) == 0 {
		return nil
	}

	// Anything already past the retention window would be deleted by the next
	// purge; ingesting it only churns the database and, before read state was
	// archived, resurfaced long-read articles as unread.
	var cutoff time.Time
	if s.retentionDays > 0 {
		cutoff = time.Now().AddDate(0, 0, -s.retentionDays)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO articles.articles (feed_url, guid, title, url, author, summary, content, published, updated, language)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, '')
		ON CONFLICT(feed_url, guid) DO NOTHING
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	feedURLs := make(map[string]struct{}, 1)
	for _, a := range articles {
		if !cutoff.IsZero() && !a.Published.IsZero() && a.Published.Before(cutoff) {
			continue
		}
		feedURLs[a.FeedURL] = struct{}{}
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

	// Restore read state for entries that were purged and have now come back
	// under a new surrogate id. Without this they would read as unread again.
	restore, err := tx.PrepareContext(ctx, `
		INSERT INTO articles.read_state (user_did, article_id, is_read, read_at)
		SELECT h.user_did, a.id, h.is_read, h.read_at
		FROM articles.read_state_history h
		JOIN articles.articles a ON a.feed_url = h.feed_url AND a.guid = h.guid
		WHERE h.feed_url = ?
		ON CONFLICT(user_did, article_id) DO NOTHING
	`)
	if err != nil {
		return err
	}
	defer restore.Close()
	for feedURL := range feedURLs {
		if _, err := restore.ExecContext(ctx, feedURL); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (s *ArticleStore) GetArticle(ctx context.Context, id int64) (*Article, error) {
	a := &Article{}
	err := s.db.QueryRowContext(ctx, `
		SELECT id, feed_url, guid, title, url, author, summary, content, full_content, published, updated, fetched_at
		FROM articles.articles WHERE id = ?
	`, id).Scan(&a.ID, &a.FeedURL, &a.GUID, &a.Title, &a.URL, &a.Author,
		&a.Summary, &a.Content, &a.FullContent, &a.Published, &a.Updated, &a.FetchedAt)
	if err != nil {
		return nil, err
	}
	return a, nil
}

func (s *ArticleStore) ListArticles(
	ctx context.Context,
	userDID, feedURL, category string,
	limit, offset int,
	sortOldest bool,
) ([]*Article, error) {
	var (
		query string
		args  []any
	)

	if feedURL != "" {
		query = `
			SELECT a.id, a.feed_url, COALESCE(f.title, ''), f.favicon_url, a.guid, a.title, a.url, a.author, a.summary, a.content,
				a.published, a.updated, a.fetched_at,
				COALESCE(r.is_read, 0),
				(SELECT COUNT(*) FROM articles.likes l2 WHERE l2.feed_url = a.feed_url AND l2.article_url = a.url),
				COALESCE((SELECT 1 FROM articles.likes l3 WHERE l3.feed_url = a.feed_url AND l3.article_url = a.url AND l3.author_did = ?), 0)
			FROM articles.articles a
			LEFT JOIN articles.feeds f ON a.feed_url = f.feed_url
			LEFT JOIN articles.read_state r ON r.user_did = ? AND r.article_id = a.id
			WHERE a.feed_url = ?
		`
		args = []any{userDID, userDID, feedURL}
	} else {
		query = `
			SELECT a.id, a.feed_url, COALESCE(f.title, ''), f.favicon_url, a.guid, a.title, a.url, a.author, a.summary, a.content,
				a.published, a.updated, a.fetched_at,
				COALESCE(r.is_read, 0),
				(SELECT COUNT(*) FROM articles.likes l2 WHERE l2.feed_url = a.feed_url AND l2.article_url = a.url),
				COALESCE((SELECT 1 FROM articles.likes l3 WHERE l3.feed_url = a.feed_url AND l3.article_url = a.url AND l3.author_did = ?), 0)
			FROM articles.articles a
			JOIN articles.subscriptions s ON a.feed_url = s.feed_url AND s.user_did = ?
			LEFT JOIN articles.feeds f ON a.feed_url = f.feed_url
			LEFT JOIN articles.read_state r ON r.user_did = ? AND r.article_id = a.id
			WHERE 1=1
		`
		args = []any{userDID, userDID, userDID}
		query, args = addCategoryWhere(query, args, category)
	}

	orderBy := articlesOrderByDesc
	if sortOldest {
		orderBy = articlesOrderByAsc
	}
	query += orderBy
	args = append(args, limit, offset)

	rows, err := s.db.QueryContext(ctx, query, args...)
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

func addCategoryWhere(query string, args []any, category string) (string, []any) {
	switch category {
	case "__none__":
		query += ` AND (s.category IS NULL OR s.category = '')`
	case "":
		// no-op
	default:
		query += ` AND s.category = ?`
		args = append(args, category)
	}

	return query, args
}

func (s *ArticleStore) ListUnreadArticles(
	ctx context.Context,
	userDID, feedURL, category string,
	limit, offset int,
	sortOldest bool,
) ([]*Article, error) {
	var query string
	var args []any

	if feedURL != "" {
		query = `
			SELECT a.id, a.feed_url, COALESCE(f.title, ''), f.favicon_url, a.guid, a.title, a.url, a.author, a.summary, a.content,
				a.published, a.updated, a.fetched_at,
				COALESCE(r.is_read, 0),
				(SELECT COUNT(*) FROM articles.likes l2 WHERE l2.feed_url = a.feed_url AND l2.article_url = a.url),
				COALESCE((SELECT 1 FROM articles.likes l3 WHERE l3.feed_url = a.feed_url AND l3.article_url = a.url AND l3.author_did = ?), 0)
			FROM articles.articles a
			LEFT JOIN articles.feeds f ON a.feed_url = f.feed_url
			LEFT JOIN articles.read_state r ON r.user_did = ? AND r.article_id = a.id
			WHERE a.feed_url = ? AND (r.is_read = 0 OR r.is_read IS NULL)
		`
		args = []any{userDID, userDID, feedURL}
	} else {
		query = `
			SELECT a.id, a.feed_url, COALESCE(f.title, ''), f.favicon_url, a.guid, a.title, a.url, a.author, a.summary, a.content,
				a.published, a.updated, a.fetched_at,
				COALESCE(r.is_read, 0),
				(SELECT COUNT(*) FROM articles.likes l2 WHERE l2.feed_url = a.feed_url AND l2.article_url = a.url),
				COALESCE((SELECT 1 FROM articles.likes l3 WHERE l3.feed_url = a.feed_url AND l3.article_url = a.url AND l3.author_did = ?), 0)
			FROM articles.articles a
			JOIN articles.subscriptions s ON a.feed_url = s.feed_url AND s.user_did = ?
			LEFT JOIN articles.feeds f ON a.feed_url = f.feed_url
			LEFT JOIN articles.read_state r ON r.user_did = ? AND r.article_id = a.id
			WHERE (r.is_read = 0 OR r.is_read IS NULL)
		`
		args = []any{userDID, userDID, userDID}
		query, args = addCategoryWhere(query, args, category)
	}

	orderBy := articlesOrderByDesc
	if sortOldest {
		orderBy = articlesOrderByAsc
	}
	query += orderBy
	args = append(args, limit, offset)

	rows, err := s.db.QueryContext(ctx, query, args...)
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

func (s *ArticleStore) ListReadArticles(
	ctx context.Context,
	userDID, feedURL, category string,
	limit, offset int,
	sortOldest bool,
) ([]*Article, error) {
	var query string
	var args []any

	if feedURL != "" {
		query = `
			SELECT a.id, a.feed_url, COALESCE(f.title, ''), f.favicon_url, a.guid, a.title, a.url, a.author, a.summary, a.content,
				a.published, a.updated, a.fetched_at,
				COALESCE(r.is_read, 0),
				(SELECT COUNT(*) FROM articles.likes l2 WHERE l2.feed_url = a.feed_url AND l2.article_url = a.url),
				COALESCE((SELECT 1 FROM articles.likes l3 WHERE l3.feed_url = a.feed_url AND l3.article_url = a.url AND l3.author_did = ?), 0)
			FROM articles.articles a
			LEFT JOIN articles.feeds f ON a.feed_url = f.feed_url
			JOIN articles.read_state r ON r.user_did = ? AND r.article_id = a.id
			WHERE r.is_read = 1 AND a.feed_url = ?
		`
		args = []any{userDID, userDID, feedURL}
	} else {
		query = `
			SELECT a.id, a.feed_url, COALESCE(f.title, ''), f.favicon_url, a.guid, a.title, a.url, a.author, a.summary, a.content,
				a.published, a.updated, a.fetched_at,
				COALESCE(r.is_read, 0),
				(SELECT COUNT(*) FROM articles.likes l2 WHERE l2.feed_url = a.feed_url AND l2.article_url = a.url),
				COALESCE((SELECT 1 FROM articles.likes l3 WHERE l3.feed_url = a.feed_url AND l3.article_url = a.url AND l3.author_did = ?), 0)
			FROM articles.articles a
			JOIN articles.subscriptions s ON a.feed_url = s.feed_url AND s.user_did = ?
			LEFT JOIN articles.feeds f ON a.feed_url = f.feed_url
			JOIN articles.read_state r ON r.user_did = ? AND r.article_id = a.id
			WHERE r.is_read = 1
		`
		args = []any{userDID, userDID, userDID}
		query, args = addCategoryWhere(query, args, category)
	}

	orderBy := articlesOrderByDesc
	if sortOldest {
		orderBy = articlesOrderByAsc
	}
	query += orderBy
	args = append(args, limit, offset)

	rows, err := s.db.QueryContext(ctx, query, args...)
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

func (s *ArticleStore) MarkArticleRead(ctx context.Context, userDID string, articleID int64) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO articles.read_state (user_did, article_id, is_read, read_at)
		VALUES (?, ?, 1, CURRENT_TIMESTAMP)
		ON CONFLICT(user_did, article_id) DO UPDATE SET
			is_read = 1, read_at = CURRENT_TIMESTAMP
	`, userDID, articleID)
	return err
}

func (s *ArticleStore) MarkArticleUnread(ctx context.Context, userDID string, articleID int64) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO articles.read_state (user_did, article_id, is_read)
		VALUES (?, ?, 0)
		ON CONFLICT(user_did, article_id) DO UPDATE SET
			is_read = 0, read_at = NULL
	`, userDID, articleID)
	return err
}

func (s *ArticleStore) MarkAllRead(ctx context.Context, userDID, feedURL string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO articles.read_state (user_did, article_id, is_read, read_at)
		SELECT ?, a.id, 1, CURRENT_TIMESTAMP
		FROM articles.articles a
		WHERE a.feed_url = ?
		ON CONFLICT(user_did, article_id) DO UPDATE SET
			is_read = 1, read_at = CURRENT_TIMESTAMP
	`, userDID, feedURL)
	return err
}

func (s *ArticleStore) MarkAllSubscribedRead(ctx context.Context, userDID string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO articles.read_state (user_did, article_id, is_read, read_at)
		SELECT ?, a.id, 1, CURRENT_TIMESTAMP
		FROM articles.articles a
		JOIN articles.subscriptions s ON a.feed_url = s.feed_url AND s.user_did = ?
		ON CONFLICT(user_did, article_id) DO UPDATE SET
			is_read = 1, read_at = CURRENT_TIMESTAMP
	`, userDID, userDID)
	return err
}

func (s *ArticleStore) MarkArticlesRead(ctx context.Context, userDID string, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO articles.read_state (user_did, article_id, is_read, read_at)
		VALUES (?, ?, 1, CURRENT_TIMESTAMP)
		ON CONFLICT(user_did, article_id) DO UPDATE SET
			is_read = 1, read_at = CURRENT_TIMESTAMP
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, id := range ids {
		if _, err := stmt.ExecContext(ctx, userDID, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *ArticleStore) GetReadState(ctx context.Context, userDID string, articleID int64) (*ReadState, error) {
	rs := &ReadState{}
	err := s.db.QueryRowContext(ctx, `
		SELECT * FROM articles.read_state WHERE user_did = ? AND article_id = ?
	`, userDID, articleID).Scan(&rs.UserDID, &rs.ArticleID, &rs.IsRead, &rs.ReadAt)
	if err == sql.ErrNoRows {
		return &ReadState{UserDID: userDID, ArticleID: articleID}, nil
	}
	if err != nil {
		return nil, err
	}
	return rs, nil
}

func (s *ArticleStore) GetUnreadCount(ctx context.Context, userDID, feedURL, category string) (int, error) {
	var count int
	if feedURL != "" {
		err := s.db.QueryRowContext(ctx, `
			SELECT COUNT(*)
			FROM articles.articles a
			LEFT JOIN articles.read_state r ON r.user_did = ? AND r.article_id = a.id
			WHERE a.feed_url = ? AND (r.is_read = 0 OR r.is_read IS NULL)
		`, userDID, feedURL).Scan(&count)
		return count, err
	}
	query := `
		SELECT COUNT(*)
		FROM articles.articles a
		JOIN articles.subscriptions s ON a.feed_url = s.feed_url AND s.user_did = ?
		LEFT JOIN articles.read_state r ON r.user_did = ? AND r.article_id = a.id
		WHERE (r.is_read = 0 OR r.is_read IS NULL)
	`
	args := []any{userDID, userDID}
	query, args = addCategoryWhere(query, args, category)
	err := s.db.QueryRowContext(ctx, query, args...).Scan(&count)
	return count, err
}

func (s *ArticleStore) UpdateArticleFullContent(ctx context.Context, id int64, fullContent string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE articles.articles SET full_content = ? WHERE id = ?
	`, fullContent, id)
	return err
}

func (s *ArticleStore) GetArticleByURL(ctx context.Context, url string) (*Article, error) {
	a := &Article{}
	err := s.db.QueryRowContext(ctx, `
		SELECT id, feed_url, guid, title, url, author, summary, content, full_content, published, updated, fetched_at
		FROM articles.articles WHERE url = ?
		LIMIT 1
	`, url).Scan(&a.ID, &a.FeedURL, &a.GUID, &a.Title, &a.URL, &a.Author,
		&a.Summary, &a.Content, &a.FullContent, &a.Published, &a.Updated, &a.FetchedAt)
	if err != nil {
		return nil, err
	}
	return a, nil
}

func (s *ArticleStore) DeleteArticleByGUID(ctx context.Context, guid string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM articles.articles WHERE guid = ?`, guid)
	return err
}

func (s *ArticleStore) CountNewArticles(ctx context.Context, userDID string, since time.Time) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM articles.articles a
		JOIN articles.subscriptions s ON a.feed_url = s.feed_url AND s.user_did = ?
		WHERE a.fetched_at > ?
	`, userDID, since).Scan(&count)
	return count, err
}

func (s *ArticleStore) GetNextArticleID(ctx context.Context, userDID string, articleID int64, feedURL string, liked bool, status string) (*int64, error) {
	var fromParts []string
	var whereParts []string

	fromParts = append(fromParts, "articles.articles a")

	if feedURL != "" {
		whereParts = append(whereParts, "a.feed_url = ?")
	} else {
		fromParts = append(fromParts, "JOIN articles.subscriptions s ON a.feed_url = s.feed_url AND s.user_did = ?")
	}

	if liked {
		fromParts = append(fromParts, "JOIN articles.likes l ON l.author_did = ? AND l.article_url = a.url")
	}

	switch status {
	case "unread":
		fromParts = append(fromParts, "LEFT JOIN articles.read_state rs ON rs.user_did = ? AND rs.article_id = a.id")
		whereParts = append(whereParts, "(rs.is_read = 0 OR rs.is_read IS NULL)")
	case "read":
		fromParts = append(fromParts, "JOIN articles.read_state rs ON rs.user_did = ? AND rs.article_id = a.id")
		whereParts = append(whereParts, "rs.is_read = 1")
	}

	whereParts = append(whereParts, "a.id != ?")

	fromClause := strings.Join(fromParts, " ")
	whereClause := strings.Join(whereParts, " AND ")

	query := fmt.Sprintf(`
		WITH cur AS (SELECT published FROM articles.articles WHERE id = ?)
		SELECT a.id FROM %s, cur
		WHERE %s AND (
			(CASE WHEN a.published > 'now' THEN 1 ELSE 0 END) > (CASE WHEN cur.published > 'now' THEN 1 ELSE 0 END)
			OR (CASE WHEN a.published > 'now' THEN 1 ELSE 0 END) = (CASE WHEN cur.published > 'now' THEN 1 ELSE 0 END) AND a.published < cur.published
			OR (CASE WHEN a.published > 'now' THEN 1 ELSE 0 END) = (CASE WHEN cur.published > 'now' THEN 1 ELSE 0 END) AND a.published = cur.published AND a.id > ?
		)
		ORDER BY (CASE WHEN a.published > 'now' THEN 1 ELSE 0 END) ASC, a.published DESC, a.id ASC
		LIMIT 1
	`, fromClause, whereClause)

	var args []any
	args = append(args, articleID)
	if feedURL != "" {
		args = append(args, feedURL)
	} else {
		args = append(args, userDID)
	}
	if liked {
		args = append(args, userDID)
	}
	if status == "unread" || status == "read" {
		args = append(args, userDID)
	}
	args = append(args, articleID, articleID)

	var next sql.NullInt64
	if err := s.db.QueryRowContext(ctx, query, args...).Scan(&next); err != nil {
		return nil, nil
	}
	return &next.Int64, nil
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

func (s *ArticleStore) SearchArticles(ctx context.Context, userDID, query string, limit, offset int) ([]*Article, error) {
	if strings.TrimSpace(query) == "" {
		return nil, nil
	}

	safeQuery := escapeFTS5(query)
	if strings.TrimSpace(safeQuery) == "" {
		return nil, nil
	}

	columnQuery := "{title summary} : " + safeQuery

	rows, err := s.db.QueryContext(ctx, `
		SELECT a.id, a.feed_url, COALESCE(f.title, ''), f.favicon_url, a.guid, a.title, a.url, a.author, a.summary, a.content,
			a.published, a.updated, a.fetched_at,
			COALESCE(r.is_read, 0),
			COALESCE(lc.cnt, 0),
			COALESCE(ul.liked, 0)
		FROM (
			SELECT rowid, rank FROM articles.articles_fts WHERE articles_fts MATCH ?
		) ft
		JOIN articles.articles a ON a.id = ft.rowid
		JOIN articles.subscriptions s ON a.feed_url = s.feed_url AND s.user_did = ?
		LEFT JOIN articles.feeds f ON a.feed_url = f.feed_url
		LEFT JOIN articles.read_state r ON r.user_did = ? AND r.article_id = a.id
		LEFT JOIN (SELECT feed_url, article_url, COUNT(*) as cnt FROM articles.likes GROUP BY feed_url, article_url) lc
			ON lc.feed_url = a.feed_url AND lc.article_url = a.url
		LEFT JOIN (SELECT feed_url, article_url, 1 as liked FROM articles.likes WHERE author_did = ?) ul
			ON ul.feed_url = a.feed_url AND ul.article_url = a.url
		ORDER BY ft.rank
		LIMIT ? OFFSET ?
	`, columnQuery, userDID, userDID, userDID, limit, offset)
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
