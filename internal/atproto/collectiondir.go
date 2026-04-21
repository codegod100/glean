package atproto

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// https://github.com/bluesky-social/indigo/tree/main/cmd/collectiondir
type listReposByCollectionResponse struct {
	Repos []struct {
		DID string `json:"did"`
	} `json:"repos"`
	Cursor string `json:"cursor"`
}

func FetchSubscriberDIDs(ctx context.Context, baseURL string) ([]string, error) {
	var allDIDs []string
	cursor := ""
	client := &http.Client{Timeout: 30 * time.Second}

	for {
		url := baseURL
		if cursor != "" {
			url = fmt.Sprintf("%s&cursor=%s", baseURL, cursor)
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, fmt.Errorf("building request: %w", err)
		}

		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("fetching collection dir: %w", err)
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("reading response: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("collection dir returned %d: %s", resp.StatusCode, string(body))
		}

		var result listReposByCollectionResponse
		if err := json.Unmarshal(body, &result); err != nil {
			return nil, fmt.Errorf("parsing response: %w", err)
		}

		for _, r := range result.Repos {
			allDIDs = append(allDIDs, r.DID)
		}

		if result.Cursor == "" || len(result.Repos) == 0 {
			break
		}
		cursor = result.Cursor
	}

	return allDIDs, nil
}
