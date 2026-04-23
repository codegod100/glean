package db

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

var ErrDuplicateLike = errors.New("already liked this article")

type Annotation struct {
	ID           int64
	URI          string
	AuthorDID    string
	AuthorHandle string
	FeedURL      string
	ArticleURL   string
	ArticleID    sql.NullInt64
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

func (s *ArticleStore) CreateAnnotation(ctx context.Context, a *Annotation) error {
	return s.BatchCreateAnnotations(ctx, []*Annotation{a})
}

func (s *ArticleStore) GetAnnotation(ctx context.Context, id int64) (*Annotation, error) {
	a := &Annotation{}
	err := s.db.QueryRowContext(ctx, `
		SELECT a.id, a.uri, a.author_did, a.feed_url, a.article_url, ar.id, a.quote, a.note, a.tags, a.rating, a.created_at, a.cid
		FROM articles.annotations a
		LEFT JOIN articles.articles ar ON ar.url = a.article_url AND ar.feed_url = a.feed_url
		WHERE a.id = ?
	`, id).Scan(&a.ID, &a.URI, &a.AuthorDID, &a.FeedURL, &a.ArticleURL, &a.ArticleID,
		&a.Quote, &a.Note, &a.Tags, &a.Rating, &a.CreatedAt, &a.CID)
	if err != nil {
		return nil, err
	}
	return a, nil
}

func (s *ArticleStore) DeleteAnnotation(ctx context.Context, uri string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM articles.annotations WHERE uri = ?`, uri)
	return err
}

func (s *ArticleStore) AnnotationExists(ctx context.Context, uri string) (bool, error) {
	var exists int
	err := s.db.QueryRowContext(ctx, `SELECT 1 FROM articles.annotations WHERE uri = ?`, uri).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (s *ArticleStore) ListAnnotations(ctx context.Context, feedURL, articleURL, authorDID string, limit, offset int) ([]*Annotation, error) {
	var conds []string
	var args []any

	if feedURL != "" {
		conds = append(conds, "a.feed_url = ?")
		args = append(args, feedURL)
	}
	if articleURL != "" {
		conds = append(conds, "a.article_url = ?")
		args = append(args, articleURL)
	}
	if authorDID != "" {
		conds = append(conds, "a.author_did = ?")
		args = append(args, authorDID)
	}

	query := `SELECT a.id, a.uri, a.author_did, a.feed_url, a.article_url, ar.id, a.quote, a.note, a.tags, a.rating, a.created_at, a.cid
		FROM articles.annotations a
		LEFT JOIN articles.articles ar ON ar.url = a.article_url AND ar.feed_url = a.feed_url`
	if len(conds) > 0 {
		query += ` WHERE ` + strings.Join(conds, " AND ")
	}
	query += ` ORDER BY a.created_at DESC LIMIT ? OFFSET ?`
	args = append(args, limit, offset)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var annotations []*Annotation
	for rows.Next() {
		a := &Annotation{}
		if err := rows.Scan(&a.ID, &a.URI, &a.AuthorDID, &a.FeedURL, &a.ArticleURL, &a.ArticleID,
			&a.Quote, &a.Note, &a.Tags, &a.Rating, &a.CreatedAt, &a.CID); err != nil {
			return nil, err
		}
		annotations = append(annotations, a)
	}
	return annotations, rows.Err()
}

func (s *ArticleStore) BatchCreateLikes(ctx context.Context, likes []*Like) error {
	if len(likes) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT OR IGNORE INTO articles.likes (uri, author_did, feed_url, article_url, created_at, cid)
		VALUES (?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, l := range likes {
		if _, err := stmt.ExecContext(ctx, l.URI, l.AuthorDID, l.FeedURL, l.ArticleURL, l.CreatedAt, l.CID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *ArticleStore) BatchCreateAnnotations(ctx context.Context, annotations []*Annotation) error {
	if len(annotations) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT OR IGNORE INTO articles.annotations (uri, author_did, feed_url, article_url, quote, note, tags, rating, created_at, cid)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, a := range annotations {
		if _, err := stmt.ExecContext(ctx, a.URI, a.AuthorDID, a.FeedURL, a.ArticleURL, a.Quote, a.Note, a.Tags, a.Rating, a.CreatedAt, a.CID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *ArticleStore) CreateLike(ctx context.Context, l *Like) error {
	exists, err := s.HasLiked(ctx, l.AuthorDID, l.FeedURL, l.ArticleURL)
	if err != nil {
		return err
	}
	if exists {
		return ErrDuplicateLike
	}
	return s.BatchCreateLikes(ctx, []*Like{l})
}

func (s *ArticleStore) DeleteLike(ctx context.Context, uri string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM articles.likes WHERE uri = ?`, uri)
	return err
}

