package cluster

import (
	"context"
	"log/slog"
	"time"

	"pkg.rbrt.fr/glean/internal/metrics"
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
		start := time.Now()

		c.engine.ComputeAll(ctx)

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
