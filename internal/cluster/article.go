package cluster

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"

	vec "github.com/asg017/sqlite-vec-go-bindings/cgo"
)

// embedBatchSize caps how many texts are sent in a single embedding API call.
// Most /v1/embeddings endpoints accept up to ~2048 inputs per request; lower
// values reduce payload size and memory pressure.
const embedBatchSize = 100

// maxEmbedChars truncates text sent to the embedding model. Most models have
// a token context window (~4 chars/token); title+summary+content stays well
// under this, but the cap is kept as a safety net.
const maxEmbedChars = 8000

func truncateForEmbed(s string) string {
	if len(s) <= maxEmbedChars {
		return s
	}
	return s[:maxEmbedChars]
}

// ComputeArticleEmbeddings embeds new articles (title + summary + content) into
// the article_embeddings vec0 table. full_content (scraped body) is excluded
// because embedding models have token context limits and the feed-provided
// content already captures the topical signal needed for recommendation KNN.

func (e *Engine) ComputeArticleEmbeddings(ctx context.Context) error {
	if e.embedder == nil {
		e.logger.Debug("article embeddings skipped, no embedder")
		return nil
	}

	conn, err := e.db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()

	_, err = conn.ExecContext(ctx, `
		DELETE FROM recs.article_embeddings
		WHERE article_id NOT IN (SELECT id FROM articles.articles)
	`)
	if err != nil {
		return fmt.Errorf("clean stale article embeddings: %w", err)
	}

	totalComputed := 0
	for {
		rows, err := conn.QueryContext(ctx, `
			SELECT a.id, COALESCE(a.title, '') || ' ' || COALESCE(a.summary, '') || ' ' || COALESCE(a.content, '')
			FROM articles.articles a
			WHERE (COALESCE(a.title, '') != '' OR COALESCE(a.summary, '') != '' OR COALESCE(a.content, '') != '')
			AND a.id NOT IN (SELECT article_id FROM recs.article_embeddings)
			ORDER BY a.id
			LIMIT ?
		`, embedBatchSize)
		if err != nil {
			return err
		}

		type article struct {
			id   int64
			text string
		}
		var batch []article
		for rows.Next() {
			var a article
			if err := rows.Scan(&a.id, &a.text); err != nil {
				rows.Close()
				return err
			}
			a.text = truncateForEmbed(a.text)
			batch = append(batch, a)
		}
		rows.Close()

		if len(batch) == 0 {
			break
		}

		texts := make([]string, len(batch))
		for j, a := range batch {
			texts[j] = a.text
		}

		embeddings, err := e.embedder.Embed(ctx, texts, "Represent this news article for retrieving topically similar articles. Focus on the subjects, themes, and key entities discussed.")
		if err != nil {
			return fmt.Errorf("embed batch starting at total %d: %w", totalComputed, err)
		}

		tx, err := conn.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer func() { _ = tx.Rollback() }()

		for j, emb := range embeddings {
			blob, err := vec.SerializeFloat32(emb)
			if err != nil {
				return fmt.Errorf("serialize embedding: %w", err)
			}
			if _, err := tx.ExecContext(ctx,
				`INSERT OR IGNORE INTO recs.article_embeddings(article_id, embedding) VALUES (?, ?)`,
				batch[j].id, blob,
			); err != nil {
				return fmt.Errorf("insert embedding: %w", err)
			}
		}

		if err := tx.Commit(); err != nil {
			return err
		}

		totalComputed += len(batch)
		e.logger.Info("article embeddings batch computed",
			slog.Int("batch_total", totalComputed),
			slog.Int("count", len(batch)),
		)
	}

	if totalComputed == 0 {
		e.logger.Info("article embeddings up to date")
	} else {
		e.logger.Info("article embeddings computed", slog.Int("total", totalComputed))
	}
	return nil
}

