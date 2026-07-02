package server

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"pkg.rbrt.fr/glean/internal/atproto"
	"pkg.rbrt.fr/glean/internal/cluster"
	"pkg.rbrt.fr/glean/internal/db"
	"pkg.rbrt.fr/glean/internal/ml"
)

// JSON response helpers.

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// nonNil returns s, or an empty slice if s is nil, so it serializes as []
// instead of null. Use only at the DB→response boundary for []string fields
// whose zero value is nil.
func nonNil[T any](s []T) []T {
	if s == nil {
		return make([]T, 0)
	}
	return s
}

type errorResponse struct {
	Error string `json:"error"`
}

func writeAPIError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, errorResponse{Error: msg})
}

// Wire types. Nullable db fields are converted to pointers so they serialize as
// null instead of the raw sql.Null* struct.

type User struct {
	DID         string `json:"did"`
	Handle      string `json:"handle"`
	DisplayName string `json:"display_name"`
	AvatarURL   string `json:"avatar_url"`
}

func toUser(u *db.User) User {
	if u == nil {
		return User{}
	}
	return User{
		DID:         u.DID,
		Handle:      u.Handle,
		DisplayName: u.DisplayName,
		AvatarURL:   u.AvatarURL,
	}
}

type Article struct {
	ID             int64      `json:"id"`
	FeedURL        string     `json:"feed_url"`
	FeedTitle      string     `json:"feed_title"`
	FeedFaviconURL string     `json:"feed_favicon_url"`
	Title          string     `json:"title"`
	URL            string     `json:"url"`
	Author         string     `json:"author"`
	Summary        string     `json:"summary"`
	Content        string     `json:"content"`
	FullContent    string     `json:"full_content"`
	Published      *time.Time `json:"published"`
	Updated        *time.Time `json:"updated"`
	IsRead         bool       `json:"is_read"`
	LikeCount      int        `json:"like_count"`
	HasLiked       bool       `json:"has_liked"`
}

func toArticle(a *db.Article) Article {
	if a == nil {
		return Article{}
	}
	return Article{
		ID:             a.ID,
		FeedURL:        a.FeedURL,
		FeedTitle:      a.FeedTitle,
		FeedFaviconURL: nullStr(a.FeedFaviconURL),
		Title:          a.Title,
		URL:            nullStr(a.URL),
		Author:         nullStr(a.Author),
		Summary:        nullStr(a.Summary),
		Content:        nullStr(a.Content),
		FullContent:    nullStr(a.FullContent),
		Published:      nullTime(a.Published),
		Updated:        nullTime(a.Updated),
		IsRead:         a.IsRead.Valid && a.IsRead.Bool,
		LikeCount:      a.LikeCount,
		HasLiked:       a.HasLiked,
	}
}

type Feed struct {
	FeedURL         string     `json:"feed_url"`
	Title           string     `json:"title"`
	SiteURL         string     `json:"site_url"`
	Description     string     `json:"description"`
	FeedType        string     `json:"feed_type"`
	FaviconURL      string     `json:"favicon_url"`
	SubscriberCount int        `json:"subscriber_count"`
	ErrorCount      int        `json:"error_count"`
	LastError       string     `json:"last_error"`
	LastFetchedAt   *time.Time `json:"last_fetched_at"`
}

func toFeed(f *db.Feed) Feed {
	if f == nil {
		return Feed{}
	}
	return Feed{
		FeedURL:         f.FeedURL,
		Title:           nullStr(f.Title),
		SiteURL:         nullStr(f.SiteURL),
		Description:     nullStr(f.Description),
		FeedType:        nullStr(f.FeedType),
		FaviconURL:      nullStr(f.FaviconURL),
		SubscriberCount: f.SubscriberCount,
		ErrorCount:      f.ErrorCount,
		LastError:       nullStr(f.LastError),
		LastFetchedAt:   nullTime(f.LastFetchedAt),
	}
}

