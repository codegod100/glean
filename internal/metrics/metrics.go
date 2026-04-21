package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	FeedsFetched = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "glean_feeds_fetched_total",
		Help: "Total number of feed fetch attempts",
	}, []string{"status"})

	FeedsFetchedDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "glean_feed_fetch_duration_seconds",
		Help:    "Time spent fetching a single feed",
		Buckets: prometheus.DefBuckets,
	})

	ArticlesUpserted = promauto.NewCounter(prometheus.CounterOpts{
		Name: "glean_articles_upserted_total",
		Help: "Total number of articles upserted",
	})

	JetstreamEvents = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "glean_jetstream_events_total",
		Help: "Total number of jetstream events processed",
	}, []string{"collection", "action"})

	JetstreamErrors = promauto.NewCounter(prometheus.CounterOpts{
		Name: "glean_jetstream_errors_total",
		Help: "Total number of jetstream handler errors",
	})

	JetstreamReconnects = promauto.NewCounter(prometheus.CounterOpts{
		Name: "glean_jetstream_reconnects_total",
		Help: "Number of jetstream reconnections",
	})

	HTTPRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "glean_http_requests_total",
		Help: "Total HTTP requests",
	}, []string{"method", "path", "status"})

	HTTPRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "glean_http_request_duration_seconds",
		Help:    "HTTP request duration",
		Buckets: prometheus.DefBuckets,
	}, []string{"method", "path"})

	ActiveUsers = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "glean_users_active_total",
		Help: "Number of users with active sessions",
	})

	ClusterRuns = promauto.NewCounter(prometheus.CounterOpts{
		Name: "glean_cluster_runs_total",
		Help: "Number of cluster/recommendation computation runs",
	})

	ClusterDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "glean_cluster_duration_seconds",
		Help:    "Time spent computing recommendations",
		Buckets: []float64{1, 5, 10, 30, 60, 120, 300},
	})

	SyncRuns = promauto.NewCounter(prometheus.CounterOpts{
		Name: "glean_pds_sync_runs_total",
		Help: "Number of PDS sync runs",
	})

	SyncErrors = promauto.NewCounter(prometheus.CounterOpts{
		Name: "glean_pds_sync_errors_total",
		Help: "Number of PDS sync errors",
	})
)
