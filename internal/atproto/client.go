package atproto

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/bluesky-social/indigo/atproto/syntax"
)

type Client struct {
	httpClient  *http.Client
	pdsURL      string
	accessToken string
}

func NewClient(pdsURL, accessToken string) *Client {
	return &Client{
		httpClient:  &http.Client{},
		pdsURL:      pdsURL,
		accessToken: accessToken,
	}
}

func (c *Client) CreateRecord(ctx context.Context, did, collection string, record any) (string, string, error) {
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

func (c *Client) DeleteRecord(ctx context.Context, did, collection, rkey string) error {
	body := map[string]any{
		"repo":       did,
		"collection": collection,
		"rkey":       rkey,
	}

	data, err := json.Marshal(body)
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
