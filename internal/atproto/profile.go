package atproto

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

var profileClient = &http.Client{Timeout: 10 * time.Second}

// FetchProfile is an unauthenticated profile fetcher. It relies on bluesky api.
func FetchProfile(ctx context.Context, identifier string) (handle, displayName, avatarURL string, err error) {
	apiURL := fmt.Sprintf("https://public.api.bsky.app/xrpc/app.bsky.actor.getProfile?actor=%s", url.QueryEscape(identifier))
	return fetchProfileFromURL(ctx, apiURL, identifier)
}

func fetchProfileFromURL(ctx context.Context, apiURL, identifier string) (handle, displayName, avatarURL string, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return "", "", "", err
	}

	resp, err := profileClient.Do(req)
	if err != nil {
		return "", "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", "", fmt.Errorf("fetch profile %s: status %d", identifier, resp.StatusCode)
	}

	var profile struct {
		Handle      string `json:"handle"`
		DisplayName string `json:"displayName"`
		Avatar      string `json:"avatar"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&profile); err != nil {
		return "", "", "", err
	}
	return profile.Handle, profile.DisplayName, profile.Avatar, nil
}