func (s *ArticleStore) DeleteLikeByUserArticle(ctx context.Context, authorDID, feedURL, articleURL string) error {
	_, err := s.db.ExecContext(ctx, `
		DELETE FROM articles.likes WHERE author_did = ? AND feed_url = ? AND article_url = ?
	`, authorDID, feedURL, articleURL)
	return err
}

func (s *ArticleStore) ListLikes(ctx context.Context, authorDID, feedURL string, limit, offset int) ([]*Like, error) {
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

	query := `SELECT id, uri, author_did, feed_url, article_url, created_at, cid FROM articles.likes`
	if len(conds) > 0 {
		query += ` WHERE ` + strings.Join(conds, " AND ")
	}
	query += ` ORDER BY created_at DESC LIMIT ? OFFSET ?`
	args = append(args, limit, offset)

	rows, err := s.db.QueryContext(ctx, query, args...)
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

func (s *ArticleStore) GetLikeCount(ctx context.Context, feedURL, articleURL string) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM articles.likes WHERE feed_url = ? AND article_url = ?
	`, feedURL, articleURL).Scan(&count)
	return count, err
}

func (s *ArticleStore) GetLike(ctx context.Context, authorDID, feedURL, articleURL string) (*Like, error) {
	l := &Like{}
	err := s.db.QueryRowContext(ctx, `
		SELECT * FROM articles.likes
		WHERE author_did = ? AND feed_url = ? AND article_url = ?
	`, authorDID, feedURL, articleURL).Scan(&l.ID, &l.URI, &l.AuthorDID, &l.FeedURL, &l.ArticleURL, &l.CreatedAt, &l.CID)
	if err != nil {
		return nil, err
	}
	return l, nil
}

func (s *ArticleStore) HasLiked(ctx context.Context, authorDID, feedURL, articleURL string) (bool, error) {
	var exists int
	err := s.db.QueryRowContext(ctx, `
		SELECT 1 FROM articles.likes WHERE author_did = ? AND feed_url = ? AND article_url = ?
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
	HasLiked        bool
}