func (e *Engine) populateContentBoost(ctx context.Context, conn *sql.Conn, userDID string) error {
	rows, err := conn.QueryContext(ctx, `
		SELECT a.id FROM articles.likes ul
		JOIN articles.articles a ON a.feed_url = ul.feed_url AND a.url = ul.article_url
		WHERE ul.author_did = ?
	`, userDID)
	if err != nil {
		return err
	}
	var articleIDs []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		articleIDs = append(articleIDs, id)
	}
	rows.Close()

	if len(articleIDs) == 0 {
		return nil
	}

	ph := make([]string, len(articleIDs))
	args := make([]any, len(articleIDs))
	for i, id := range articleIDs {
		ph[i] = "?"
		args[i] = id
	}
	embRows, err := conn.QueryContext(ctx,
		fmt.Sprintf("SELECT article_id, embedding FROM recs.article_embeddings WHERE article_id IN (%s)", joinPh(ph)),
		args...,
	)
	if err != nil {
		return err
	}

	dim := e.embedder.Dimension()
	var blobs [][]byte
	likedSet := make(map[int64]bool)
	for embRows.Next() {
		var id int64
		var blob []byte
		if err := embRows.Scan(&id, &blob); err != nil {
			embRows.Close()
			return err
		}
		if len(blob) != dim*4 {
			continue
		}
		blobs = append(blobs, blob)
		likedSet[id] = true
	}
	embRows.Close()

	if len(blobs) == 0 {
		return nil
	}

	queryBlob, err := avgEmbeddings(blobs, dim)
	if err != nil {
		return fmt.Errorf("serialize query vector: %w", err)
	}

	const topK = 200
	knnRows, err := conn.QueryContext(ctx, `
		SELECT article_id, distance FROM recs.article_embeddings
		WHERE embedding MATCH ? AND k = ?
		ORDER BY distance
	`, queryBlob, topK+len(likedSet))
	if err != nil {
		return err
	}

	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	for knnRows.Next() {
		var id int64
		var dist float64
		if err := knnRows.Scan(&id, &dist); err != nil {
			knnRows.Close()
			return err
		}
		if likedSet[id] {
			continue
		}
		score := 1.0 - dist
		if score <= 0 {
			continue
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT OR IGNORE INTO _content_boost (article_id, score) VALUES (?, ?)`,
			id, score,
		); err != nil {
			return err
		}
	}
	knnRows.Close()

	return tx.Commit()
}

// ComputeFeedEmbeddings embeds feed descriptions (title + description) into the
// feed_embeddings vec0 table and tracks source text in feed_embedding_meta for
// re-embedding on description change. Skipped when no embedder is configured.
func (e *Engine) ComputeFeedEmbeddings(ctx context.Context) error {
	if e.embedder == nil {
		e.logger.Debug("feed embeddings skipped, no embedder")
		return nil
	}

	conn, err := e.db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()

	_, err = conn.ExecContext(ctx, `
		DELETE FROM recs.feed_embeddings
		WHERE feed_url NOT IN (SELECT feed_url FROM articles.feeds)
	`)
	if err != nil {
		return fmt.Errorf("clean stale feed embeddings: %w", err)
	}
	_, err = conn.ExecContext(ctx, `
		DELETE FROM recs.feed_embedding_meta
		WHERE feed_url NOT IN (SELECT feed_url FROM articles.feeds)
	`)
	if err != nil {
		return fmt.Errorf("clean stale feed embeddings: %w", err)
	}

	totalComputed := 0
	for {
		rows, err := conn.QueryContext(ctx, `
			SELECT f.feed_url, COALESCE(f.title, '') || ' ' || COALESCE(f.description, '')
			FROM articles.feeds f
			WHERE (COALESCE(f.title, '') != '' OR COALESCE(f.description, '') != '')
			AND (
				f.feed_url NOT IN (SELECT feed_url FROM recs.feed_embedding_meta)
				OR EXISTS (
					SELECT 1 FROM recs.feed_embedding_meta fm
					WHERE fm.feed_url = f.feed_url
					AND fm.source_text != COALESCE(f.title, '') || ' ' || COALESCE(f.description, '')
				)
			)
			ORDER BY f.feed_url
			LIMIT ?
		`, embedBatchSize)
		if err != nil {
			return err
		}

		type feed struct {
			url  string
			text string
		}
		var batch []feed
		for rows.Next() {
			var f feed
			if err := rows.Scan(&f.url, &f.text); err != nil {
				rows.Close()
				return err
			}
			batch = append(batch, f)
		}
		rows.Close()

		if len(batch) == 0 {
			break
		}

		texts := make([]string, len(batch))
		for j, f := range batch {
			texts[j] = f.text
		}

		embeddings, err := e.embedder.Embed(ctx, texts, "Represent this RSS feed description for discovering feeds with similar editorial focus and topic coverage.")
		if err != nil {
			return fmt.Errorf("embed feed batch starting at total %d: %w", totalComputed, err)
		}

		tx, err := conn.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer func() { _ = tx.Rollback() }()

		for j, emb := range embeddings {
			blob, err := vec.SerializeFloat32(emb)
			if err != nil {
				return fmt.Errorf("serialize feed embedding: %w", err)
			}
			if _, err := tx.ExecContext(ctx,
				`DELETE FROM recs.feed_embeddings WHERE feed_url = ?`, batch[j].url,
			); err != nil {
				return fmt.Errorf("delete feed embedding: %w", err)
			}
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO recs.feed_embeddings(feed_url, embedding) VALUES (?, ?)`,
				batch[j].url, blob,
			); err != nil {
				return fmt.Errorf("insert feed embedding: %w", err)
			}
			if _, err := tx.ExecContext(ctx,
				`INSERT OR REPLACE INTO recs.feed_embedding_meta(feed_url, source_text) VALUES (?, ?)`,
				batch[j].url, batch[j].text,
			); err != nil {
				return fmt.Errorf("insert feed embedding: %w", err)
			}
		}

		if err := tx.Commit(); err != nil {
			return err
		}

		totalComputed += len(batch)
		e.logger.Info("feed embeddings batch computed",
			slog.Int("batch_total", totalComputed),
			slog.Int("count", len(batch)),
		)
	}

	if totalComputed == 0 {
		e.logger.Info("feed embeddings up to date")
	} else {
		e.logger.Info("feed embeddings computed", slog.Int("total", totalComputed))
	}
	return nil
}

