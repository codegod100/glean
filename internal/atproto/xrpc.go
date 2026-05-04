package atproto

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"pkg.rbrt.fr/glean/internal/cluster"
	"pkg.rbrt.fr/glean/internal/db"
)

type XRPCHandler struct {
	store  *db.Store
	engine *cluster.Engine
}

func NewXRPCHandler(store *db.Store, engine *cluster.Engine) *XRPCHandler {
	return &XRPCHandler{store: store, engine: engine}
}

func (h *XRPCHandler) ListSubscriptions(w http.ResponseWriter, r *http.Request) {
	repo := r.URL.Query().Get("repo")
	if repo == "" {
		http.Error(w, "missing required query param: repo", http.StatusBadRequest)
		return
	}
	category := r.URL.Query().Get("category")
	limit := parseIntParam(r, "limit", 50, 100)
	offset := cursorToOffset(r.URL.Query().Get("cursor"))

	subs, err := h.store.Articles.ListSubscriptions(r.Context(), repo, category, limit+1, offset)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var nextCursor string
	if len(subs) > limit {
		nextCursor = strconv.Itoa(offset + limit)
		subs = subs[:limit]
	}

	views := make([]SubscriptionView, len(subs))
	for i, s := range subs {
		views[i] = SubscriptionView{
			URI: fmtATURI(repo, CollectionSubscription, strconv.FormatInt(s.ID, 10)),
			Value: SubscriptionRecord{
				CreatedAt: formatNullTime(s.AddedAt),
				FeedURL:   s.FeedURL,
				Title:     s.FeedTitle,
				Category:  s.Category.String,
			},
			IndexedAt: formatNullTime(s.AddedAt),
		}
	}

	writeJSON(w, ListSubscriptionsResponse{
		Cursor:        nextCursor,
		Subscriptions: views,
	})
}

func (h *XRPCHandler) ListAnnotations(w http.ResponseWriter, r *http.Request) {
	feedURL := r.URL.Query().Get("feedUrl")
	articleURL := r.URL.Query().Get("articleUrl")
	author := r.URL.Query().Get("author")
	limit := parseIntParam(r, "limit", 50, 100)
	offset := cursorToOffset(r.URL.Query().Get("cursor"))

	annotations, err := h.store.Articles.ListAnnotations(r.Context(), feedURL, articleURL, author, limit+1, offset)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	profiles := resolveProfiles(r.Context(), uniqueDIDsFromAnnotations(annotations))

	var nextCursor string
	if len(annotations) > limit {
		nextCursor = strconv.Itoa(offset + limit)
		annotations = annotations[:limit]
	}

	views := make([]AnnotationView, len(annotations))
	for i, a := range annotations {
		var tagSlice []string
		if a.Tags.Valid && a.Tags.String != "" {
			tagSlice = strings.Split(a.Tags.String, ",")
		}

		views[i] = AnnotationView{
			URI: a.URI,
			CID: a.CID.String,
			Author: ActorView{
				DID:    a.AuthorDID,
				Handle: profiles[a.AuthorDID].Handle,
			},
			Value: AnnotationRecord{
				CreatedAt:  formatNullTime(a.CreatedAt),
				FeedURL:    a.FeedURL,
				ArticleURL: a.ArticleURL,
				Quote:      a.Quote.String,
				Note:       a.Note.String,
				Tags:       tagSlice,
				Rating:     int(a.Rating.Int64),
			},
			IndexedAt: formatNullTime(a.CreatedAt),
		}
	}

	writeJSON(w, ListAnnotationsResponse{
		Cursor:      nextCursor,
		Annotations: views,
	})
}

func (h *XRPCHandler) ListLikes(w http.ResponseWriter, r *http.Request) {
	author := r.URL.Query().Get("author")
	feedURL := r.URL.Query().Get("feedUrl")
	limit := parseIntParam(r, "limit", 50, 100)
	offset := cursorToOffset(r.URL.Query().Get("cursor"))

	likes, err := h.store.Articles.ListLikes(r.Context(), author, feedURL, limit+1, offset)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	profiles := resolveProfiles(r.Context(), uniqueDIDsFromLikes(likes))

	var nextCursor string
	if len(likes) > limit {
		nextCursor = strconv.Itoa(offset + limit)
		likes = likes[:limit]
	}

	views := make([]LikeView, len(likes))
	for i, l := range likes {
		views[i] = LikeView{
			URI: l.URI,
			CID: l.CID.String,
			Author: ActorView{
				DID:    l.AuthorDID,
				Handle: profiles[l.AuthorDID].Handle,
			},
			Value: LikeRecord{
				CreatedAt:  formatNullTime(l.CreatedAt),
				FeedURL:    l.FeedURL,
				ArticleURL: l.ArticleURL,
			},
			IndexedAt: formatNullTime(l.CreatedAt),
		}
	}

	writeJSON(w, ListLikesResponse{
		Cursor: nextCursor,
		Likes:  views,
	})
}

