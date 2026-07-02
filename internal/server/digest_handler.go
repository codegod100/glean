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
	Title       string  `json:"title"`
	Summary     string  `json:"summary"`
	Excerpt     string  `json:"excerpt"`
	ArticleIDs  []int64 `json:"article_ids"`
	GeneratedAt int64   `json:"generated_at"`
	Consumed    bool    `json:"consumed"`
}

type digestCacheEntry struct {
	data     *digestCtx
	expiry   time.Time
	consumed bool
}

var (
	digestCache  = sync.Map{}
	digestFlight = singleflight.Group{}
)

const digestTTL = 24 * time.Hour

var refRe = regexp.MustCompile(`\[(\d+)\]`)

// linkifyRefs turns [n] references in the LLM summary into links to the
// corresponding article. The frontend receives the already-linkified HTML.
func linkifyRefs(htmlText string, articles []*db.Article) string {
	return refRe.ReplaceAllStringFunc(htmlText, func(match string) string {
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

	ids := make([]int64, len(articles))
	for i, a := range articles {
		ids[i] = a.ID
	}

	return &digestCtx{
		Title:       title,
		Summary:     summary,
		Excerpt:     excerptFromHTML(summary),
		ArticleIDs:  ids,
		GeneratedAt: time.Now().Unix(),
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

func excerptFromHTML(htmlText string) string {
	text := plainText(htmlText)
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
				writeJSON(w, http.StatusOK, &digestCtx{ArticleIDs: []int64{}, Consumed: true})
			} else {
				writeJSON(w, http.StatusOK, entry.data)
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
		digestCache.Store(key, &digestCacheEntry{
			data:   d,
			expiry: time.Now().Add(digestTTL),
		})
		return d, nil
	})

	if result == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	writeJSON(w, http.StatusOK, result.(*digestCtx))
}

func (s *Server) handleDigestMarkRead(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)

	if err := r.ParseForm(); err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
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
		writeAPIError(w, http.StatusInternalServerError, err.Error())
		return
	}

	digestCache.Store(user.DID, &digestCacheEntry{
		expiry:   time.Now().Add(digestTTL),
		consumed: true,
	})

	writeJSON(w, http.StatusOK, &digestCtx{ArticleIDs: []int64{}, Consumed: true})
}
