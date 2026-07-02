package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	oauth "github.com/bluesky-social/indigo/atproto/auth/oauth"
	"github.com/bluesky-social/indigo/atproto/syntax"

	"pkg.rbrt.fr/glean/internal/atproto"
	"pkg.rbrt.fr/glean/internal/cluster"
	"pkg.rbrt.fr/glean/internal/db"
	"pkg.rbrt.fr/glean/internal/feed"
	"pkg.rbrt.fr/glean/internal/feedback"
	"pkg.rbrt.fr/glean/internal/metrics"
	"pkg.rbrt.fr/glean/internal/ml"
	"pkg.rbrt.fr/glean/internal/scraper"
)

var oauthScopes = []string{
	"atproto",
	"blob:*/*",

	fmt.Sprintf("repo:%s", atproto.CollectionSubscription),
	fmt.Sprintf("repo:%s", atproto.CollectionLike),
	fmt.Sprintf("repo:%s", atproto.CollectionAnnotation),
	fmt.Sprintf("repo:%s", atproto.CollectionMarginNote),

	"rpc:at.glean.listSubscriptions?aud=*",
	"rpc:at.glean.listLikes?aud=*",
	"rpc:at.glean.getTrending?aud=*",
	"rpc:at.glean.getRecommendations?aud=*",
	"rpc:at.glean.listFeedLists?aud=*",
	"rpc:at.glean.listAnnotations?aud=*",

	"rpc:app.bsky.actor.getProfile?aud=*",
}

type Server struct {
	dbs         *db.Store
	router      *chi.Mux
	logger      *slog.Logger
	oauth       *oauth.ClientApp
	oauthStore  *db.OAuthStore
	fetcher     *feed.Fetcher
	scheduler   *feed.Scheduler
	engine      *cluster.Engine
	feedback    *feedback.Service
	scraper     *scraper.Scraper
	llm         ml.TextModel
	clientID    string // empty in localhost dev; toggles the OAuth flow mode
	frontendURL string // public browser origin (e.g. https://glean.at)
	sessionKey  []byte
}

func New(
	dbs *db.Store,
	clientID, frontendURL string,
	scheduler *feed.Scheduler,
	fetcher *feed.Fetcher,
	engine *cluster.Engine,
	logger *slog.Logger,
	sessionKey []byte,
	textModel ml.TextModel,
) *Server {
	oauthStore := db.NewOAuthStore(dbs)

	// The OAuth callback is always served by the frontend at /api/auth/callback
	// (SvelteKit proxies it here). clientID empty = localhost dev flow.
	callbackURL := strings.TrimRight(frontendURL, "/") + "/api/auth/callback"

	var config oauth.ClientConfig
	if clientID == "" {
		config = oauth.NewLocalhostConfig(callbackURL, oauthScopes)
	} else {
		config = oauth.NewPublicConfig(clientID, callbackURL, oauthScopes)
	}
	oauthClient := oauth.NewClientApp(&config, oauthStore)

	s := &Server{
		dbs:         dbs,
		router:      chi.NewMux(),
		logger:      logger,
		oauth:       oauthClient,
		oauthStore:  oauthStore,
		fetcher:     fetcher,
		scheduler:   scheduler,
		engine:      engine,
		feedback:    feedback.NewService(dbs.SQLDB()),
		scraper:     scraper.New(logger),
		llm:         textModel,
		clientID:    clientID,
		frontendURL: frontendURL,
		sessionKey:  sessionKey,
	}

	s.setupMiddleware()
	s.setupRoutes()

	return s
}

func (s *Server) setupMiddleware() {
	s.router.Use(s.realIPLogger)
	s.router.Use(middleware.Recoverer)
	s.router.Use(middleware.Compress(5))
	s.router.Use(s.metricsMiddleware)
	s.router.Use(cors.Handler(cors.Options{
		AllowedOrigins:   s.allowedOrigins(),
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		AllowCredentials: true,
		MaxAge:           300,
	}))
	s.router.Use(s.sessionMiddleware)
	s.router.Use(s.csrfMiddleware)
}

