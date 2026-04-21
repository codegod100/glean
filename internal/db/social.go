package db

import (
	"context"
	"database/sql"
	"strings"
)

type Annotation struct {
	ID           int64
	URI          string
	AuthorDID    string
	AuthorHandle string
	FeedURL      string
	ArticleURL   string
	Quote        sql.NullString
	Note         sql.NullString
	Tags         sql.NullString
	Rating       sql.NullInt64
	CreatedAt    sql.NullTime
	CID          sql.NullString
}

type Like struct {
	ID         int64
	URI        string
	AuthorDID  string
	FeedURL    string
	ArticleURL string
	CreatedAt  sql.NullTime
	CID        sql.NullString
}

func (db *DB) CreateAnnotation(ctx context.Context, a *Annotation) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO annotations (uri, author_did, feed_url, article_url, quote, note, tags, rating, created_at, cid)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, a.URI, a.AuthorDID, a.FeedURL, a.ArticleURL, a.Quote, a.Note, a.Tags, a.Rating, a.CreatedAt, a.CID)
	return err
}

func (db *DB) GetAnnotation(ctx context.Context, id int64) (*Annotation, error) {
	a := &Annotation{}
	err := db.QueryRowContext(ctx, `
		SELECT a.id, a.uri, a.author_did, COALESCE(u.handle, ''), a.feed_url, a.article_url, a.quote, a.note, a.tags, a.rating, a.created_at, a.cid
		FROM annotations a
		LEFT JOIN users u ON a.author_did = u.did
		WHERE a.id = ?
	`, id).Scan(&a.ID, &a.URI, &a.AuthorDID, &a.AuthorHandle, &a.FeedURL, &a.ArticleURL,
		&a.Quote, &a.Note, &a.Tags, &a.Rating, &a.CreatedAt, &a.CID)
	if err != nil {
		return nil, err
	}
	return a, nil
}

func (db *DB) DeleteAnnotation(ctx context.Context, uri string) error {
	_, err := db.ExecContext(ctx, `DELETE FROM annotations WHERE uri = ?`, uri)
	return err
}

func (db *DB) AnnotationExists(ctx context.Context, uri string) (bool, error) {
	var exists int
	err := db.QueryRowContext(ctx, `SELECT 1 FROM annotations WHERE uri = ?`, uri).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (db *DB) ListAnnotations(ctx context.Context, feedURL, articleURL, authorDID string, limit, offset int) ([]*Annotation, error) {
	var conds []string
	var args []any

	if feedURL != "" {
		conds = append(conds, "feed_url = ?")
		args = append(args, feedURL)
	}
	if articleURL != "" {
		conds = append(conds, "article_url = ?")
		args = append(args, articleURL)
	}
	if authorDID != "" {
		conds = append(conds, "author_did = ?")
		args = append(args, authorDID)
	}

	query := `SELECT a.id, a.uri, a.author_did, COALESCE(u.handle, ''), a.feed_url, a.article_url, a.quote, a.note, a.tags, a.rating, a.created_at, a.cid
		FROM annotations a
		LEFT JOIN users u ON a.author_did = u.did`
	if len(conds) > 0 {
		query += ` WHERE ` + strings.Join(conds, " AND ")
	}
	query += ` ORDER BY a.created_at DESC LIMIT ? OFFSET ?`
	args = append(args, limit, offset)

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var annotations []*Annotation
	for rows.Next() {
		a := &Annotation{}
		if err := rows.Scan(&a.ID, &a.URI, &a.AuthorDID, &a.AuthorHandle, &a.FeedURL, &a.ArticleURL,
			&a.Quote, &a.Note, &a.Tags, &a.Rating, &a.CreatedAt, &a.CID); err != nil {
			return nil, err
		}
		annotations = append(annotations, a)
	}
	return annotations, rows.Err()
}

func (db *DB) CreateLike(ctx context.Context, l *Like) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO likes (uri, author_did, feed_url, article_url, created_at, cid)
		VALUES (?, ?, ?, ?, ?, ?)
	`, l.URI, l.AuthorDID, l.FeedURL, l.ArticleURL, l.CreatedAt, l.CID)
	return err
}

func (db *DB) DeleteLike(ctx context.Context, uri string) error {
	_, err := db.ExecContext(ctx, `DELETE FROM likes WHERE uri = ?`, uri)
	return err
}

func (db *DB) DeleteLikeByUserArticle(ctx context.Context, authorDID, feedURL, articleURL string) error {
	_, err := db.ExecContext(ctx, `
		DELETE FROM likes WHERE author_did = ? AND feed_url = ? AND article_url = ?
	`, authorDID, feedURL, articleURL)
	return err
}

func (db *DB) ListLikes(ctx context.Context, authorDID, feedURL string, limit, offset int) ([]*Like, error) {
	var conds []string
	var args []any

	if authorDID != "" {
		conds = append(conds, "author_did = ?")
		args = append(args, authorDID)
	}
	if feedURL != "" {
		conds = append(conds, "feed_url = ?")
		args = append(args, feedURL)
	}

	query := `SELECT id, uri, author_did, feed_url, article_url, created_at, cid FROM likes`
	if len(conds) > 0 {
		query += ` WHERE ` + strings.Join(conds, " AND ")
	}
	query += ` ORDER BY created_at DESC LIMIT ? OFFSET ?`
	args = append(args, limit, offset)

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var likes []*Like
	for rows.Next() {
		l := &Like{}
		if err := rows.Scan(&l.ID, &l.URI, &l.AuthorDID, &l.FeedURL, &l.ArticleURL, &l.CreatedAt, &l.CID); err != nil {
			return nil, err
		}
		likes = append(likes, l)
	}
	return likes, rows.Err()
}

func (db *DB) GetLikeCount(ctx context.Context, feedURL, articleURL string) (int, error) {
	var count int
	err := db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM likes WHERE feed_url = ? AND article_url = ?
	`, feedURL, articleURL).Scan(&count)
	return count, err
}

