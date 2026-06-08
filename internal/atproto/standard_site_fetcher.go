package atproto

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"pkg.rbrt.fr/glean/internal/feed"
)

func IsATProtoFeedURL(feedURL string) bool {
	return strings.HasPrefix(feedURL, "at://")
}

type StandardSiteFetcher struct {
	logger *slog.Logger
}

func NewStandardSiteFetcher(logger *slog.Logger) *StandardSiteFetcher {
	return &StandardSiteFetcher{logger: logger}
}

func (f *StandardSiteFetcher) FetchFeed(ctx context.Context, feedURL string) (*feed.ParseResult, error) {
	parsed, ok := ParseRecordURI(feedURL)
	if !ok {
		return nil, fmt.Errorf("invalid AT URI: %s", feedURL)
	}

	if parsed.Collection != CollectionStandardPublication {
		return nil, fmt.Errorf("unsupported AT Protocol collection: %s", parsed.Collection)
	}

	pdsURL, err := ResolvePDSEndpoint(ctx, parsed.DID)
	if err != nil {
		return nil, fmt.Errorf("resolving PDS for %s: %w", parsed.DID, err)
	}

	client := NewUnauthenticatedClient(pdsURL)

	raw, err := client.GetRecord(ctx, parsed.DID, parsed.Collection, parsed.RKey)
	if err != nil {
		return nil, fmt.Errorf("fetching publication record: %w", err)
	}

	var pub StandardPublicationRecord
	if err := json.Unmarshal(raw, &pub); err != nil {
		return nil, fmt.Errorf("parsing publication record: %w", err)
	}

	documents, err := f.fetchDocuments(ctx, client, parsed.DID, feedURL, pub.URL)
	if err != nil {
		f.logger.Warn("failed to fetch documents for publication", "error", err, "uri", feedURL)
	}

	result := &feed.ParseResult{
		Feed: feed.Feed{
			URL:         feedURL,
			Title:       pub.Name,
			SiteURL:     pub.URL,
			Description: pub.Description,
			Type:        "atproto",
		},
		Articles: documents,
	}

	return result, nil
}

func (f *StandardSiteFetcher) fetchDocuments(ctx context.Context, client *Client, did, publicationURI, publicationURL string) ([]feed.Article, error) {
	records, err := listAllRecords(ctx, client, did, CollectionStandardDocument)
	if err != nil {
		return nil, fmt.Errorf("listing documents: %w", err)
	}

	var articles []feed.Article
	for _, r := range records {
		var doc StandardDocumentRecord
		if err := json.Unmarshal(r.Value, &doc); err != nil {
			continue
		}
		if doc.Title == "" {
			continue
		}
		if doc.Site != publicationURI && doc.Site != publicationURL {
			continue
		}

		published := parseRFC3339(doc.PublishedAt)
		updated := parseRFC3339(doc.UpdatedAt)

		articleURL := publicationURL + doc.Path
		if related := doc.RelatedLinkURL(); related != "" && articleURL == "" {
			articleURL = related
		}

		articles = append(articles, feed.Article{
			FeedURL:   publicationURI,
			GUID:      r.URI,
			Title:     doc.Title,
			URL:       articleURL,
			Author:    firstContributor(doc.Contributors),
			Content:   doc.TextContent,
			Summary:   doc.Description,
			Published: published,
			Updated:   updated,
		})
	}

	return articles, nil
}

func firstContributor(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var contributors []struct {
		DisplayName string `json:"displayName"`
	}
	if err := json.Unmarshal(raw, &contributors); err != nil {
		return ""
	}
	if len(contributors) == 0 || contributors[0].DisplayName == "" {
		return ""
	}
	return contributors[0].DisplayName
}

func listAllRecords(ctx context.Context, client *Client, did, collection string) ([]Record, error) {
	var all []Record
	cursor := ""
	for {
		records, next, err := client.ListRecords(ctx, did, collection, 100, cursor)
		if err != nil {
			return nil, err
		}
		all = append(all, records...)
		if next == "" || len(records) == 0 {
			break
		}
		cursor = next
	}
	return all, nil
}

func parseRFC3339(s string) time.Time {
	t, _ := time.Parse(time.RFC3339, s)
	return t
}
