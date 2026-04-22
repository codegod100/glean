// External lexicon record types (not owned by glean.at).
// These are maintained by hand to match the upstream lexicon schemas.
// See lexicon_test.go for the test ensuring these stay in sync with lexicons/.
package atproto

import "encoding/json"

type FollowRecord struct {
	Subject   string          `json:"subject"`
	CreatedAt string          `json:"createdAt"`
	Via       json.RawMessage `json:"via,omitempty"`
}

type SkyreaderSubscriptionRecord struct {
	CreatedAt      string   `json:"createdAt"`
	FeedURL        string   `json:"feedUrl"`
	Title          string   `json:"title"`
	SiteURL        string   `json:"siteUrl"`
	Category       string   `json:"category"`
	Tags           []string `json:"tags,omitempty"`
	UpdatedAt      string   `json:"updatedAt,omitempty"`
	Source         string   `json:"source,omitempty"`
	ExternalRef    string   `json:"externalRef,omitempty"`
	SourceType     string   `json:"sourceType,omitempty"`
	SubjectDid     string   `json:"subjectDid,omitempty"`
	CollectionNsid string   `json:"collectionNsid,omitempty"`
	CustomTitle    string   `json:"customTitle,omitempty"`
	CustomIconURL  string   `json:"customIconUrl,omitempty"`
}

type MarginNoteRecord struct {
	Body       *MarginNoteBody      `json:"body,omitempty"`
	Color      string               `json:"color,omitempty"`
	CreatedAt  string               `json:"createdAt"`
	Facets     json.RawMessage      `json:"facets,omitempty"`
	Generator  *MarginNoteGenerator `json:"generator,omitempty"`
	Labels     json.RawMessage      `json:"labels,omitempty"`
	ModifiedAt string               `json:"modifiedAt,omitempty"`
	Motivation string               `json:"motivation"`
	Rights     string               `json:"rights,omitempty"`
	Tags       []string             `json:"tags,omitempty"`
	Target     MarginNoteTarget     `json:"target"`
}

type MarginNoteBody struct {
	Format string `json:"format,omitempty"`
	URI    string `json:"uri,omitempty"`
	Value  string `json:"value,omitempty"`
}

type MarginNoteGenerator struct {
	Homepage string `json:"homepage,omitempty"`
	ID       string `json:"id,omitempty"`
	Name     string `json:"name,omitempty"`
}

type MarginNoteSelector struct {
	ConformsTo string `json:"conformsTo,omitempty"`
	End        int    `json:"end,omitempty"`
	Exact      string `json:"exact,omitempty"`
	Prefix     string `json:"prefix,omitempty"`
	Start      int    `json:"start,omitempty"`
	Suffix     string `json:"suffix,omitempty"`
	Type       string `json:"type"`
	Value      string `json:"value,omitempty"`
}

type MarginNoteTarget struct {
	Selector   *MarginNoteSelector  `json:"selector,omitempty"`
	Source     string               `json:"source"`
	SourceHash string               `json:"sourceHash,omitempty"`
	State      *MarginNoteTimeState `json:"state,omitempty"`
	Title      string               `json:"title,omitempty"`
}

type MarginNoteTimeState struct {
	Cached     string `json:"cached,omitempty"`
	SourceDate string `json:"sourceDate,omitempty"`
}

func (r MarginNoteRecord) ToAnnotation() (articleURL, quote, note string, tags []string) {
	articleURL = r.Target.Source
	if r.Body != nil {
		note = r.Body.Value
	}
	if r.Target.Selector != nil && r.Target.Selector.Exact != "" {
		quote = r.Target.Selector.Exact
	}
	tags = r.Tags
	return
}