func (s *ArticleStore) ListTrendingArticlesForUser(ctx context.Context, userDID, since string, limit, offset int) ([]*TrendingItem, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT ar.id, ar.title, COALESCE(ar.url, ''), COALESCE(ar.author, ''),
		       COALESCE(ar.summary, ''), l.feed_url, COALESCE(f.title, ''),
		       COALESCE(f.favicon_url, ''),
		       COUNT(DISTINCT l.id) AS like_count,
		       COUNT(DISTINCT a.id) AS annotation_count,
		       COALESCE(MAX(CASE WHEN ul.id IS NOT NULL THEN 1 ELSE 0 END), 0)
		FROM articles.likes l
		JOIN articles.articles ar ON ar.url = l.article_url AND ar.feed_url = l.feed_url
		LEFT JOIN articles.feeds f ON f.feed_url = l.feed_url
		LEFT JOIN articles.annotations a ON a.feed_url = l.feed_url AND a.article_url = l.article_url AND a.created_at >= ?
		LEFT JOIN articles.likes ul ON ul.feed_url = l.feed_url AND ul.article_url = l.article_url AND ul.author_did = ?
		WHERE l.created_at >= ?
		  AND l.author_did IN (
		    SELECT CASE WHEN us.user_a = ? THEN us.user_b ELSE us.user_a END
		    FROM user_similarity us
		    WHERE us.user_a = ? OR us.user_b = ?
		    UNION SELECT ?
		    UNION SELECT f.target_did FROM follows f WHERE f.user_did = ?
		  )
		GROUP BY ar.id
		-- Future-published articles (e.g., scheduled) sort last
		ORDER BY like_count DESC, annotation_count DESC, (CASE WHEN ar.published > 'now' THEN 1 ELSE 0 END), ar.published DESC
		LIMIT ? OFFSET ?
	`, since, userDID, since, userDID, userDID, userDID, userDID, userDID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*TrendingItem
	for rows.Next() {
		item := &TrendingItem{}
		if err := rows.Scan(&item.ArticleID, &item.Title, &item.URL, &item.Author,
			&item.Summary, &item.FeedURL, &item.FeedTitle, &item.FaviconURL,
			&item.LikeCount, &item.AnnotationCount, &item.HasLiked); err != nil {
			return nil, err
		}
		results = append(results, item)
	}
	return results, rows.Err()
}

func (s *ArticleStore) ListTrendingArticles(ctx context.Context, userDID, since string, limit, offset int) ([]*TrendingItem, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT ar.id, ar.title, COALESCE(ar.url, ''), COALESCE(ar.author, ''),
		       COALESCE(ar.summary, ''), l.feed_url, COALESCE(f.title, ''),
		       COALESCE(f.favicon_url, ''),
		       COUNT(DISTINCT l.id) AS like_count,
		       COUNT(DISTINCT a.id) AS annotation_count,
		       COALESCE(MAX(CASE WHEN ul.id IS NOT NULL THEN 1 ELSE 0 END), 0)
		FROM articles.likes l
		JOIN articles.articles ar ON ar.url = l.article_url AND ar.feed_url = l.feed_url
		LEFT JOIN articles.feeds f ON f.feed_url = l.feed_url
		LEFT JOIN articles.annotations a ON a.feed_url = l.feed_url AND a.article_url = l.article_url AND a.created_at >= ?
		LEFT JOIN articles.likes ul ON ul.feed_url = l.feed_url AND ul.article_url = l.article_url AND ul.author_did = ?
		WHERE l.created_at >= ?
		GROUP BY ar.id
		-- Future-published articles (e.g., scheduled) sort last
		ORDER BY like_count DESC, annotation_count DESC, (CASE WHEN ar.published > 'now' THEN 1 ELSE 0 END), ar.published DESC
		LIMIT ? OFFSET ?
	`, since, userDID, since, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*TrendingItem
	for rows.Next() {
		item := &TrendingItem{}
		if err := rows.Scan(&item.ArticleID, &item.Title, &item.URL, &item.Author,
			&item.Summary, &item.FeedURL, &item.FeedTitle, &item.FaviconURL,
			&item.LikeCount, &item.AnnotationCount, &item.HasLiked); err != nil {
			return nil, err
		}
		results = append(results, item)
	}
	return results, rows.Err()
}

func (s *ArticleStore) ListLikedArticles(ctx context.Context, userDID string, limit, offset int) ([]*Article, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT DISTINCT a.id, a.feed_url, a.guid, a.title, a.url, a.author, a.summary, a.content,
			a.published, a.updated, a.fetched_at,
			COALESCE(f.title, ''),
			COALESCE(r.is_read, 0),
			COALESCE(lc.cnt, 0),
			1
		FROM articles.likes l
		JOIN articles.articles a ON a.url = l.article_url AND a.feed_url = l.feed_url
		LEFT JOIN articles.feeds f ON f.feed_url = a.feed_url
		LEFT JOIN read_state r ON r.user_did = ? AND r.article_id = a.id
		LEFT JOIN (SELECT feed_url, article_url, COUNT(*) as cnt FROM articles.likes GROUP BY feed_url, article_url) lc
			ON lc.feed_url = a.feed_url AND lc.article_url = a.url
		WHERE l.author_did = ?
		ORDER BY l.created_at DESC
		LIMIT ? OFFSET ?
	`, userDID, userDID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var articles []*Article
	for rows.Next() {
		a := &Article{}
		if err := rows.Scan(&a.ID, &a.FeedURL, &a.GUID, &a.Title, &a.URL, &a.Author,
			&a.Summary, &a.Content, &a.Published, &a.Updated, &a.FetchedAt,
			&a.FeedTitle, &a.IsRead, &a.LikeCount, &a.HasLiked); err != nil {
			return nil, err
		}
		articles = append(articles, a)
	}
	return articles, rows.Err()
}