// DetectArticleLanguages detects the language of articles that still have the
// default language ('en') using the embedding model. It processes articles in
// batches alongside the embedding computation to reuse the same API client.
func (e *Engine) DetectArticleLanguages(ctx context.Context) error {
	if e.llm == nil {
		e.logger.Debug("language detection skipped, no LLM client")
		return nil
	}

	conn, err := e.db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()

	rows, err := conn.QueryContext(ctx, `
		SELECT id, COALESCE(title, '') || ' ' || COALESCE(summary, '')
		FROM articles.articles
		WHERE language = ''
		ORDER BY id DESC
		LIMIT 5000
	`)
	if err != nil {
		return err
	}

	type article struct {
		id   int64
		text string
	}
	var batch []article
	for rows.Next() {
		var a article
		if err := rows.Scan(&a.id, &a.text); err != nil {
			rows.Close()
			return err
		}
		if strings.TrimSpace(a.text) == "" {
			continue
		}
		if len(a.text) > 500 {
			a.text = a.text[:500]
		}
		batch = append(batch, a)
	}
	rows.Close()

	if len(batch) == 0 {
		e.logger.Info("article languages up to date")
		return nil
	}

	for i := 0; i < len(batch); i += embedBatchSize {
		end := min(i+embedBatchSize, len(batch))
		sub := batch[i:end]

		texts := make([]string, len(sub))
		for j, a := range sub {
			texts[j] = a.text
		}

		langs, err := e.llm.DetectLanguages(ctx, texts)
		if err != nil {
			return fmt.Errorf("detect languages batch %d: %w", i/embedBatchSize, err)
		}

		tx, err := conn.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer func() { _ = tx.Rollback() }()

		stmt, err := tx.PrepareContext(ctx, `UPDATE articles.articles SET language = ? WHERE id = ? AND language = ''`)
		if err != nil {
			return err
		}
		defer stmt.Close()

		updated := 0
		for j, lang := range langs {
			if lang == "" {
				continue
			}
			res, err := stmt.ExecContext(ctx, lang, sub[j].id)
			if err != nil {
				return fmt.Errorf("update language: %w", err)
			}
			n, _ := res.RowsAffected()
			updated += int(n)
		}

		if err := tx.Commit(); err != nil {
			return err
		}

		e.logger.Info("article languages detected",
			slog.Int("batch", i/embedBatchSize),
			slog.Int("count", len(sub)),
			slog.Int("updated", updated),
		)
	}

	e.logger.Info("article languages computed", slog.Int("total", len(batch)))
	return nil
}

func (e *Engine) ensureContentBoostTable(ctx context.Context, conn *sql.Conn) error {
	_, err := conn.ExecContext(ctx, `CREATE TEMP TABLE IF NOT EXISTS _content_boost (article_id INT PRIMARY KEY, score REAL)`)
	if err != nil {
		return err
	}
	_, err = conn.ExecContext(ctx, `DELETE FROM _content_boost`)
	return err
}

func joinPh(ph []string) string {
	var s strings.Builder
	for i, p := range ph {
		if i > 0 {
			s.WriteString(",")
		}
		s.WriteString(p)
	}
	return s.String()
}
