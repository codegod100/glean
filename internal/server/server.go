package server

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
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
	"pkg.rbrt.fr/glean/internal/tmpl"
	"pkg.rbrt.fr/glean/static"
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

func splitString(s, sep string) []string {
	return strings.Split(s, sep)
}

type Server struct {
	dbs         *db.Store
	router      *chi.Mux
	templates   *template.Template
	logger      *slog.Logger
	oauth       *oauth.ClientApp
	oauthStore  *db.OAuthStore
	fetcher     *feed.Fetcher
	scheduler   *feed.Scheduler
	engine      *cluster.Engine
	feedback    *feedback.Service
	scraper     *scraper.Scraper
	llm         ml.TextModel
	clientID    string
	callbackURL string
	sessionKey  []byte
}

func New(
	dbs *db.Store,
	clientID, callbackURL, addr string,
	scheduler *feed.Scheduler,
	fetcher *feed.Fetcher,
	engine *cluster.Engine,
	logger *slog.Logger,
	sessionKey []byte,
	textModel ml.TextModel,
) *Server {
	oauthStore := db.NewOAuthStore(dbs)

	var config oauth.ClientConfig
	if clientID == "" {
		host := addr
		if strings.HasPrefix(host, ":") {
			host = "127.0.0.1" + host
		}
		cbURL := fmt.Sprintf("http://%s/auth/callback", host)
		config = oauth.NewLocalhostConfig(cbURL, oauthScopes)
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
		callbackURL: callbackURL,
		sessionKey:  sessionKey,
	}

	s.setupMiddleware()
	s.setupRoutes()
	s.loadTemplates()

	return s
}

