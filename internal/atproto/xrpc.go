package atproto

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
)

type XRPCHandler struct {
	db *sql.DB
}

func NewXRPCHandler(db *sql.DB) *XRPCHandler {
	return &XRPCHandler{db: db}
}

func (h *XRPCHandler) ListSubscriptions(w http.ResponseWriter, r *http.Request) {
	repo := chi.URLParam(r, "repo")
	category := r.URL.Query().Get("category")
	limit := parseIntParam(r, "limit", 50)
	cursor := r.URL.Query().Get("cursor")

	query := `
		SELECT s.id, f.feed_url, COALESCE(s.title, f.title), s.category, s.added_at
		FROM subscriptions s
		JOIN feeds f ON s.feed_url = f.feed_url
		WHERE s.user_did = ?`
	args := []any{repo}

	if category != "" {
		query += " AND s.category = ?"
		args = append(args, category)
	}
	if cursor != "" {
		query += " AND s.id > ?"
		args = append(args, cursor)
	}

	query += " ORDER BY s.id ASC LIMIT ?"
	args = append(args, limit+1)

	rows, err := h.db.QueryContext(r.Context(), query, args...)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	subs := make([]SubscriptionView, 0)
	for rows.Next() {
		var id int
		var feedURL, title string
		var cat, addedAt sql.NullString
		if err := rows.Scan(&id, &feedURL, &title, &cat, &addedAt); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		sv := SubscriptionView{
			URI: fmtATURI(repo, CollectionSubscription, strconv.Itoa(id)),
			Value: SubscriptionRecord{
				CreatedAt: addedAt.String,
				FeedURL:   feedURL,
				Title:     title,
				Category:  cat.String,
			},
			IndexedAt: addedAt.String,
		}
		subs = append(subs, sv)
	}

	resp := ListSubscriptionsResponse{Subscriptions: subs}
	if len(subs) > limit {
		resp.Cursor = strconv.Itoa(limit)
		resp.Subscriptions = subs[:limit]
	}

	writeJSON(w, resp)
}

func (h *XRPCHandler) ListAnnotations(w http.ResponseWriter, r *http.Request) {
	feedURL := r.URL.Query().Get("feedUrl")
	articleURL := r.URL.Query().Get("articleUrl")
	author := r.URL.Query().Get("author")
	limit := parseIntParam(r, "limit", 50)
	cursor := r.URL.Query().Get("cursor")

	query := `
		SELECT a.uri, a.cid, u.did, u.handle, a.feed_url, a.article_url,
		       a.quote, a.note, a.tags, a.rating, a.created_at
		FROM annotations a
		JOIN users u ON a.author_did = u.did
		WHERE 1=1`
	args := []any{}

	if feedURL != "" {
		query += " AND a.feed_url = ?"
		args = append(args, feedURL)
	}
	if articleURL != "" {
		query += " AND a.article_url = ?"
		args = append(args, articleURL)
	}
	if author != "" {
		query += " AND a.author_did = ?"
		args = append(args, author)
	}
	if cursor != "" {
		query += " AND a.id > ?"
		args = append(args, cursor)
	}

	query += " ORDER BY a.id ASC LIMIT ?"
	args = append(args, limit+1)

	rows, err := h.db.QueryContext(r.Context(), query, args...)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	annotations := make([]AnnotationView, 0)
	for rows.Next() {
		var uri, did, handle, fURL, artURL, createdAt string
		var cid, quote, note, tags sql.NullString
		var rating sql.NullInt64
		if err := rows.Scan(&uri, &cid, &did, &handle, &fURL, &artURL, &quote, &note, &tags, &rating, &createdAt); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		var tagSlice []string
		if tags.Valid && tags.String != "" {
			tagSlice = strings.Split(tags.String, ",")
		}

		av := AnnotationView{
			URI: uri,
			CID: cid.String,
			Author: ActorView{
				DID:    did,
				Handle: handle,
			},
			Value: AnnotationRecord{
				CreatedAt:  createdAt,
				FeedURL:    fURL,
				ArticleURL: artURL,
				Quote:      quote.String,
				Note:       note.String,
				Tags:       tagSlice,
				Rating:     int(rating.Int64),
			},
			IndexedAt: createdAt,
		}
		annotations = append(annotations, av)
	}

	resp := ListAnnotationsResponse{Annotations: annotations}
	if len(annotations) > limit {
		resp.Cursor = strconv.Itoa(limit)
		resp.Annotations = annotations[:limit]
	}

	writeJSON(w, resp)
}

