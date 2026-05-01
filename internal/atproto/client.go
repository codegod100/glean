package atproto

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/bluesky-social/indigo/atproto/atclient"
	"github.com/bluesky-social/indigo/atproto/syntax"
)

type Client struct {
	api *atclient.APIClient
}

func NewClient(api *atclient.APIClient) *Client {
	return &Client{api: api}
}

func NewUnauthenticatedClient(pdsURL string) *Client {
	return &Client{api: atclient.NewAPIClient(pdsURL)}
}

func (c *Client) CreateRecord(ctx context.Context, did, collection string, record any) (string, string, error) {
	input := map[string]any{
		"repo":       did,
		"collection": collection,
		"record":     record,
	}

	var out struct {
		URI string `json:"uri"`
		CID string `json:"cid"`
	}

	nsid, err := syntax.ParseNSID("com.atproto.repo.createRecord")
	if err != nil {
		return "", "", fmt.Errorf("parsing NSID: %w", err)
	}

	if err := c.api.Post(ctx, nsid, input, &out); err != nil {
		return "", "", err
	}
	return out.URI, out.CID, nil
}

func (c *Client) DeleteRecord(ctx context.Context, did, collection, rkey string) error {
	input := map[string]any{
		"repo":       did,
		"collection": collection,
		"rkey":       rkey,
	}

	nsid, err := syntax.ParseNSID("com.atproto.repo.deleteRecord")
	if err != nil {
		return fmt.Errorf("parsing NSID: %w", err)
	}

	return c.api.Post(ctx, nsid, input, nil)
}

func (c *Client) ListRecords(ctx context.Context, did, collection string, limit int, cursor string) ([]Record, string, error) {
	nsid, err := syntax.ParseNSID("com.atproto.repo.listRecords")
	if err != nil {
		return nil, "", fmt.Errorf("parsing NSID: %w", err)
	}

	params := map[string]any{
		"repo":       did,
		"collection": collection,
	}
	if limit > 0 {
		params["limit"] = limit
	}
	if cursor != "" {
		params["cursor"] = cursor
	}

	var result struct {
		Records []struct {
			URI   string          `json:"uri"`
			CID   string          `json:"cid"`
			Value json.RawMessage `json:"value"`
		} `json:"records"`
		Cursor string `json:"cursor"`
	}

	if err := c.api.Get(ctx, nsid, params, &result); err != nil {
		return nil, "", err
	}

	records := make([]Record, len(result.Records))
	for i, r := range result.Records {
		records[i] = Record{
			URI:   r.URI,
			CID:   r.CID,
			Value: r.Value,
		}
	}
	return records, result.Cursor, nil
}

func (c *Client) GetRecord(ctx context.Context, did, collection, rkey string) (json.RawMessage, error) {
	nsid, err := syntax.ParseNSID("com.atproto.repo.getRecord")
	if err != nil {
		return nil, fmt.Errorf("parsing NSID: %w", err)
	}

	params := map[string]any{
		"repo":       did,
		"collection": collection,
		"rkey":       rkey,
	}

	var result struct {
		Value json.RawMessage `json:"value"`
	}
	if err := c.api.Get(ctx, nsid, params, &result); err != nil {
		return nil, err
	}
	return result.Value, nil
}

func (c *Client) GetProfile(ctx context.Context, did string) (displayName, avatarURL string, err error) {
	nsid, err := syntax.ParseNSID("app.bsky.actor.getProfile")
	if err != nil {
		return "", "", fmt.Errorf("parsing NSID: %w", err)
	}

	var profile struct {
		DisplayName string `json:"displayName"`
		Avatar      string `json:"avatar"`
	}
	if err := c.api.Get(ctx, nsid, map[string]any{"actor": did}, &profile); err != nil {
		return "", "", err
	}
	return profile.DisplayName, profile.Avatar, nil
}