func (s *Server) metricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

		next.ServeHTTP(ww, r)

		path := normalizeMetricsPath(r.URL.Path)
		status := strconv.Itoa(ww.Status())
		metrics.HTTPRequests.WithLabelValues(r.Method, path, status).Inc()
		metrics.HTTPRequestDuration.WithLabelValues(r.Method, path).Observe(time.Since(start).Seconds())
	})
}

func normalizeMetricsPath(p string) string {
	if strings.HasPrefix(p, "/api/articles/") {
		return "/api/articles/*"
	}
	if strings.HasPrefix(p, "/api/profile/") {
		return "/api/profile/*"
	}
	return p
}

func (s *Server) setupRoutes() {
	r := s.router

	r.Get("/api/me", s.handleMe)

	r.Route("/api/dashboard", func(r chi.Router) {
		r.Use(s.requireAuth)
		r.Get("/", s.handleDashboard)
	})

	r.Route("/api/feeds", func(r chi.Router) {
		r.Use(s.requireAuth)
		r.Get("/", s.handleFeeds)
		r.Post("/add", s.handleAddFeed)
		r.Post("/edit", s.handleEditFeed)
		r.Delete("/remove", s.handleRemoveFeed)
		r.Post("/opml/upload", s.handleOPMLUpload)
		r.Get("/opml/download", s.handleOPMLDownload)
		r.Post("/refresh", s.handleRefreshFeeds)
		r.Post("/retry", s.handleRetryFeed)
		r.Get("/list", s.handleFeedList)
		r.Post("/clear", s.handleClearAllSubscriptions)
	})

	r.Route("/api/articles", func(r chi.Router) {
		r.Use(s.requireAuth)
		r.Get("/", s.handleArticles)
		r.Get("/new-count", s.handleNewArticleCount)
		r.Get("/{id}", s.handleArticleDetail)
		r.Post("/{id}/read", s.handleMarkRead)
		r.Post("/{id}/unread", s.handleMarkUnread)
		r.Post("/{id}/like", s.handleLikeArticle)
		r.Post("/{id}/fetch-content", s.handleFetchContent)
		r.Post("/mark-all-read", s.handleMarkAllRead)
	})

	r.Route("/api/trending", func(r chi.Router) {
		r.Get("/", s.handleTrending)
	})

	r.Route("/api/profile", func(r chi.Router) {
		r.Use(s.requireAuth)
		r.Get("/{did}", s.handleProfile)
	})

	r.Route("/api/library", func(r chi.Router) {
		r.Use(s.requireAuth)
		r.Get("/", s.handleLibrary)
		r.Post("/create", s.handleCreateAnnotation)
		r.Post("/{id}/delete", s.handleDeleteAnnotation)
	})

	r.Route("/api/recs", func(r chi.Router) {
		r.Use(s.requireAuth)
		r.Get("/articles", s.handleArticleRecommendations)
		r.Get("/feeds", s.handleFeedRecommendations)
		r.Get("/people", s.handlePeopleRecommendations)
		r.Post("/dismiss-feed", s.handleDismissFeedRecommendation)
		r.Post("/dismiss-article", s.handleDismissArticleRecommendation)
		r.Post("/dismiss-person", s.handleDismissPersonRecommendation)
	})

	r.Route("/api/settings", func(r chi.Router) {
		r.Use(s.requireAuth)
		r.Post("/languages/{code}", s.handleToggleLanguage)
		r.Post("/expanded-view", s.handleToggleExpandedView)
		r.Post("/digest-enabled", s.handleToggleDigestEnabled)
	})

	r.With(s.requireAuth).Get("/api/digest", s.handleDigest)
	r.With(s.requireAuth).Post("/api/digest/mark-read", s.handleDigestMarkRead)

	r.Get("/api/auth/login", s.handleAuthLoginMeta)
	r.Get("/api/auth/register", s.handleAuthRegister)
	r.Get("/api/auth/actors", s.handleAuthResolve)
	r.Post("/api/auth/start", s.handleAuthStart)
	r.Get("/api/auth/callback", s.handleAuthCallback)
	r.Post("/api/auth/logout", s.handleAuthLogout)
	r.Get("/api/oauth/client-metadata", s.handleOAuthClientMetadata)

	xrpc := atproto.NewXRPCHandler(s.dbs, s.engine)
	r.Get("/xrpc/at.glean.listSubscriptions", xrpc.ListSubscriptions)
	r.Get("/xrpc/at.glean.listAnnotations", xrpc.ListAnnotations)
	r.Get("/xrpc/at.glean.listLikes", xrpc.ListLikes)
	r.Get("/xrpc/at.glean.getTrending", xrpc.GetTrending)
	r.Get("/xrpc/at.glean.getRecommendations", xrpc.GetRecommendations)
	r.Get("/xrpc/at.glean.listFeedLists", xrpc.ListFeedLists)

	r.Get("/api/sitemap", s.handleSitemap)
	r.Handle("/metrics", promhttp.Handler())
	r.Get("/api/stats", s.handleStats)
	r.NotFound(s.handleNotFound)
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	csrf := ""
	if c, err := r.Cookie("glean_csrf"); err == nil {
		csrf = c.Value
	}
	var userObj *User
	if user != nil {
		userObj = new(toUser(user))
	}
	writeJSON(w, http.StatusOK, meResponse{
		User:      userObj,
		CSRFToken: csrf,
		HasLLM:    s.llm != nil,
		ClientID:  s.clientID,
	})
}

