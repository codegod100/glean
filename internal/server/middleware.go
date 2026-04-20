package server

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"
)

func (s *Server) sessionMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := s.getUserFromSession(r)
		if user != nil {
			ctx := contextWithUser(r.Context(), user)
			r = r.WithContext(ctx)
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if currentUser(r) == nil {
			http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func csrfToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}

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
					SameSite: http.SameSiteLaxMode,
				})
			}
			next.ServeHTTP(w, r)
			return
		}

		if r.Header.Get("HX-Request") == "true" {
			origin := r.Header.Get("Origin")
			if origin != "" && !sameOrigin(origin, r.Host) {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
			return
		}

		cookie, err := r.Cookie("glean_csrf")
		if err != nil {
			http.Error(w, "missing csrf token", http.StatusForbidden)
			return
		}
		formToken := r.FormValue("csrf_token")
		if formToken == "" {
			formToken = r.Header.Get("X-CSRF-Token")
		}
		if formToken == "" || formToken != cookie.Value {
			http.Error(w, "csrf mismatch", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func sameOrigin(origin, host string) bool {
	return strings.HasPrefix(origin, "http://"+host) || strings.HasPrefix(origin, "https://"+host)
}
