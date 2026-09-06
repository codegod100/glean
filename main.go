package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/pprof"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"pkg.rbrt.fr/glean/internal/atproto"
	"pkg.rbrt.fr/glean/internal/cluster"
	"pkg.rbrt.fr/glean/internal/db"
	"pkg.rbrt.fr/glean/internal/feed"
	"pkg.rbrt.fr/glean/internal/feedback"
	"pkg.rbrt.fr/glean/internal/ml"
	"pkg.rbrt.fr/glean/internal/server"

	vec "github.com/asg017/sqlite-vec-go-bindings/cgo"
)

func main() {
	addr := flag.String("addr", envOr("GLEAN_ADDR", ":8080"), "listen address")
	dbPath := flag.String("db", envOr("GLEAN_DB", "glean.db"), "database path")
	jetstreamURL := flag.String("jetstream", envOr("GLEAN_JETSTREAM", "wss://jetstream1.eurosky.network"), "Jetstream URL")
	syncInterval := flag.Duration("sync-interval", envDuration("GLEAN_SYNC_INTERVAL", 8*time.Hour), "PDS sync interval")
	clusterInterval := flag.Duration("cluster-interval", envDuration("GLEAN_CLUSTER_INTERVAL", 1*time.Hour), "cluster recomputation interval")
	fetchInterval := flag.Duration("fetch-interval", envDuration("GLEAN_FETCH_INTERVAL", 15*time.Minute), "feed fetch tick interval")
	collectionDirURL := flag.String("collection-dir", envOr("GLEAN_COLLECTION_DIR_URL", ""), "collection directory URL for startup backfill")
	backfillConcurrency := flag.Int("backfill-concurrency", envInt("GLEAN_BACKFILL_CONCURRENCY", 5), "max concurrent backfill workers")
	articleRetentionDays := flag.Int("article-retention-days", envInt("GLEAN_ARTICLE_RETENTION_DAYS", 30), "delete articles older than this many days")
	sessionKey := envOr("GLEAN_SESSION_KEY", "")
	flag.Parse()

	if sessionKey == "" {
		fmt.Fprintln(os.Stderr, "GLEAN_SESSION_KEY is required")
		os.Exit(1)
	}

	atproto.InitIdentity(envOr("GLEAN_PLC_URL", "https://plc.eurosky.network"))

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	if pprofAddr := envOr("GLEAN_PPROF_ADDR", ""); pprofAddr != "" {
		mux := http.NewServeMux()
		mux.HandleFunc("/debug/pprof/", pprof.Index)
		mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
		mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
		mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
		mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
		go func() {
			logger.Info("starting pprof server", "addr", pprofAddr)
			if err := http.ListenAndServe(pprofAddr, mux); err != nil {
				logger.Error("pprof server error", "error", err)
			}
		}()
	}

	vec.Auto()
	dbs, err := db.Open(*dbPath)
	if err != nil {
		logger.Error("failed to open databases", "error", err)
		os.Exit(1)
	}
	defer dbs.Close()
	// Ingest must use the same window as the purge, or every fetch re-adds the
	// articles the last purge deleted.
	dbs.Articles.SetRetentionDays(*articleRetentionDays)

	clientID := envOr("GLEAN_OAUTH_CLIENT_ID", "")
	frontendURL := envOr("GLEAN_FRONTEND_URL", "")
	if frontendURL == "" {
		fmt.Fprintln(os.Stderr, "GLEAN_FRONTEND_URL is required (the public browser origin, e.g. https://glean.at)")
		os.Exit(1)
	}

	storeAdapter := db.NewFeedAdapter(dbs.Articles)
	siteFetcher := atproto.NewStandardSiteFetcher(logger)
	scheduler := feed.NewScheduler(storeAdapter, siteFetcher, logger, *fetchInterval, 30*time.Minute)

	var embedder ml.Embedder
	if embedURL := envOr("GLEAN_EMBED_BASE_URL", ""); embedURL != "" {
		embedder = ml.NewEmbedder(ml.EmbedConfig{
			BaseURL:   embedURL,
			APIKey:    envOr("GLEAN_EMBED_API_KEY", ""),
			Model:     envOr("GLEAN_EMBED_MODEL", "text-embedding-3-small"),
			Dimension: envInt("GLEAN_EMBED_DIMENSION", 1536),
		})
	}

	if embedder != nil {
		if err := dbs.InitVecTables(embedder.Dimension()); err != nil {
			logger.Error("failed to init vec tables", "error", err)
			os.Exit(1)
		}
	}

	var llm ml.TextModel
	if llmURL := envOr("GLEAN_LLM_BASE_URL", ""); llmURL != "" {
		llm = ml.NewLLM(ml.LLMConfig{
			BaseURL: llmURL,
			APIKey:  envOr("GLEAN_LLM_API_KEY", ""),
			Model:   envOr("GLEAN_LLM_MODEL", "gpt-4o-mini"),
		})
	}

	engine := cluster.NewEngine(dbs.SQLDB(), dbs.Articles, embedder, llm, feedback.NewService(dbs.SQLDB()), logger, cluster.DefaultConfig())

	fetcher := feed.NewFetcher(siteFetcher)
	srv := server.New(dbs, clientID, frontendURL, scheduler, fetcher, engine, logger, []byte(sessionKey), llm)

	cron := cluster.NewCron(engine, *clusterInterval, logger, dbs, *articleRetentionDays)

	handler := atproto.NewStreamDBHandler(dbs.Articles, dbs.Users, logger)
	// Only stream events of known users; without this filter jetstream
	// delivers every matching record on the network and the database grows
	// unbounded.
	jetstream := atproto.NewJetstreamConsumer(*jetstreamURL, handler.Handle, logger, dbs.CursorStore(), dbs.Users.UserDIDList)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Serve HTTP first so sign-in works while the one-time purge is still
	// deleting network-wide rows. Jobs that write wait until that pass finishes.
	go func() {
		retentionCtx, cancelRetention := context.WithTimeout(ctx, 30*time.Minute)
		defer cancelRetention()
		start := time.Now()
		unknownStats, err := dbs.PurgeUnknownUserRows(retentionCtx)
		if err != nil {
			logger.Error("initial purge of unknown-user rows failed", "error", err)
		} else if unknownStats.Total() > 0 {
			logger.Info("purged rows of unknown users", "stats", unknownStats)
		}
		expired, err := dbs.PurgeExpiredArticles(retentionCtx, *articleRetentionDays)
		if err != nil {
			logger.Error("initial article retention purge failed", "error", err)
		} else if expired > 0 {
			logger.Info("purged expired articles", "count", expired)
		}
		if unknownStats.Total()+expired > 0 {
			if err := dbs.ReclaimSpace(retentionCtx); err != nil {
				logger.Error("reclaiming database space incomplete", "error", err)
			} else {
				logger.Info("reclaimed database space")
			}
		}
		logger.Info("initial retention complete", "elapsed", time.Since(start).Round(time.Second))

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
