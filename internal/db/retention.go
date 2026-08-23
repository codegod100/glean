package db

import (
	"context"
	"fmt"
	"time"
)

// Retention keeps the database from growing without bound. Glean used to ingest
// network-wide ATProto activity (jetstream) and keep full article bodies
// forever; once SQLite cannot write, everything that persists state breaks —
// including OAuth sign-in.

const purgeBatchSize = 5000

// PurgeStats reports the rows removed per table by a purge pass. Tables with
// nothing to purge are omitted.
type PurgeStats map[string]int64

// Total returns the number of rows removed across all tables.
func (p PurgeStats) Total() int64 {
	var total int64
	for _, n := range p {
		total += n
	}
	return total
}

// PurgeUnknownUserRows deletes likes, follows, subscriptions, annotations,
// read state, impressions and dismissals whose owner DID is not in the users
// table. Jetstream used to deliver events for the whole network; only rows
// owned by known users are meaningful for feeds and recommendations.
//
// Deletes run in batches so the WAL cannot grow by the full table size on a
// nearly-full volume.
func (s *Store) PurgeUnknownUserRows(ctx context.Context) (PurgeStats, error) {
	stats := PurgeStats{}
	// Follows first: that table is the one that filled the users DB.
	queries := []struct {
		name  string
		query string
	}{
		{"follows", `DELETE FROM follows WHERE rowid IN (SELECT rowid FROM follows WHERE user_did NOT IN (SELECT did FROM users) LIMIT ?)`},
		{"likes", `DELETE FROM articles.likes WHERE rowid IN (SELECT rowid FROM articles.likes WHERE author_did NOT IN (SELECT did FROM users) LIMIT ?)`},
		{"annotations", `DELETE FROM articles.annotations WHERE rowid IN (SELECT rowid FROM articles.annotations WHERE author_did NOT IN (SELECT did FROM users) LIMIT ?)`},
		{"subscriptions", `DELETE FROM articles.subscriptions WHERE rowid IN (SELECT rowid FROM articles.subscriptions WHERE user_did NOT IN (SELECT did FROM users) LIMIT ?)`},
		{"read_state", `DELETE FROM articles.read_state WHERE rowid IN (SELECT rowid FROM articles.read_state WHERE user_did NOT IN (SELECT did FROM users) LIMIT ?)`},
		{"recommendation_impressions", `DELETE FROM recommendation_impressions WHERE rowid IN (SELECT rowid FROM recommendation_impressions WHERE user_did NOT IN (SELECT did FROM users) LIMIT ?)`},
		{"dismissed_recommendations", `DELETE FROM dismissed_recommendations WHERE rowid IN (SELECT rowid FROM dismissed_recommendations WHERE user_did NOT IN (SELECT did FROM users) LIMIT ?)`},
	}
	for _, q := range queries {
		n, err := s.deleteInBatches(ctx, q.query)
		if err != nil {
			return stats, fmt.Errorf("purge %s: %w", q.name, err)
		}
		if n > 0 {
			stats[q.name] = n
		}
	}
	return stats, nil
}

func (s *Store) deleteInBatches(ctx context.Context, query string) (int64, error) {
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return total, err
		}
		res, err := s.db.ExecContext(ctx, query, purgeBatchSize)
		if err != nil {
			return total, err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return total, err
		}
		total += n
		if n < int64(purgeBatchSize) {
			return total, nil
		}
	}
}

// PurgeExpiredArticles deletes articles older than maxAgeDays — by published
// date, falling back to fetched_at for undated items — together with their
// read-state rows. It returns the number of articles removed.
func (s *Store) PurgeExpiredArticles(ctx context.Context, maxAgeDays int) (int64, error) {
	if maxAgeDays <= 0 {
		return 0, nil
	}
	cutoff := time.Now().AddDate(0, 0, -maxAgeDays)
	const stale = `(published IS NOT NULL AND published < ?1) OR (published IS NULL AND fetched_at < ?1)`

	if _, err := s.db.ExecContext(ctx,
		`DELETE FROM articles.read_state WHERE article_id IN (SELECT id FROM articles.articles WHERE `+stale+`)`,
		cutoff,
	); err != nil {
		return 0, fmt.Errorf("purge read state of old articles: %w", err)
	}

	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return total, err
		}
		res, err := s.db.ExecContext(ctx,
			`DELETE FROM articles.articles WHERE id IN (SELECT id FROM articles.articles WHERE `+stale+` LIMIT ?2)`,
			cutoff, purgeBatchSize,
		)
		if err != nil {
			return total, fmt.Errorf("purge old articles: %w", err)
		}
		n, _ := res.RowsAffected()
		total += n
		if n < int64(purgeBatchSize) {
			return total, nil
		}
	}
}

// ReclaimSpace shrinks the database files after large purges. VACUUM is
// pointed at the container's ephemeral /tmp so it can complete even when the
// data volume itself is full; incremental vacuum is a fallback if VACUUM
// cannot run.
func (s *Store) ReclaimSpace(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `PRAGMA temp_store_directory = '/tmp'`); err != nil {
		return fmt.Errorf("temp_store_directory: %w", err)
	}
	var first error
	for _, schema := range []string{"main", "articles", "recs"} {
		if _, err := s.db.ExecContext(ctx, fmt.Sprintf(`PRAGMA %s.wal_checkpoint(TRUNCATE)`, schema)); err != nil && first == nil {
			first = fmt.Errorf("wal_checkpoint %s: %w", schema, err)
		}
		if _, err := s.db.ExecContext(ctx, fmt.Sprintf(`VACUUM %s`, schema)); err != nil {
			if first == nil {
				first = fmt.Errorf("vacuum %s: %w", schema, err)
			}
			if _, err := s.db.ExecContext(ctx, fmt.Sprintf(`PRAGMA %s.incremental_vacuum`, schema)); err != nil && first == nil {
				first = fmt.Errorf("incremental_vacuum %s: %w", schema, err)
			}
		}
	}
	return first
}
