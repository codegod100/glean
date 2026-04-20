package atproto

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/bluesky-social/indigo/atproto/atclient"
	"github.com/bluesky-social/indigo/atproto/syntax"
)

type Client struct {
	httpClient  *http.Client
	pdsURL      string
	accessToken string
	APIClient   *atclient.APIClient
}

func NewClient(pdsURL, accessToken string) *Client {
	return &Client{
		httpClient:  &http.Client{},
		pdsURL:      pdsURL,
		accessToken: accessToken,
	}
}

func (c *Client) CreateRecord(ctx context.Context, did, collection string, record any) (string, string, error) {
	if c.APIClient != nil {
		return c.createRecordWithAPI(ctx, did, collection, record)
	}

	nsid, err := syntax.ParseNSID(collection)
	if err != nil {
		return "", "", fmt.Errorf("parsing collection NSID: %w", err)
	}

	body := map[string]any{
		"repo":       did,
		"collection": nsid.String(),
		"record":     record,
	}

	data, err := json.Marshal(body)
	if err != nil {
		return "", "", err
	}

	url := c.pdsURL + "/xrpc/com.atproto.repo.createRecord"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.accessToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("create record returned %d", resp.StatusCode)
	}

	var result struct {
		URI string `json:"uri"`
		CID string `json:"cid"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", "", err
	}

	return result.URI, result.CID, nil
}

func (c *Client) createRecordWithAPI(ctx context.Context, did, collection string, record any) (string, string, error) {
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

	if err := c.APIClient.Post(ctx, nsid, input, &out); err != nil {
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

	if c.APIClient != nil {
		nsid, err := syntax.ParseNSID("com.atproto.repo.deleteRecord")
		if err != nil {
			return fmt.Errorf("parsing NSID: %w", err)
		}
		return c.APIClient.Post(ctx, nsid, input, nil)
	}

	data, err := json.Marshal(input)
	if err != nil {
		return err
	}

	url := c.pdsURL + "/xrpc/com.atproto.repo.deleteRecord"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.accessToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("delete record returned %d", resp.StatusCode)
	}

	return nil
}

func (c *Client) ListRecords(ctx context.Context, did, collection string, limit int, cursor string) ([]Record, string, error) {
	if c.APIClient != nil {
		return c.listRecordsWithAPI(ctx, did, collection, limit, cursor)
	}

	url := fmt.Sprintf("%s/xrpc/com.atproto.repo.listRecords?repo=%s&collection=%s", c.pdsURL, did, collection)
	if limit > 0 {
		url += fmt.Sprintf("&limit=%d", limit)
	}
	if cursor != "" {
		url += "&cursor=" + cursor
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.accessToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("list records returned %d", resp.StatusCode)
	}

	var result struct {
		Records []struct {
			URI   string          `json:"uri"`
			CID   string          `json:"cid"`
			Value json.RawMessage `json:"value"`
		} `json:"records"`
		Cursor string `json:"cursor"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
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

func (c *Client) listRecordsWithAPI(ctx context.Context, did, collection string, limit int, cursor string) ([]Record, string, error) {
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

	if err := c.APIClient.Get(ctx, nsid, params, &result); err != nil {
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