func (h *XRPCHandler) ListLikes(w http.ResponseWriter, r *http.Request) {
	author := r.URL.Query().Get("author")
	feedURL := r.URL.Query().Get("feedUrl")
	limit := parseIntParam(r, "limit", 50)
	cursor := r.URL.Query().Get("cursor")

	query := `
		SELECT l.uri, l.cid, u.did, u.handle, l.feed_url, l.article_url, l.created_at
		FROM likes l
		JOIN users u ON l.author_did = u.did
		WHERE 1=1`
	args := []any{}

	if author != "" {
		query += " AND l.author_did = ?"
		args = append(args, author)
	}
	if feedURL != "" {
		query += " AND l.feed_url = ?"
		args = append(args, feedURL)
	}
	if cursor != "" {
		query += " AND l.id > ?"
		args = append(args, cursor)
	}

	query += " ORDER BY l.id ASC LIMIT ?"
	args = append(args, limit+1)

	rows, err := h.db.QueryContext(r.Context(), query, args...)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	likes := make([]LikeView, 0)
	for rows.Next() {
		var uri, did, handle, fURL, artURL, createdAt string
		var cid sql.NullString
		if err := rows.Scan(&uri, &cid, &did, &handle, &fURL, &artURL, &createdAt); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		lv := LikeView{
			URI: uri,
			CID: cid.String,
			Author: ActorView{
				DID:    did,
				Handle: handle,
			},
			Value: LikeRecord{
				CreatedAt:  createdAt,
				FeedURL:    fURL,
				ArticleURL: artURL,
			},
			IndexedAt: createdAt,
		}
		likes = append(likes, lv)
	}

	resp := ListLikesResponse{Likes: likes}
	if len(likes) > limit {
		resp.Cursor = strconv.Itoa(limit)
		resp.Likes = likes[:limit]
	}

	writeJSON(w, resp)
}

func (h *XRPCHandler) GetTrending(w http.ResponseWriter, r *http.Request) {
	limit := parseIntParam(r, "limit", 25)
	cursor := r.URL.Query().Get("cursor")
	since := r.URL.Query().Get("since")

	query := `
		SELECT l.feed_url, l.article_url, a.title, COUNT(*) as like_count
		FROM likes l
		LEFT JOIN articles a ON l.article_url = a.url
		WHERE 1=1`
	args := []any{}

	if since != "" {
		query += " AND l.created_at >= ?"
		args = append(args, since)
	}
	if cursor != "" {
		query += " AND l.article_url > ?"
		args = append(args, cursor)
	}

	query += " GROUP BY l.feed_url, l.article_url ORDER BY like_count DESC LIMIT ?"
	args = append(args, limit+1)

	rows, err := h.db.QueryContext(r.Context(), query, args...)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	articles := make([]TrendingArticle, 0)
	for rows.Next() {
		var feedURL, articleURL string
		var title sql.NullString
		var likeCount int
		if err := rows.Scan(&feedURL, &articleURL, &title, &likeCount); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		ta := TrendingArticle{
			FeedURL:    feedURL,
			ArticleURL: articleURL,
			Title:      title.String,
			LikeCount:  likeCount,
		}
		articles = append(articles, ta)
	}

	resp := GetTrendingResponse{Articles: articles}
	if len(articles) > limit {
		resp.Cursor = strconv.Itoa(limit)
		resp.Articles = articles[:limit]
	}

	writeJSON(w, resp)
}

