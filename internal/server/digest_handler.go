package server

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/singleflight"

	"pkg.rbrt.fr/glean/internal/db"
)

type digestCtx struct {
	Title       string
	Summary     string
	Excerpt     string
	ArticleIDs  []int64
	GeneratedAt time.Time
	CSRFToken   string
	Consumed    bool
}

type digestCacheEntry struct {
	html       string
	expiry     time.Time
	articleIDs []int64
	consumed   bool
}

var (
	digestCache  sync.Map
	digestFlight singleflight.Group
)

const digestTTL = 24 * time.Hour

var refRe = regexp.MustCompile(`\[(\d+)\]`)

func linkifyRefs(html string, articles []*db.Article) string {
	return refRe.ReplaceAllStringFunc(html, func(match string) string {
		n, err := strconv.Atoi(match[1 : len(match)-1])
		if err != nil || n < 1 || n > len(articles) {
			return match
		}
		a := articles[n-1]
		return fmt.Sprintf(`<a href="/articles/%d" class="text-spot-green hover:underline">[%d]</a>`, a.ID, n)
	})
}

func (s *Server) buildDigestData(ctx context.Context, user *db.User) *digestCtx {
	var (
		articles  []*db.Article
		userLangs []string
	)

	g, gCtx := errgroup.WithContext(ctx)

	g.Go(func() error {
		var err error
		articles, err = s.dbs.Articles.ListUnreadArticles(gCtx, user.DID, "", "", 50, 0, false)
		return err
	})

	g.Go(func() error {
		var err error
		userLangs, err = s.dbs.Users.GetLanguages(gCtx, user.DID)
		return err
	})

	if err := g.Wait(); err != nil {
		s.logger.Warn("digest data error", "error", err, "did", user.DID)
		return nil
	}

	if len(articles) == 0 {
		return nil
	}

	trending, err := s.engine.GetPersonalTrending(ctx, user.DID, userLangs, 5, 0)
	if err != nil {
		s.logger.Warn("failed to get trending for digest", "error", err, "did", user.DID)
	}

	var articleEntries []string
	for _, a := range articles {
		var parts []string
		parts = append(parts, a.Title)
		if a.FeedTitle != "" {
			parts = append(parts, "["+a.FeedTitle+"]")
		}
		if a.URL.Valid && a.URL.String != "" {
			parts = append(parts, a.URL.String)
		}
		if content := digestArticleContent(a); content != "" {
			parts = append(parts, content)
		}
		articleEntries = append(articleEntries, strings.Join(parts, " "))
	}

	trendingTopics := make([]string, len(trending))
	for i, t := range trending {
		trendingTopics[i] = t.Title
	}

	title, summary, err := s.llm.GenerateDigest(ctx, articleEntries, trendingTopics)
	if err != nil {
		s.logger.Warn("failed to generate digest", "error", err, "did", user.DID)
		return nil
	}

	summary = linkifyRefs(summary, articles)

	return &digestCtx{
		Title:   title,
		Summary: summary,
		Excerpt: excerptFromHTML(summary),
		ArticleIDs: func() []int64 {
			ids := make([]int64, len(articles))
			for i, a := range articles {
				ids[i] = a.ID
			}
			return ids
		}(),
		GeneratedAt: time.Now(),
	}
}

const digestContentLimit = 500

func digestArticleContent(a *db.Article) string {
	src := a.FullContent
	if !src.Valid || src.String == "" {
		src = a.Content
	}
	if !src.Valid || src.String == "" {
		src = a.Summary
	}
	if !src.Valid || src.String == "" {
		return ""
	}
	text := plainText(src.String)
	runes := []rune(text)
	if len(runes) > digestContentLimit {
		runes = runes[:digestContentLimit]
	}
	return string(runes)
}

func excerptFromHTML(html string) string {
	text := plainText(html)
	if len(text) > 200 {
		text = text[:200]
		if idx := strings.LastIndex(text, ". "); idx > 0 {
			text = text[:idx+1]
		}
	}
	return text
}

func (s *Server) handleDigest(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	ctx := r.Context()

	settings, err := s.dbs.Users.GetSettings(ctx, user.DID)
	if err != nil {
		s.logger.Warn("failed to get settings for digest", "error", err, "did", user.DID)
	}

	if settings != nil && !settings.DigestEnabled {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	if s.llm == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	unreadCount, _ := s.dbs.Articles.GetUnreadCount(ctx, user.DID, "", "")
	if unreadCount == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	key := user.DID
	if cached, ok := digestCache.Load(key); ok {
		entry := cached.(*digestCacheEntry)
		if time.Now().Before(entry.expiry) {
			if entry.consumed {
				s.renderDigest(w, &digestCtx{Consumed: true})
			} else {
				w.Header().Set("Content-Type", "text/html")
				w.Write([]byte(entry.html))
			}
			return
		}
		digestCache.Delete(key)
	}

	result, _, _ := digestFlight.Do(key, func() (any, error) {
		d := s.buildDigestData(ctx, user)
		if d == nil {
			return nil, nil
		}

		if cookie, err := r.Cookie("glean_csrf"); err == nil {
			d.CSRFToken = cookie.Value
		}

		var buf strings.Builder
		if err := s.templates.ExecuteTemplate(&buf, "partials/digest.html", d); err != nil {
			s.logger.Error("digest template error", "error", err)
			return nil, nil
		}

		html := buf.String()
		digestCache.Store(key, &digestCacheEntry{
			html:       html,
			expiry:     time.Now().Add(digestTTL),
			articleIDs: d.ArticleIDs,
		})

		return html, nil
	})

	if result == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(result.(string)))
}

func (s *Server) renderDigest(w http.ResponseWriter, d *digestCtx) {
	var buf strings.Builder
	if err := s.templates.ExecuteTemplate(&buf, "partials/digest.html", d); err != nil {
		s.logger.Error("digest template error", "error", err)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(buf.String()))
}

func (s *Server) handleDigestMarkRead(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)

	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var ids []int64
	for _, idStr := range r.Form["ids"] {
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			continue
		}
		ids = append(ids, id)
	}

	if len(ids) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	if err := s.dbs.Articles.MarkArticlesRead(r.Context(), user.DID, ids); err != nil {
		s.logger.Error("failed to mark digest articles read", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	digestCache.Store(user.DID, &digestCacheEntry{
		expiry:   time.Now().Add(digestTTL),
		consumed: true,
	})

	w.Header().Set("HX-Refresh", "true")
	s.renderDigest(w, &digestCtx{Consumed: true})
}