type Subscription struct {
	ID          int64      `json:"id"`
	FeedURL     string     `json:"feed_url"`
	FeedTitle   string     `json:"feed_title"`
	Category    string     `json:"category"`
	AddedAt     *time.Time `json:"added_at"`
	UnreadCount int        `json:"unread_count"`
	FaviconURL  string     `json:"favicon_url"`
}

func toSubscription(s *db.Subscription) Subscription {
	if s == nil {
		return Subscription{}
	}
	return Subscription{
		ID:          s.ID,
		FeedURL:     s.FeedURL,
		FeedTitle:   s.FeedTitle,
		Category:    nullStr(s.Category),
		AddedAt:     nullTime(s.AddedAt),
		UnreadCount: s.UnreadCount,
		FaviconURL:  nullStr(s.FaviconURL),
	}
}

type Annotation struct {
	ID           int64      `json:"id"`
	AuthorDID    string     `json:"author_did"`
	AuthorHandle string     `json:"author_handle"`
	FeedURL      string     `json:"feed_url"`
	ArticleURL   string     `json:"article_url"`
	ArticleID    *int64     `json:"article_id"`
	Quote        string     `json:"quote"`
	Note         string     `json:"note"`
	Tags         []string   `json:"tags"`
	Rating       *int64     `json:"rating"`
	CreatedAt    *time.Time `json:"created_at"`
}

func toAnnotation(a *db.Annotation) Annotation {
	if a == nil {
		return Annotation{Tags: make([]string, 0)}
	}
	tags := make([]string, 0)
	if a.Tags.Valid && a.Tags.String != "" {
		for t := range strings.SplitSeq(a.Tags.String, ",") {
			if t != "" {
				tags = append(tags, t)
			}
		}
	}
	return Annotation{
		ID:           a.ID,
		AuthorDID:    a.AuthorDID,
		AuthorHandle: a.AuthorHandle,
		FeedURL:      a.FeedURL,
		ArticleURL:   a.ArticleURL,
		ArticleID:    nullInt64(a.ArticleID),
		Quote:        nullStr(a.Quote),
		Note:         nullStr(a.Note),
		Tags:         tags,
		Rating:       nullInt64(a.Rating),
		CreatedAt:    nullTime(a.CreatedAt),
	}
}

type TrendingItem struct {
	ArticleID       int64  `json:"article_id"`
	Title           string `json:"title"`
	URL             string `json:"url"`
	Author          string `json:"author"`
	Summary         string `json:"summary"`
	FeedURL         string `json:"feed_url"`
	FeedTitle       string `json:"feed_title"`
	FaviconURL      string `json:"favicon_url"`
	LikeCount       int    `json:"like_count"`
	AnnotationCount int    `json:"annotation_count"`
	HasLiked        bool   `json:"has_liked"`
}

func toTrendingItem(t *db.TrendingItem) TrendingItem {
	if t == nil {
		return TrendingItem{}
	}
	return TrendingItem{
		ArticleID:       t.ArticleID,
		Title:           t.Title,
		URL:             t.URL,
		Author:          t.Author,
		Summary:         t.Summary,
		FeedURL:         t.FeedURL,
		FeedTitle:       t.FeedTitle,
		FaviconURL:      t.FaviconURL,
		LikeCount:       t.LikeCount,
		AnnotationCount: t.AnnotationCount,
		HasLiked:        t.HasLiked,
	}
}

type FeedRecommendation struct {
	FeedURL         string  `json:"feed_url"`
	Title           string  `json:"title"`
	SiteURL         string  `json:"site_url"`
	Description     string  `json:"description"`
	SubscriberCount int     `json:"subscriber_count"`
	FaviconURL      string  `json:"favicon_url"`
	Score           float64 `json:"score"`
}

func toFeedRecommendation(r *cluster.FeedRecommendation) FeedRecommendation {
	if r == nil {
		return FeedRecommendation{}
	}
	return FeedRecommendation{
		FeedURL:         r.FeedURL,
		Title:           r.Title,
		SiteURL:         r.SiteURL,
		Description:     r.Description,
		SubscriberCount: r.SubscriberCount,
		FaviconURL:      r.FaviconURL,
		Score:           r.Score,
	}
}