func (h *XRPCHandler) GetRecommendations(w http.ResponseWriter, r *http.Request) {
	repo := r.URL.Query().Get("repo")
	limit := min(parseIntParam(r, "limit", 20), 50)

	feedRows, err := h.db.QueryContext(r.Context(), `
		SELECT r.feed_url, f.title, f.site_url, f.description, f.subscriber_count, r.score
		FROM user_feed_recommendations r
		JOIN feeds f ON r.feed_url = f.feed_url
		WHERE r.user_did = ?
		ORDER BY r.score DESC
		LIMIT ?
	`, repo, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer feedRows.Close()

	feeds := make([]RecommendedFeed, 0)
	for feedRows.Next() {
		var feedURL, title, siteURL, description string
		var subscriberCount int
		var score float64
		if err := feedRows.Scan(&feedURL, &title, &siteURL, &description, &subscriberCount, &score); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		feeds = append(feeds, RecommendedFeed{
			FeedURL:         feedURL,
			Title:           title,
			SiteURL:         siteURL,
			Description:     description,
			SubscriberCount: subscriberCount,
			Score:           score,
		})
	}

	peopleRows, err := h.db.QueryContext(r.Context(), `
		SELECT u.did, u.handle, u.display_name, u.avatar_url, s.jaccard, s.common_feeds
		FROM user_similarity s
		JOIN users u ON (
			CASE WHEN s.user_a = ? THEN s.user_b ELSE s.user_a END
		) = u.did
		WHERE s.user_a = ? OR s.user_b = ?
		ORDER BY s.jaccard DESC
		LIMIT ?
	`, repo, repo, repo, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer peopleRows.Close()

	people := make([]RecommendedPerson, 0)
	for peopleRows.Next() {
		var did, handle string
		var displayName, avatar sql.NullString
		var jaccard float64
		var commonFeeds int
		if err := peopleRows.Scan(&did, &handle, &displayName, &avatar, &jaccard, &commonFeeds); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		people = append(people, RecommendedPerson{
			DID:         did,
			Handle:      handle,
			DisplayName: displayName.String,
			Avatar:      avatar.String,
			Jaccard:     jaccard,
			CommonFeeds: commonFeeds,
		})
	}

	writeJSON(w, GetRecommendationsResponse{Feeds: feeds, People: people})
}

func (h *XRPCHandler) ListFeedLists(w http.ResponseWriter, r *http.Request) {
	actorsParam := r.URL.Query().Get("actors")
	limit := parseIntParam(r, "limit", 50)
	cursor := r.URL.Query().Get("cursor")

	var dids []string
	if actorsParam != "" {
		dids = strings.Split(actorsParam, ",")
	}

	if len(dids) == 0 {
		writeJSON(w, ListFeedListsResponse{})
		return
	}

	placeholders := make([]string, len(dids))
	args := make([]any, len(dids))
	for i, d := range dids {
		placeholders[i] = "?"
		args[i] = d
	}

	query := `
		SELECT u.did, u.handle, COUNT(s.id) as subscription_count
		FROM users u
		LEFT JOIN subscriptions s ON u.did = s.user_did
		WHERE u.did IN (` + strings.Join(placeholders, ",") + `)`

	if cursor != "" {
		query += " AND u.did > ?"
		args = append(args, cursor)
	}

	query += " GROUP BY u.did ORDER BY u.did ASC LIMIT ?"
	args = append(args, limit+1)

	rows, err := h.db.QueryContext(r.Context(), query, args...)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	feedLists := make([]FeedListEntry, 0)
	type userRow struct {
		did      string
		subCount int
	}
	var users []userRow

	for rows.Next() {
		var did, handle string
		var subCount int
		if err := rows.Scan(&did, &handle, &subCount); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		users = append(users, userRow{did: did, subCount: subCount})
	}

	subsByDID := make(map[string][]SubscriptionRecord)
	if len(users) > 0 {
		ph := make([]string, len(users))
		args := make([]any, len(users))
		for i, u := range users {
			ph[i] = "?"
			args[i] = u.did
		}
		subRows, err := h.db.QueryContext(r.Context(), `
			SELECT s.user_did, s.feed_url, COALESCE(s.title, f.title), s.category
			FROM subscriptions s
			JOIN feeds f ON s.feed_url = f.feed_url
			WHERE s.user_did IN (`+strings.Join(ph, ",")+`)
			ORDER BY s.user_did, s.added_at DESC
		`, args...)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		for subRows.Next() {
			var did, feedURL, title string
			var cat sql.NullString
			if err := subRows.Scan(&did, &feedURL, &title, &cat); err != nil {
				subRows.Close()
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			subsByDID[did] = append(subsByDID[did], SubscriptionRecord{
				FeedURL:  feedURL,
				Title:    title,
				Category: cat.String,
			})
		}
		subRows.Close()
	}

	for _, u := range users {
		feedLists = append(feedLists, FeedListEntry{
			DID:               u.did,
			SubscriptionCount: u.subCount,
			Subscriptions:     subsByDID[u.did],
		})
	}

	resp := ListFeedListsResponse{Feeds: feedLists}
	if len(feedLists) > limit {
		resp.Cursor = strconv.Itoa(limit)
		resp.Feeds = feedLists[:limit]
	}

	writeJSON(w, resp)
}

func parseIntParam(r *http.Request, key string, defaultVal int) int {
	v := r.URL.Query().Get(key)
	if v == "" {
		return defaultVal
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return defaultVal
	}
	return n
}

func fmtATURI(did, collection, rkey string) string {
	return "at://" + did + "/" + collection + "/" + rkey
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
