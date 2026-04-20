package server

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	oauth "github.com/bluesky-social/indigo/atproto/auth/oauth"
	"github.com/bluesky-social/indigo/atproto/syntax"

	"pkg.rbrt.fr/glean/internal/atproto"
	"pkg.rbrt.fr/glean/internal/db"
	"pkg.rbrt.fr/glean/internal/feed"
	"pkg.rbrt.fr/glean/internal/sanitize"
)

func splitString(s, sep string) []string {
	return strings.Split(s, sep)
}

type Server struct {
	db          *db.DB
	router      *chi.Mux
	templates   *template.Template
	logger      *slog.Logger
	oauth       *oauth.ClientApp
	oauthStore  *db.OAuthStore
	fetcher     *feed.Fetcher
	clientID    string
	callbackURL string
}

func New(database *db.DB, clientID, callbackURL, addr string, logger *slog.Logger) *Server {
	oauthStore := db.NewOAuthStore(database)

	var config oauth.ClientConfig
	if clientID == "" {
		host := addr
		if strings.HasPrefix(host, ":") {
			host = "127.0.0.1" + host
		}
		cbURL := fmt.Sprintf("http://%s/auth/callback", host)
		config = oauth.NewLocalhostConfig(cbURL, []string{"atproto", "transition:generic"})
	} else {
		config = oauth.NewPublicConfig(clientID, callbackURL, []string{"atproto", "transition:generic"})
	}
	oauthClient := oauth.NewClientApp(&config, oauthStore)

	s := &Server{
		db:          database,
		router:      chi.NewRouter(),
		logger:      logger,
		oauth:       oauthClient,
		oauthStore:  oauthStore,
		fetcher:     feed.NewFetcher(),
		clientID:    clientID,
		callbackURL: callbackURL,
	}

	s.setupMiddleware()
	s.setupRoutes()
	s.loadTemplates()

	return s
}

func (s *Server) setupMiddleware() {
	s.router.Use(middleware.Logger)
	s.router.Use(middleware.Recoverer)
	s.router.Use(middleware.Compress(5))
	s.router.Use(cors.Handler(cors.Options{
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders: []string{"Accept", "Authorization", "Content-Type"},
		MaxAge:         300,
	}))
	s.router.Use(s.sessionMiddleware)
	s.router.Use(s.csrfMiddleware)
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
		r.Delete("/remove", s.handleRemoveFeed)
		r.Post("/opml/upload", s.handleOPMLUpload)
		r.Get("/opml/download", s.handleOPMLDownload)
		r.Post("/refresh", s.handleRefreshFeeds)
		r.Get("/list", s.handleFeedList)
		r.Post("/set-interval", s.handleUpdateFeedInterval)
		r.Get("/discover-url", s.handleDiscoverFeedURL)
		r.Post("/clear", s.handleClearAllSubscriptions)
	})

	s.router.Route("/articles", func(r chi.Router) {
		r.Use(s.requireAuth)
		r.Get("/", s.handleArticles)
		r.Get("/{id}", s.handleArticleDetail)
		r.Post("/{id}/read", s.handleMarkRead)
		r.Post("/{id}/unread", s.handleMarkUnread)
		r.Post("/{id}/like", s.handleLikeArticle)
		r.Post("/mark-all-read", s.handleMarkAllRead)
	})

	s.router.Route("/trending", func(r chi.Router) {
		r.Use(s.requireAuth)
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
	})

	s.router.Get("/auth/login", s.handleAuthLogin)
	s.router.Post("/auth/start", s.handleAuthStart)
	s.router.Get("/auth/callback", s.handleAuthCallback)
	s.router.Post("/auth/logout", s.handleAuthLogout)
	s.router.Get("/oauth/client-metadata", s.handleOAuthClientMetadata)

	xrpc := atproto.NewXRPCHandler(s.db.DB)
	s.router.Get("/xrpc/at.glean.listSubscriptions", xrpc.ListSubscriptions)
	s.router.Get("/xrpc/at.glean.listAnnotations", xrpc.ListAnnotations)
	s.router.Get("/xrpc/at.glean.listLikes", xrpc.ListLikes)
	s.router.Get("/xrpc/at.glean.getTrending", xrpc.GetTrending)
	s.router.Get("/xrpc/at.glean.getRecommendations", xrpc.GetRecommendations)
	s.router.Get("/xrpc/at.glean.listFeedLists", xrpc.ListFeedLists)

	s.router.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))
}

func (s *Server) loadTemplates() {
	fm := template.FuncMap{
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
		"sanitizeHTML": func(input string) template.HTML {
			return template.HTML(sanitize.HTML(input))
		},
		"plainText": func(input string) string {
			return sanitize.PlainText(input)
		},
		"now": func() time.Time {
			return time.Now()
		},
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
	}

	var allFiles []string
	matches, _ := filepath.Glob("internal/tmpl/*.html")
	allFiles = append(allFiles, matches...)
	partials, _ := filepath.Glob("internal/tmpl/partials/*.html")
	allFiles = append(allFiles, partials...)

	var err error
	s.templates, err = template.New("").Funcs(fm).ParseFiles(allFiles...)
	if err != nil {
		s.logger.Error("failed to load templates", "error", err)
	}
}

func (s *Server) pdsClientForUser(r *http.Request) *atproto.Client {
	session := s.getSessionData(r)
	if session == nil {
		return nil
	}

	if session.SessionID != "" {
		did, err := syntax.ParseDID(session.DID)
		if err != nil {
			return nil
		}
		sess, err := s.oauth.ResumeSession(r.Context(), did, session.SessionID)
		if err != nil {
			s.logger.Warn("failed to resume OAuth session", "error", err)
			return nil
		}
		apiClient := sess.APIClient()
		return &atproto.Client{APIClient: apiClient}
	}

	if session.AccessToken != "" && session.PDSURL != "" {
		return atproto.NewClient(session.PDSURL, session.AccessToken)
	}
	return nil
}

func (s *Server) syncUserInBackground(userDID string, client *atproto.Client) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		sync := atproto.NewSync(s.db, client, s.logger)
		if err := sync.Run(ctx, userDID); err != nil {
			s.logger.Error("background sync failed", "error", err, "did", userDID)
		}
	}()
}

func (s *Server) PeriodicSync(ctx context.Context, interval time.Duration) {
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

func (s *Server) runSyncAll(ctx context.Context) {
	users, err := s.db.ListUsers(ctx)
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

		client := &atproto.Client{APIClient: sess.APIClient()}
		sync := atproto.NewSync(s.db, client, s.logger)
		if err := sync.Run(ctx, u.DID); err != nil {
			s.logger.Error("periodic sync failed", "error", err, "did", u.DID)
		}
	}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.router.ServeHTTP(w, r)
}

func (s *Server) render(w http.ResponseWriter, r *http.Request, name string, data map[string]any) {
	if data == nil {
		data = map[string]any{}
	}

	if cookie, err := r.Cookie("glean_csrf"); err == nil {
		data["CSRFToken"] = cookie.Value
	}

	if r.Header.Get("HX-Request") == "true" {
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
	}

	if err := s.templates.ExecuteTemplate(w, "base.html", baseData); err != nil {
		s.logger.Error("template error", "error", err, "template", "base.html")
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
