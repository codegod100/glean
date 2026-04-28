package atproto

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

type Actor struct {
	DID         string `json:"did"`
	Handle      string `json:"handle"`
	DisplayName string `json:"displayName"`
	Avatar      string `json:"avatar"`
}

var profileClient = &http.Client{Timeout: 10 * time.Second}

// FetchProfile is an unauthenticated profile fetcher. It relies on bluesky api.
func FetchProfile(ctx context.Context, identifier string) (*Actor, error) {
	apiURL := fmt.Sprintf("https://public.api.bsky.app/xrpc/app.bsky.actor.getProfile?actor=%s", url.QueryEscape(identifier))
	return fetchProfileFromURL(ctx, apiURL, identifier)
}

func fetchProfileFromURL(ctx context.Context, apiURL, identifier string) (*Actor, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := profileClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch profile %s: status %d", identifier, resp.StatusCode)
	}

	var actor Actor
	if err := json.NewDecoder(resp.Body).Decode(&actor); err != nil {
		return nil, err
	}
	return &actor, nil
}

func SearchActorsTypeahead(ctx context.Context, query string, limit int) ([]Actor, error) {
	if limit <= 0 {
		limit = 5
	}
	apiURL := fmt.Sprintf("https://public.api.bsky.app/xrpc/app.bsky.actor.searchActorsTypeahead?q=%s&limit=%d", url.QueryEscape(query), limit)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := profileClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("search actors typeahead: status %d", resp.StatusCode)
	}

	var result struct {
		Actors []Actor `json:"actors"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result.Actors, nil
}
