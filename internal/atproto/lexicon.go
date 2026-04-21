// Lexicon record types are maintained by hand (no lexgen).
// See lexicon_test.go for the test ensuring these stay in sync with lexicons/.
package atproto

import (
	"strings"
	"time"
)

const (
	CollectionSubscription          = "at.glean.subscription"
	CollectionAnnotation            = "at.glean.annotation"
	CollectionLike                  = "at.glean.like"
	CollectionMarginNote            = "at.margin.note"
	CollectionSkyreaderSubscription = "app.skyreader.feed.subscription"
	CollectionBskyFollow            = "app.bsky.graph.follow"
	CollectionTangledFollow         = "sh.tangled.graph.follow"
)

type SubscriptionRecord struct {
	CreatedAt string `json:"createdAt"`
	FeedURL   string `json:"feedUrl"`
	Title     string `json:"title,omitempty"`
	Category  string `json:"category,omitempty"`
}

type AnnotationRecord struct {
	CreatedAt  string   `json:"createdAt"`
	FeedURL    string   `json:"feedUrl"`
	ArticleURL string   `json:"articleUrl"`
	Quote      string   `json:"quote,omitempty"`
	Note       string   `json:"note,omitempty"`
	Tags       []string `json:"tags,omitempty"`
	Rating     int      `json:"rating,omitempty"`
}

type LikeRecord struct {
	CreatedAt  string `json:"createdAt"`
	FeedURL    string `json:"feedUrl"`
	ArticleURL string `json:"articleUrl"`
}

type Record struct {
	URI        string
	CID        string
	DID        string
	Collection string
	RKey       string
	Value      []byte
	IndexedAt  time.Time
}

type SubscriptionView struct {
	URI       string             `json:"uri"`
	CID       string             `json:"cid"`
	Value     SubscriptionRecord `json:"value"`
	IndexedAt string             `json:"indexedAt"`
}

type ListSubscriptionsResponse struct {
	Cursor        string             `json:"cursor,omitempty"`
	Subscriptions []SubscriptionView `json:"subscriptions"`
}

type FeedListEntry struct {
	DID               string               `json:"did"`
	SubscriptionCount int                  `json:"subscriptionCount"`
	Subscriptions     []SubscriptionRecord `json:"subscriptions"`
}

type ListFeedListsResponse struct {
	Cursor string          `json:"cursor,omitempty"`
	Feeds  []FeedListEntry `json:"feeds"`
}

type ActorView struct {
	DID    string `json:"did"`
	Handle string `json:"handle"`
}

type AnnotationView struct {
	URI       string           `json:"uri"`
	CID       string           `json:"cid"`
	Author    ActorView        `json:"author"`
	Value     AnnotationRecord `json:"value"`
	IndexedAt string           `json:"indexedAt"`
}

type ListAnnotationsResponse struct {
	Cursor      string           `json:"cursor,omitempty"`
	Annotations []AnnotationView `json:"annotations"`
}

type LikeView struct {
	URI       string     `json:"uri"`
	CID       string     `json:"cid"`
	Author    ActorView  `json:"author"`
	Value     LikeRecord `json:"value"`
	IndexedAt string     `json:"indexedAt"`
}

type ListLikesResponse struct {
	Cursor string     `json:"cursor,omitempty"`
	Likes  []LikeView `json:"likes"`
}

type TrendingArticle struct {
	FeedURL     string           `json:"feedUrl"`
	ArticleURL  string           `json:"articleUrl"`
	Title       string           `json:"title"`
	LikeCount   int              `json:"likeCount"`
	Annotations []AnnotationView `json:"annotations"`
}

type GetTrendingResponse struct {
	Cursor   string            `json:"cursor,omitempty"`
	Articles []TrendingArticle `json:"articles"`
}

type RecommendedFeed struct {
	FeedURL         string  `json:"feedUrl"`
	Title           string  `json:"title"`
	SiteURL         string  `json:"siteUrl"`
	Description     string  `json:"description"`
	SubscriberCount int     `json:"subscriberCount"`
	Score           float64 `json:"score"`
}

type RecommendedPerson struct {
	DID         string  `json:"did"`
	Handle      string  `json:"handle"`
	DisplayName string  `json:"displayName"`
	Avatar      string  `json:"avatar"`
	Jaccard     float64 `json:"jaccard"`
	CommonFeeds int     `json:"commonFeeds"`
}

type GetRecommendationsResponse struct {
	Feeds  []RecommendedFeed   `json:"feeds"`
	People []RecommendedPerson `json:"people"`
}

type RecordURI struct {
	DID        string
	Collection string
	RKey       string
}

func ParseRecordURI(uri string) (RecordURI, bool) {
	if !strings.HasPrefix(uri, "at://") {
		return RecordURI{}, false
	}
	parts := strings.SplitN(uri[5:], "/", 3)
	if len(parts) != 3 {
		return RecordURI{}, false
	}
	return RecordURI{DID: parts[0], Collection: parts[1], RKey: parts[2]}, true
}