func (h *XRPCHandler) GetTrending(w http.ResponseWriter, r *http.Request) {
	limit := parseIntParam(r, "limit", 25, 100)
	offset := cursorToOffset(r.URL.Query().Get("cursor"))
	since := r.URL.Query().Get("since")

	items, err := h.store.Articles.ListTrendingArticles(r.Context(), "", since, limit+1, offset)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var nextCursor string
	if len(items) > limit {
		nextCursor = strconv.Itoa(offset + limit)
		items = items[:limit]
	}

	articles := make([]TrendingArticle, len(items))
	for i, item := range items {
		articles[i] = TrendingArticle{
			FeedURL:    item.FeedURL,
			ArticleURL: item.URL,
			Title:      item.Title,
			LikeCount:  item.LikeCount,
		}
	}

	writeJSON(w, GetTrendingResponse{
		Cursor:   nextCursor,
		Articles: articles,
	})
}

func (h *XRPCHandler) GetRecommendations(w http.ResponseWriter, r *http.Request) {
	repo := r.URL.Query().Get("repo")
	limit := parseIntParam(r, "limit", 20, 50)
	ctx := r.Context()

	feedRecs, err := h.engine.GetFeedRecommendations(ctx, repo, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	feeds := make([]RecommendedFeed, 0, len(feedRecs))
	for _, rec := range feedRecs {
		feeds = append(feeds, RecommendedFeed{
			FeedURL:         rec.FeedURL,
			Title:           rec.Title,
			SiteURL:         rec.SiteURL,
			Description:     rec.Description,
			SubscriberCount: rec.SubscriberCount,
			Score:           rec.Score,
		})
	}

	peopleRecs, err := h.engine.GetPeopleRecommendations(ctx, repo, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	dids := make([]string, 0, len(peopleRecs))
	for _, rec := range peopleRecs {
		dids = append(dids, rec.DID)
	}
	profiles := resolveProfiles(ctx, dids)

	people := make([]RecommendedPerson, 0, len(peopleRecs))
	for _, rec := range peopleRecs {
		p := profiles[rec.DID]
		people = append(people, RecommendedPerson{
			DID:         rec.DID,
			Handle:      p.Handle,
			DisplayName: rec.DisplayName,
			Avatar:      rec.AvatarURL,
			Jaccard:     rec.Jaccard,
			CommonFeeds: rec.CommonFeeds,
		})
	}

	writeJSON(w, GetRecommendationsResponse{Feeds: feeds, People: people})
}

func (h *XRPCHandler) ListFeedLists(w http.ResponseWriter, r *http.Request) {
	actorsParam := r.URL.Query().Get("actors")
	limit := parseIntParam(r, "limit", 50, 100)
	offset := cursorToOffset(r.URL.Query().Get("cursor"))

	var dids []string
	if actorsParam != "" {
		dids = strings.Split(actorsParam, ",")
	}

	if len(dids) == 0 {
		writeJSON(w, ListFeedListsResponse{})
		return
	}

	const maxActors = 50
	if len(dids) > maxActors {
		dids = dids[:maxActors]
	}

	lists, err := h.store.Articles.ListFeedListsByDIDs(r.Context(), dids, limit+1, offset)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var nextCursor string
	if len(lists) > limit {
		nextCursor = strconv.Itoa(offset + limit)
		lists = lists[:limit]
	}

	entries := make([]FeedListEntry, len(lists))
	for i, l := range lists {
		subs := make([]SubscriptionRecord, len(l.Subscriptions))
		for j, s := range l.Subscriptions {
			subs[j] = SubscriptionRecord{
				FeedURL:  s.FeedURL,
				Title:    s.Title,
				Category: s.Category,
			}
		}
		entries[i] = FeedListEntry{
			DID:               l.DID,
			SubscriptionCount: l.SubscriptionCount,
			Subscriptions:     subs,
		}
	}

	writeJSON(w, ListFeedListsResponse{
		Cursor: nextCursor,
		Feeds:  entries,
	})
}

func parseIntParam(r *http.Request, key string, defaultVal, maxVal int) int {
	v := r.URL.Query().Get(key)
	if v == "" {
		return defaultVal
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 {
		return defaultVal
	}
	if n > maxVal {
		return maxVal
	}
	return n
}

func cursorToOffset(cursor string) int {
	if cursor == "" {
		return 0
	}
	n, err := strconv.Atoi(cursor)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

func fmtATURI(did, collection, rkey string) string {
	return "at://" + did + "/" + collection + "/" + rkey
}

func formatNullTime(nt sql.NullTime) string {
	if nt.Valid {
		return nt.Time.Format(time.RFC3339)
	}
	return ""
}

func writeJSON(w http.ResponseWriter, v any) {
	var buf bytes.Buffer
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(&buf).Encode(v); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Write(buf.Bytes())
}

func uniqueDIDsFromAnnotations(annotations []*db.Annotation) []string {
	seen := make(map[string]bool)
	var dids []string
	for _, a := range annotations {
		if !seen[a.AuthorDID] {
			seen[a.AuthorDID] = true
			dids = append(dids, a.AuthorDID)
		}
	}
	return dids
}

func uniqueDIDsFromLikes(likes []*db.Like) []string {
	seen := make(map[string]bool)
	var dids []string
	for _, l := range likes {
		if !seen[l.AuthorDID] {
			seen[l.AuthorDID] = true
			dids = append(dids, l.AuthorDID)
		}
	}
	return dids
}

func resolveProfiles(ctx context.Context, dids []string) map[string]Profile {
	profiles := make(map[string]Profile, len(dids))
	for _, did := range dids {
		profiles[did] = ResolveProfile(ctx, did)
	}
	return profiles
}
