package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"pkg.rbrt.fr/glean/internal/atproto"
	"pkg.rbrt.fr/glean/internal/cluster"
	"pkg.rbrt.fr/glean/internal/db"
	"pkg.rbrt.fr/glean/internal/feed"
	"pkg.rbrt.fr/glean/internal/server"
)

func main() {
	addr := flag.String("addr", envOr("GLEAN_ADDR", ":8080"), "listen address")
	dbPath := flag.String("db", envOr("GLEAN_DB", "glean.db"), "database path")
	jetstreamURL := flag.String("jetstream", envOr("GLEAN_JETSTREAM", "wss://jetstream.glean.at"), "Jetstream URL")
	syncInterval := flag.Duration("sync-interval", envDuration("GLEAN_SYNC_INTERVAL", 30*time.Minute), "PDS sync interval")
	clusterInterval := flag.Duration("cluster-interval", envDuration("GLEAN_CLUSTER_INTERVAL", 1*time.Hour), "cluster recomputation interval")
	fetchInterval := flag.Duration("fetch-interval", envDuration("GLEAN_FETCH_INTERVAL", 15*time.Minute), "feed fetch tick interval")
	collectionDirURL := flag.String("collection-dir", envOr("GLEAN_COLLECTION_DIR_URL", ""), "collection directory URL for startup backfill")
	backfillConcurrency := flag.Int("backfill-concurrency", envInt("GLEAN_BACKFILL_CONCURRENCY", 5), "max concurrent backfill workers")
	sessionKey := envOr("GLEAN_SESSION_KEY", "")
	flag.Parse()

	if sessionKey == "" {
		fmt.Fprintln(os.Stderr, "GLEAN_SESSION_KEY is required")
		os.Exit(1)
	}

	atproto.InitIdentity(envOr("GLEAN_PLC_URL", "https://didplc.glean.at"))

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	dbs, err := db.OpenAll(*dbPath)
	if err != nil {
		logger.Error("failed to open databases", "error", err)
		os.Exit(1)
	}
	defer dbs.Close()

	clientID := envOr("GLEAN_OAUTH_CLIENT_ID", "")
	callbackURL := envOr("GLEAN_OAUTH_REDIRECT_URL", "")

	storeAdapter := db.NewFeedStoreAdapter(dbs.Articles)
	scheduler := feed.NewScheduler(storeAdapter, logger, *fetchInterval, 30*time.Minute)

	engine := cluster.NewEngine(dbs.DB(), logger)

	srv := server.New(dbs, clientID, callbackURL, *addr, scheduler, engine, logger, []byte(sessionKey))

	cron := cluster.NewCron(engine, *clusterInterval, logger)

	handler := atproto.NewStreamDBHandler(dbs.Articles, dbs.Users, logger)
	jetstream := atproto.NewJetstreamConsumer(*jetstreamURL, handler.Handle, logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		if err := scheduler.Run(ctx); err != nil && ctx.Err() == nil {
			logger.Error("scheduler error", "error", err)
		}
	}()
	go func() {
		if err := cron.Run(ctx); err != nil && ctx.Err() == nil {
			logger.Error("cron error", "error", err)
		}
	}()
	go func() {
		srv.PeriodicSync(ctx, *syncInterval)
	}()
	go func() {
		srv.BackfillFromCollectionDir(ctx, *collectionDirURL, *backfillConcurrency)
	}()
	go func() {
		if err := jetstream.Start(ctx); err != nil && ctx.Err() == nil {
			logger.Error("jetstream error", "error", err)
		}
	}()

	httpServer := &http.Server{
		Addr:    *addr,
		Handler: srv,
	}

	sigCh := make(chan os.Signal, 1)
	signal.Reset(syscall.SIGINT, syscall.SIGTERM)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		logger.Info("starting server", "addr", *addr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	sig := <-sigCh
	logger.Info("shutting down", "signal", sig)

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("shutdown error", "error", err)
	}

	cancel()

	fmt.Println("glean stopped")
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envDuration(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}