type PersonRecommendation struct {
	DID         string  `json:"did"`
	Handle      string  `json:"handle"`
	DisplayName string  `json:"display_name"`
	AvatarURL   string  `json:"avatar_url"`
	CommonFeeds int     `json:"common_feeds"`
	CommonLikes int     `json:"common_likes"`
	CommonTags  int     `json:"common_tags"`
	IsFollowed  bool    `json:"is_followed"`
	Score       float64 `json:"score"`
}

func toPersonRecommendation(r *cluster.PersonRecommendation) PersonRecommendation {
	if r == nil {
		return PersonRecommendation{}
	}
	return PersonRecommendation{
		DID:         r.DID,
		Handle:      r.Handle,
		DisplayName: r.DisplayName,
		AvatarURL:   r.AvatarURL,
		CommonFeeds: r.CommonFeeds,
		CommonLikes: r.CommonLikes,
		CommonTags:  r.CommonTags,
		IsFollowed:  r.IsFollowed,
		Score:       r.Jaccard,
	}
}

type ArticleRecommendation struct {
	ArticleID  int64      `json:"article_id"`
	Title      string     `json:"title"`
	URL        string     `json:"url"`
	FeedURL    string     `json:"feed_url"`
	FeedTitle  string     `json:"feed_title"`
	FaviconURL string     `json:"favicon_url"`
	Author     string     `json:"author"`
	Summary    string     `json:"summary"`
	Published  *time.Time `json:"published"`
	Score      float64    `json:"score"`
}

func toArticleRecommendation(r *cluster.ArticleRecommendation) ArticleRecommendation {
	if r == nil {
		return ArticleRecommendation{}
	}
	return ArticleRecommendation{
		ArticleID:  r.ArticleID,
		Title:      r.Title,
		URL:        r.URL,
		FeedURL:    r.FeedURL,
		FeedTitle:  r.FeedTitle,
		FaviconURL: r.FaviconURL,
		Author:     r.Author,
		Summary:    r.Summary,
		Published:  nullTime(r.Published),
		Score:      r.Score,
	}
}

// null helpers.

func nullStr(s sql.NullString) string {
	if s.Valid {
		return s.String
	}
	return ""
}

func nullTime(t sql.NullTime) *time.Time {
	if t.Valid {
		return &t.Time
	}
	return nil
}

func nullInt64(n sql.NullInt64) *int64 {
	if n.Valid {
		v := n.Int64
		return &v
	}
	return nil
}

// --- /api/me ---

type meResponse struct {
	User      *User  `json:"user"`
	CSRFToken string `json:"csrf_token"`
	HasLLM    bool   `json:"has_llm"`
	ClientID  string `json:"client_id"`
}

// --- dashboard ---

type dashboardResponse struct {
	User              User           `json:"user"`
	SubscriptionCount int            `json:"subscription_count"`
	UnreadCount       int            `json:"unread_count"`
	Articles          []Article      `json:"articles"`
	PersonalTrending  []TrendingItem `json:"personal_trending"`
	GlobalTrending    []TrendingItem `json:"global_trending"`
	DigestEnabled     bool           `json:"digest_enabled"`
	HasLLM            bool           `json:"has_llm"`
	Now               int64          `json:"now"`
}

// --- articles ---

type articlesResponse struct {
	User         User       `json:"user"`
	Articles     []Article  `json:"articles"`
	FeedURL      string     `json:"feed_url"`
	Status       string     `json:"status"`
	SearchQuery  string     `json:"search_query"`
	SortOldest   bool       `json:"sort_oldest"`
	Category     string     `json:"category"`
	Categories   []string   `json:"categories"`
	ExpandedView bool       `json:"expanded_view"`
	Pagination   Pagination `json:"pagination"`
	Now          int64      `json:"now"`
	Feed         *Feed      `json:"feed,omitempty"`
	IsSubscribed bool       `json:"is_subscribed,omitempty"`
}