func (db *DB) GetLike(ctx context.Context, authorDID, feedURL, articleURL string) (*Like, error) {
	l := &Like{}
	err := db.QueryRowContext(ctx, `
		SELECT id, uri, author_did, feed_url, article_url, created_at, cid FROM likes
		WHERE author_did = ? AND feed_url = ? AND article_url = ?
	`, authorDID, feedURL, articleURL).Scan(&l.ID, &l.URI, &l.AuthorDID, &l.FeedURL, &l.ArticleURL, &l.CreatedAt, &l.CID)
	if err != nil {
		return nil, err
	}
	return l, nil
}

func (db *DB) HasLiked(ctx context.Context, authorDID, feedURL, articleURL string) (bool, error) {
	var exists int
	err := db.QueryRowContext(ctx, `
		SELECT 1 FROM likes WHERE author_did = ? AND feed_url = ? AND article_url = ?
	`, authorDID, feedURL, articleURL).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

type TrendingItem struct {
	ArticleID       int64
	Title           string
	URL             string
	Author          string
	Summary         string
	FeedURL         string
	FeedTitle       string
	FaviconURL      string
	LikeCount       int
	AnnotationCount int
}

func (db *DB) ListTrendingArticlesForUser(ctx context.Context, userDID, since string, limit, offset int) ([]*TrendingItem, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT ar.id, ar.title, COALESCE(ar.url, ''), COALESCE(ar.author, ''),
		       COALESCE(ar.summary, ''), l.feed_url, COALESCE(f.title, ''),
		       COALESCE(f.favicon_url, ''),
		       COUNT(DISTINCT l.id) AS like_count,
		       COUNT(DISTINCT a.id) AS annotation_count
		FROM likes l
		JOIN articles ar ON ar.url = l.article_url AND ar.feed_url = l.feed_url
		LEFT JOIN feeds f ON f.feed_url = l.feed_url
		LEFT JOIN annotations a ON a.feed_url = l.feed_url AND a.article_url = l.article_url AND a.created_at >= ?
		WHERE l.created_at >= ?
		  AND l.author_did IN (
		    SELECT CASE WHEN us.user_a = ? THEN us.user_b ELSE us.user_a END
		    FROM user_similarity us
		    WHERE us.user_a = ? OR us.user_b = ?
		    UNION SELECT ?
		    UNION SELECT f.target_did FROM follows f WHERE f.user_did = ?
		  )
		GROUP BY ar.id
		ORDER BY like_count DESC, annotation_count DESC
		LIMIT ? OFFSET ?
	`, since, since, userDID, userDID, userDID, userDID, userDID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*TrendingItem
	for rows.Next() {
		item := &TrendingItem{}
		if err := rows.Scan(&item.ArticleID, &item.Title, &item.URL, &item.Author,
			&item.Summary, &item.FeedURL, &item.FeedTitle, &item.FaviconURL,
			&item.LikeCount, &item.AnnotationCount); err != nil {
			return nil, err
		}
		results = append(results, item)
	}
	return results, rows.Err()
}

func (db *DB) ListTrendingArticles(ctx context.Context, since string, limit, offset int) ([]*TrendingItem, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT ar.id, ar.title, COALESCE(ar.url, ''), COALESCE(ar.author, ''),
		       COALESCE(ar.summary, ''), l.feed_url, COALESCE(f.title, ''),
		       COALESCE(f.favicon_url, ''),
		       COUNT(DISTINCT l.id) AS like_count,
		       COUNT(DISTINCT a.id) AS annotation_count
		FROM likes l
		JOIN articles ar ON ar.url = l.article_url AND ar.feed_url = l.feed_url
		LEFT JOIN feeds f ON f.feed_url = l.feed_url
		LEFT JOIN annotations a ON a.feed_url = l.feed_url AND a.article_url = l.article_url AND a.created_at >= ?
		WHERE l.created_at >= ?
		GROUP BY ar.id
		ORDER BY like_count DESC, annotation_count DESC
		LIMIT ? OFFSET ?
	`, since, since, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*TrendingItem
	for rows.Next() {
		item := &TrendingItem{}
		if err := rows.Scan(&item.ArticleID, &item.Title, &item.URL, &item.Author,
			&item.Summary, &item.FeedURL, &item.FeedTitle, &item.FaviconURL,
			&item.LikeCount, &item.AnnotationCount); err != nil {
			return nil, err
		}
		results = append(results, item)
	}
	return results, rows.Err()
}

func (db *DB) ListLikedArticles(ctx context.Context, userDID string, limit, offset int) ([]*Article, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT DISTINCT a.id, a.feed_url, a.guid, a.title, a.url, a.author, a.summary, a.content,
			a.published, a.updated, a.fetched_at,
			COALESCE(f.title, '')
		FROM likes l
		JOIN articles a ON a.url = l.article_url AND a.feed_url = l.feed_url
		LEFT JOIN feeds f ON f.feed_url = a.feed_url
		WHERE l.author_did = ?
		ORDER BY l.created_at DESC
		LIMIT ? OFFSET ?
	`, userDID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var articles []*Article
	for rows.Next() {
		a := &Article{}
		if err := rows.Scan(&a.ID, &a.FeedURL, &a.GUID, &a.Title, &a.URL, &a.Author,
			&a.Summary, &a.Content, &a.Published, &a.Updated, &a.FetchedAt,
			&a.FeedTitle); err != nil {
			return nil, err
		}
		articles = append(articles, a)
	}
	return articles, rows.Err()
}
