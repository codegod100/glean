package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/bluesky-social/indigo/atproto/syntax"
)

func (s *Server) sessionMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := s.getUserFromSession(r)
		if user != nil {
			data := s.getSessionData(r)
			if data != nil && data.SessionID != "" && !s.isOAuthSessionValid(r.Context(), data) {
				s.clearUserSession(w)
				next.ServeHTTP(w, r)
				return
			}
			ctx := contextWithUser(r.Context(), user)
			r = r.WithContext(ctx)
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) isOAuthSessionValid(ctx context.Context, data *sessionData) bool {
	did, err := syntax.ParseDID(data.DID)
	if err != nil {
		return false
	}
	_, err = s.oauthStore.GetSession(ctx, did, data.SessionID)
	return err == nil
}

// requireAuth gates API routes. Returns 401 JSON so the SvelteKit load layer
// can redirect unauthenticated users to the login page.
func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if currentUser(r) == nil {
			writeAPIError(w, http.StatusUnauthorized, "authentication required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func csrfToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	return hex.EncodeToString(b)
}

// csrfMiddleware enforces double-submit CSRF. The token is issued in a readable
// cookie (glean_csrf) and must be echoed back via the X-CSRF-Token header or
// csrf_token form field on every state-changing request.
func (s *Server) csrfMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
			if _, err := r.Cookie("glean_csrf"); err != nil {
				http.SetCookie(w, &http.Cookie{
					Name:     "glean_csrf",
					Value:    csrfToken(),
					Path:     "/",
					MaxAge:   86400,
					HttpOnly: false,
					Secure:   s.secureCookies,
					SameSite: http.SameSiteLaxMode,
				})
			}
			next.ServeHTTP(w, r)
			return
		}

		// Origin must match the configured allowlist, not the proxied Host header.
		if !s.originAllowed(r) {
			writeAPIError(w, http.StatusForbidden, "forbidden")
			return
		}

		cookie, err := r.Cookie("glean_csrf")
		if err != nil {
			writeAPIError(w, http.StatusForbidden, "missing csrf token")
			return
		}
		formToken := r.Header.Get("X-CSRF-Token")
		if formToken == "" {
			formToken = r.FormValue("csrf_token")
		}
		if formToken == "" || formToken != cookie.Value {
			writeAPIError(w, http.StatusForbidden, "csrf mismatch")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) originAllowed(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true // non-browser client
	}
	if s.allowedOrigin != "" {
		return origin == s.allowedOrigin
	}
	return sameOrigin(origin, r.Host)
}

func sameOrigin(origin, host string) bool {
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	return u.Host == host
}

func (s *Server) realIPLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w}

		next.ServeHTTP(sw, r)

		ip := r.RemoteAddr
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			ip = strings.TrimSpace(strings.SplitN(xff, ",", 2)[0])
		}

		scheme := "http"
		if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
			scheme = "https"
		}

		s.logger.Info("request",
			"method", r.Method,
			"url", scheme+"://"+r.Host+r.RequestURI,
			"from", ip,
			"status", sw.status,
			"bytes", sw.bytes,
			"duration", time.Since(start).Round(time.Microsecond),
		)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	n, err := w.ResponseWriter.Write(b)
	w.bytes += n
	return n, err
}
