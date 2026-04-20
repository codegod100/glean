package cluster

import (
	"context"
	"log/slog"
	"time"
)

type Cron struct {
	engine   *Engine
	interval time.Duration
	logger   *slog.Logger
}

func NewCron(engine *Engine, interval time.Duration, logger *slog.Logger) *Cron {
	return &Cron{engine: engine, interval: interval, logger: logger}
}

func (c *Cron) Run(ctx context.Context) error {
	for {
		c.logger.Info("starting similarity computation")

		if err := c.engine.ComputeFeedSimilarity(ctx); err != nil {
			c.logger.Error("feed similarity failed", "error", err)
		}

		if err := c.engine.ComputeUserSimilarity(ctx); err != nil {
			c.logger.Error("user similarity failed", "error", err)
		}

		if err := c.engine.ComputeRecommendations(ctx); err != nil {
			c.logger.Error("recommendations failed", "error", err)
		}

		c.logger.Info("similarity computation complete", "next_run", c.interval)

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(c.interval):
		}
	}
}