type articleDetailResponse struct {
	User           User         `json:"user"`
	CurrentUserDID string       `json:"current_user_did"`
	Article        Article      `json:"article"`
	Feed           Feed         `json:"feed"`
	Annotations    []Annotation `json:"annotations"`
	NextID         *int64       `json:"next_id"`
	NextSuffix     string       `json:"next_suffix"`
}

type newArticleCountResponse struct {
	Count int `json:"count"`
}

type articleStateResponse struct {
	ID     int64 `json:"id"`
	IsRead bool  `json:"is_read"`
}

type likeResponse struct {
	ID        int64 `json:"id"`
	Liked     bool  `json:"liked"`
	LikeCount int   `json:"like_count"`
}

type fetchContentResponse struct {
	ID          int64  `json:"id"`
	FullContent string `json:"full_content"`
}

// --- feeds ---

type feedsResponse struct {
	User              User           `json:"user"`
	Subscriptions     []Subscription `json:"subscriptions"`
	SubscriptionCount int            `json:"subscription_count"`
	Categories        []string       `json:"categories"`
	Category          string         `json:"category"`
	DeadFeeds         []Feed         `json:"dead_feeds"`
	Pagination        Pagination     `json:"pagination"`
}

type subscriptionResponse struct {
	Subscription Subscription `json:"subscription"`
}

type feedListResponse struct {
	Subscriptions []Subscription `json:"subscriptions"`
}

type deadFeedsResponse struct {
	DeadFeeds []Feed `json:"dead_feeds"`
}

type opmlUploadResponse struct {
	Added int `json:"added"`
}

// --- trending ---

type trendingResponse struct {
	User       *User          `json:"user"`
	Trending   []TrendingItem `json:"trending"`
	Scope      string         `json:"scope"`
	Pagination Pagination     `json:"pagination"`
}

// --- library ---

type libraryResponse struct {
	User        User         `json:"user"`
	Articles    []Article    `json:"articles"`
	Annotations []Annotation `json:"annotations"`
	LikedPage   Pagination   `json:"liked_page"`
	AnnotPage   Pagination   `json:"annot_page"`
}

type annotationResponse struct {
	Annotation Annotation `json:"annotation"`
}

// --- profile ---

type profileResponse struct {
	User               User           `json:"user"`
	ProfileUser        User           `json:"profile_user"`
	Subscriptions      []Subscription `json:"subscriptions"`
	Annotations        []Annotation   `json:"annotations"`
	SubscriptionCount  int            `json:"subscription_count"`
	AnnotationCount    int            `json:"annotation_count"`
	UserLanguages      []string       `json:"user_languages"`
	AvailableLanguages []ml.Language  `json:"available_languages"`
	ExpandedView       bool           `json:"expanded_view"`
	DigestEnabled      bool           `json:"digest_enabled"`
}

// --- recommendations ---

type articleRecsResponse struct {
	Articles []Article `json:"articles"`
}

type feedRecsResponse struct {
	Feeds             []FeedRecommendation `json:"feeds"`
	SubscriptionCount int                  `json:"subscription_count"`
}

type peopleRecsResponse struct {
	Followed []PersonRecommendation `json:"followed"`
	Discover []PersonRecommendation `json:"discover"`
}

// --- settings ---

type languagesResponse struct {
	Languages []string `json:"languages"`
}

type expandedViewResponse struct {
	ExpandedView bool `json:"expanded_view"`
}

type digestEnabledResponse struct {
	DigestEnabled bool `json:"digest_enabled"`
}

// --- digest ---

// (digestCtx in digest_handler.go is already a typed response struct.)

// --- stats ---

type statsResponse struct {
	User    *User                     `json:"user"`
	Metrics map[string][]metricFamily `json:"metrics"`
}

// --- auth ---

type oauthEnabledResponse struct {
	OAuthEnabled bool `json:"oauth_enabled"`
}

type redirectResponse struct {
	Redirect string `json:"redirect"`
}

type actorsResponse struct {
	Actors []atproto.Actor `json:"actors"`
}