func (s *Server) pdsClientForUser(r *http.Request) *atproto.Client {
	session := s.getSessionData(r)
	if session == nil || session.SessionID == "" {
		return nil
	}

	did, err := syntax.ParseDID(session.DID)
	if err != nil {
		return nil
	}
	sess, err := s.oauth.ResumeSession(r.Context(), did, session.SessionID)
	if err != nil {
		s.logger.Warn("failed to resume OAuth session", "error", err)
		return nil
	}
	return atproto.NewClient(sess.APIClient())
}

func (s *Server) syncUserInBackground(userDID string, client *atproto.Client) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()

		sync := atproto.NewSync(s.dbs.Articles, s.dbs.Users, client, s.logger)
		if err := sync.Run(ctx, userDID); err != nil {
			s.logger.Error("background sync failed", "error", err, "did", userDID)
		}
	}()
}

// PeriodicSync runs a full PDS sync for all users on a fixed interval.
// Jetstream handles real-time create/update/delete events, but gaps can
// appear after Jetstream downtime or rare missed events. This catches up by
// reconciling each user's PDS records against the local index.
func (s *Server) PeriodicSync(ctx context.Context, interval time.Duration) {
	s.runSyncAll(ctx)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runSyncAll(ctx)
		}
	}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.router.ServeHTTP(w, r)
}

func (s *Server) handleNotFound(w http.ResponseWriter, r *http.Request) {
	writeAPIError(w, http.StatusNotFound, "not found")
}

// allowedOrigins returns the browser origins permitted by CORS. The app is
// normally same-origin (SvelteKit proxies /api to Go); this mainly matters for
// the localhost dev server.
func (s *Server) allowedOrigins() []string {
	if o := originOf(s.frontendURL); o != "" {
		return []string{o}
	}
	return []string{"http://localhost:3000", "http://localhost:5173"}
}

// originOf parses a URL and returns its scheme://host, defaulting an absent
// scheme to https. Returns "" if raw is empty or unparseable.
func originOf(raw string) string {
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return ""
	}
	if u.Scheme == "" {
		u.Scheme = "https"
	}
	return u.Scheme + "://" + u.Host
}