func (s *Server) setupMiddleware() {
	s.router.Use(s.realIPLogger)
	s.router.Use(middleware.Recoverer)
	s.router.Use(middleware.Compress(5))
	s.router.Use(s.metricsMiddleware)
	s.router.Use(cors.Handler(cors.Options{
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders: []string{"Accept", "Authorization", "Content-Type"},
		MaxAge:         300,
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
	if strings.HasPrefix(p, "/static/") {
		return "/static/*"
	}
	return p
}

func (s *Server) setupRoutes() {
	s.router.Get("/", s.handleIndex)

	s.router.Route("/dashboard", func(r chi.Router) {
		r.Use(s.requireAuth)
		r.Get("/", s.handleDashboard)
	})

	s.router.Route("/feeds", func(r chi.Router) {
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

	s.router.Route("/articles", func(r chi.Router) {
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

	s.router.Route("/trending", func(r chi.Router) {
		r.Get("/", s.handleTrending)
	})

	s.router.Route("/profile", func(r chi.Router) {
		r.Use(s.requireAuth)
		r.Get("/{did}", s.handleProfile)
	})

	s.router.Route("/library", func(r chi.Router) {
		r.Use(s.requireAuth)
		r.Get("/", s.handleLibrary)
		r.Post("/create", s.handleCreateAnnotation)
		r.Post("/{id}/delete", s.handleDeleteAnnotation)
	})

	s.router.Route("/recs", func(r chi.Router) {
		r.Use(s.requireAuth)
		r.Get("/articles", s.handleArticleRecommendations)
		r.Get("/feeds", s.handleFeedRecommendations)
		r.Get("/people", s.handlePeopleRecommendations)
		r.Post("/dismiss-feed", s.handleDismissFeedRecommendation)
		r.Post("/dismiss-article", s.handleDismissArticleRecommendation)
		r.Post("/dismiss-person", s.handleDismissPersonRecommendation)
	})

	s.router.Route("/settings", func(r chi.Router) {
		r.Use(s.requireAuth)
		r.Post("/languages/{code}", s.handleToggleLanguage)
		r.Post("/expanded-view", s.handleToggleExpandedView)
		r.Post("/digest-enabled", s.handleToggleDigestEnabled)
	})

	s.router.With(s.requireAuth).Get("/digest", s.handleDigest)
	s.router.With(s.requireAuth).Post("/digest/mark-read", s.handleDigestMarkRead)

	s.router.Get("/auth/login", s.handleAuthLogin)
	s.router.Get("/auth/register", s.handleAuthRegister)
	s.router.Get("/auth/resolve", s.handleAuthResolve)
	s.router.Post("/auth/start", s.handleAuthStart)
	s.router.Get("/auth/callback", s.handleAuthCallback)
	s.router.Post("/auth/logout", s.handleAuthLogout)
	s.router.Get("/oauth/client-metadata", s.handleOAuthClientMetadata)

	xrpc := atproto.NewXRPCHandler(s.dbs, s.engine)
	s.router.Get("/xrpc/at.glean.listSubscriptions", xrpc.ListSubscriptions)
	s.router.Get("/xrpc/at.glean.listAnnotations", xrpc.ListAnnotations)
	s.router.Get("/xrpc/at.glean.listLikes", xrpc.ListLikes)
	s.router.Get("/xrpc/at.glean.getTrending", xrpc.GetTrending)
	s.router.Get("/xrpc/at.glean.getRecommendations", xrpc.GetRecommendations)
	s.router.Get("/xrpc/at.glean.listFeedLists", xrpc.ListFeedLists)

	s.router.Get("/terms", s.handleTerms)
	s.router.Get("/sitemap.xml", s.handleSitemap)
	s.router.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.FS(static.Files))))
	s.router.Handle("/metrics", promhttp.Handler())
	s.router.Get("/stats", s.handleStats)
	s.router.NotFound(s.handleNotFound)
}

func (s *Server) loadTemplates() {
	fm := template.FuncMap{
		"dict": func(values ...any) (map[string]any, error) {
			if len(values)%2 != 0 {
				return nil, fmt.Errorf("dict requires even number of arguments")
			}
			m := make(map[string]any, len(values)/2)
			for i := 0; i < len(values); i += 2 {
				key, ok := values[i].(string)
				if !ok {
					return nil, fmt.Errorf("dict key must be string")
				}
				m[key] = values[i+1]
			}
			return m, nil
		},
		"formatDate": func(t time.Time) string {
			return t.Format("Jan 02, 2006")
		},
		"formatDateTime": func(t time.Time) string {
			return t.Format("Jan 02, 2006 15:04")
		},
		"split": func(sv, sep string) []string {
			if sv == "" {
				return nil
			}
			var result []string
			for _, p := range splitString(sv, sep) {
				if p != "" {
					result = append(result, p)
				}
			}
			return result
		},
		"repeat": func(str string, n int) string {
			b := strings.Builder{}
			for range n {
				b.WriteString(str)
			}
			return b.String()
		},
		"int": func(n int64) int {
			return int(n)
		},
		"add": func(a, b int) int {
			return a + b
		},
		"youtubeID": func(rawURL string) string {
			u, err := url.Parse(rawURL)
			if err != nil {
				return ""
			}
			host := strings.ToLower(u.Hostname())
			if host == "youtu.be" {
				id := strings.TrimPrefix(u.Path, "/")
				if id != "" {
					return id
				}
				return ""
			}
			if host == "www.youtube.com" || host == "youtube.com" || host == "m.youtube.com" {
				if u.Path == "/watch" || u.Path == "/watch/" {
					id := u.Query().Get("v")
					if id != "" {
						return id
					}
				}
				if after, ok := strings.CutPrefix(u.Path, "/embed/"); ok {
					id := after
					if id != "" {
						return id
					}
				}
				if after, ok := strings.CutPrefix(u.Path, "/shorts/"); ok {
					id := after
					if id != "" {
						return id
					}
				}
			}
			return ""
		},
		"isEmbedURL": func(rawURL string) bool {
			u, err := url.Parse(rawURL)
			if err != nil {
				return false
			}
			host := strings.ToLower(u.Hostname())
			return slices.Contains([]string{
				"www.youtube.com", "youtube.com", "m.youtube.com", "youtu.be",
				"vimeo.com", "player.vimeo.com",
				"open.spotify.com", "embed.spotify.com",
				"w.soundcloud.com",
				"bandcamp.com",
			}, host)
		},
		"sanitizeHTML": func(input string) template.HTML {
			return template.HTML(sanitizeHTML(input))
		},
		"plainText": plainText,
		"now":       time.Now,
		"activeClass": func(activePath, linkPath string) string {
			if activePath == linkPath || (len(activePath) > len(linkPath) && activePath[:len(linkPath)+1] == linkPath+"/") {
				return "bg-spot-hover text-spot-text font-bold"
			}
			return "text-spot-secondary"
		},
		"csrfInput": func(token any) template.HTML {
			s, ok := token.(string)
			if !ok || s == "" {
				return ""
			}
			return template.HTML(`<input type="hidden" name="csrf_token" value="` + s + `">`)
		},
		"paginationURL": func(baseURL string, page int, queryParams map[string]string) string {
			u, _ := url.Parse(baseURL)
			q := u.Query()
			for k, v := range queryParams {
				q.Set(k, v)
			}
			if page > 1 {
				q.Set("page", fmt.Sprintf("%d", page))
			} else {
				q.Del("page")
			}
			u.RawQuery = q.Encode()
			return u.String()
		},
		"containsString": func(slice any, s string) bool {
			sl, ok := slice.([]string)
			if !ok {
				return false
			}
			return slices.Contains(sl, s)
		},
	}

	var err error
	s.templates, err = template.New("").Funcs(fm).ParseFS(tmpl.Files, "*.html", "partials/*.html")
	if err != nil {
		s.logger.Error("failed to load templates", "error", err)
	}
}

func (s *Server) pdsClientForUser(r *http.Request) *atproto.Client {
	session := s.getSessionData(r)
	if session == nil {
		return nil
	}

	if session.SessionID == "" {
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

func (s *Server) BackfillFromCollectionDir(ctx context.Context, collectionDirURL string, concurrency int) {
	if collectionDirURL == "" {
		return
	}

	s.logger.Info("backfilling from collection directory", "url", collectionDirURL)

	dids, err := atproto.FetchSubscriberDIDs(ctx, collectionDirURL)
	if err != nil {
		s.logger.Error("failed to fetch subscriber DIDs", "error", err)
		return
	}

	existing, err := s.dbs.Users.UserDIDs(ctx)
	if err != nil {
		s.logger.Error("failed to list existing users", "error", err)
		return
	}

	var missing []string
	for _, did := range dids {
		if !existing[did] {
			missing = append(missing, did)
		}
	}

	s.logger.Info("collection directory backfill", "total", len(dids), "missing", len(missing))

	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	for _, did := range missing {
		if ctx.Err() != nil {
			break
		}

		sem <- struct{}{}
		wg.Add(1)

		go func(did string) {
			defer func() { <-sem }()
			defer wg.Done()

			if _, err := s.dbs.Users.CreateUser(ctx, did); err != nil {
				s.logger.Error("failed to create user during backfill", "error", err, "did", did)
				return
			}

			pdsURL, err := atproto.ResolvePDSEndpoint(ctx, did)
			if err != nil {
				s.logger.Error("failed to resolve PDS for backfill", "error", err, "did", did)
				return
			}

			client := atproto.NewUnauthenticatedClient(pdsURL)
			sync := atproto.NewSync(s.dbs.Articles, s.dbs.Users, client, s.logger)
			if err := sync.Run(ctx, did); err != nil {
				s.logger.Error("backfill sync failed", "error", err, "did", did)
			}
		}(did)
	}

	wg.Wait()
	s.logger.Info("collection directory backfill complete")
}

func (s *Server) runSyncAll(ctx context.Context) {
	if n, err := s.oauthStore.CountActiveUsers(ctx); err == nil {
		metrics.ActiveUsers.Set(float64(n))
	}

	users, err := s.dbs.Users.ListUsers(ctx)
	if err != nil {
		s.logger.Error("failed to list users for sync", "error", err)
		return
	}

	for _, u := range users {
		sessionIDs, err := s.oauthStore.ListSessionsForDID(ctx, u.DID)
		if err != nil || len(sessionIDs) == 0 {
			continue
		}

		did, err := syntax.ParseDID(u.DID)
		if err != nil {
			continue
		}

		sess, err := s.oauth.ResumeSession(ctx, did, sessionIDs[0])
		if err != nil {
			s.logger.Warn("failed to resume session for periodic sync", "error", err, "did", u.DID)
			continue
		}

		client := atproto.NewClient(sess.APIClient())
		sync := atproto.NewSync(s.dbs.Articles, s.dbs.Users, client, s.logger)
		if err := sync.Run(ctx, u.DID); err != nil {
			metrics.SyncErrors.Inc()
			s.logger.Error("periodic sync failed", "error", err, "did", u.DID)
		}

		metrics.SyncRuns.Inc()
	}

	// Recompute subscriber_count once after all users are synced.
	if err := s.dbs.Articles.RecountSubscriberCounts(ctx); err != nil {
		s.logger.Error("recount subscriber counts failed", "error", err)
	}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.router.ServeHTTP(w, r)
}

func (s *Server) handleNotFound(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotFound)
	s.render(w, r, "404.html", nil)
}

func (s *Server) renderError(w http.ResponseWriter, r *http.Request, code int, title, message string) {
	if isHXRequest(r) {
		w.WriteHeader(code)
		w.Write([]byte(message))
		return
	}
	w.WriteHeader(code)
	s.render(w, r, "error.html", map[string]any{
		"Title":   title,
		"Message": message,
	})
}

func (s *Server) render(w http.ResponseWriter, r *http.Request, name string, data map[string]any) {
	if data == nil {
		data = map[string]any{}
	}

	data["HasLLM"] = s.llm != nil

	if cookie, err := r.Cookie("glean_csrf"); err == nil {
		data["CSRFToken"] = cookie.Value
	}

	if isHXRequest(r) {
		if err := s.templates.ExecuteTemplate(w, name, data); err != nil {
			s.logger.Error("template error", "error", err, "template", name)
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	var buf bytes.Buffer
	if err := s.templates.ExecuteTemplate(&buf, name, data); err != nil {
		s.logger.Error("template error", "error", err, "template", name)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	path := r.URL.Path
	if len(path) > 1 {
		path = strings.TrimRight(path, "/")
	}
	baseData := map[string]any{
		"Content":    template.HTML(buf.String()),
		"ActivePath": path,
	}
	if data != nil {
		if u, ok := data["User"]; ok {
			baseData["User"] = u
		}
		if csrf, ok := data["CSRFToken"]; ok {
			baseData["CSRFToken"] = csrf
		}
		if hasLLM, ok := data["HasLLM"]; ok {
			baseData["HasLLM"] = hasLLM
		}
	}

	if err := s.templates.ExecuteTemplate(w, "base.html", baseData); err != nil {
		s.logger.Error("template error", "error", err, "template", "base.html")
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
