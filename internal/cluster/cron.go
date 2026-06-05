package cluster

import (
	"context"
	"log/slog"
	"time"

	"pkg.rbrt.fr/glean/internal/db"
	"pkg.rbrt.fr/glean/internal/metrics"
)

// Cron periodically runs all cluster engine computations (similarity, embeddings,
// follow distances, signal profiles, auto-dismiss, recommendation precomputation)
// on a fixed interval.
type Cron struct {
	engine   *Engine
	interval time.Duration
	logger   *slog.Logger
	dbs      *db.Store
}

// NewCron creates a new cron runner with the given engine and interval.
func NewCron(engine *Engine, interval time.Duration, logger *slog.Logger, dbs *db.Store) *Cron {
	return &Cron{
		engine:   engine,
		interval: interval,
		logger:   logger,
		dbs:      dbs,
	}
}

// Run starts the cron loop. It blocks until ctx is cancelled. Each tick runs
// all computations sequentially; if a previous run is still in progress the
// tick is skipped.
func (c *Cron) Run(ctx context.Context) error {
	for {
		c.logger.Info("starting similarity computation")
		start := time.Now()

		if !c.engine.mu.TryLock() {
			c.logger.Info("skipping computation: already in progress")
		} else {
			if err := c.engine.ComputeFeedEmbeddings(ctx); err != nil {
				c.engine.logger.Error("feed embeddings failed", "error", err)
			}
			if err := c.engine.ComputeFeedSimilarity(ctx); err != nil {
				c.engine.logger.Error("feed similarity failed", "error", err)
			}
			if err := c.engine.ComputeUserSimilarity(ctx); err != nil {
				c.engine.logger.Error("user similarity failed", "error", err)
			}
			if err := c.engine.ComputeArticleEmbeddings(ctx); err != nil {
				c.engine.logger.Error("article embeddings failed", "error", err)
			}
			if err := c.engine.DetectArticleLanguages(ctx); err != nil {
				c.engine.logger.Error("language detection failed", "error", err)
			}
			if err := c.engine.ComputeSignalProfiles(ctx); err != nil {
				c.engine.logger.Error("signal profiles failed", "error", err)
			}
			if err := c.engine.ComputeFollowDistances(ctx); err != nil {
				c.logger.Error("follow distances failed", "error", err)
			}
			if err := c.engine.feedback.AutoDismissStale(ctx, 5, 5); err != nil {
				c.engine.logger.Error("auto dismiss failed", "error", err)
			}
			if err := c.engine.PrecomputeAllRecommendations(ctx); err != nil {
				c.engine.logger.Error("recommendation precomputation failed", "error", err)
			}

			c.engine.mu.Unlock()
		}

		if err := c.dbs.RunMaintenance(ctx, 90); err != nil {
			c.logger.Error("db maintenance failed", "error", err)
		}

		metrics.ClusterRuns.Inc()
		metrics.ClusterDuration.Observe(time.Since(start).Seconds())
		c.logger.Info("similarity computation complete", "next_run", c.interval)

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(c.interval):
		}
	}
}
