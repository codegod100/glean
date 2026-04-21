package atproto

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"gotest.tools/v3/assert"
)

func TestFetchSubscriberDIDs(t *testing.T) {
	page1 := struct {
		Repos []struct {
			DID string `json:"did"`
		} `json:"repos"`
		Cursor string `json:"cursor"`
	}{
		Repos: []struct {
			DID string `json:"did"`
		}{
			{DID: "did:plc:aaa"},
			{DID: "did:plc:bbb"},
		},
		Cursor: "nextpage",
	}
	page2 := struct {
		Repos []struct {
			DID string `json:"did"`
		} `json:"repos"`
		Cursor string `json:"cursor"`
	}{
		Repos: []struct {
			DID string `json:"did"`
		}{
			{DID: "did:plc:ccc"},
		},
		Cursor: "",
	}

	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		assert.Equal(t, r.URL.Query().Get("collection"), "at.glean.subscription")

		w.Header().Set("Content-Type", "application/json")
		if callCount == 1 {
			assert.Equal(t, r.URL.Query().Get("cursor"), "")
			json.NewEncoder(w).Encode(page1)
		} else {
			assert.Equal(t, r.URL.Query().Get("cursor"), "nextpage")
			json.NewEncoder(w).Encode(page2)
		}
	}))
	defer server.Close()

	url := server.URL + "/xrpc/com.atproto.sync.listReposByCollection?collection=at.glean.subscription"
	dids, err := FetchSubscriberDIDs(context.Background(), url)
	assert.NilError(t, err)
	assert.Equal(t, callCount, 2)
	assert.DeepEqual(t, dids, []string{"did:plc:aaa", "did:plc:bbb", "did:plc:ccc"})
}

func TestFetchSubscriberDIDs_EmptyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"repos": []any{}})
	}))
	defer server.Close()

	url := server.URL + "/xrpc/com.atproto.sync.listReposByCollection?collection=at.glean.subscription"
	dids, err := FetchSubscriberDIDs(context.Background(), url)
	assert.NilError(t, err)
	assert.Equal(t, len(dids), 0)
}

func TestFetchSubscriberDIDs_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal error"))
	}))
	defer server.Close()

	url := server.URL + "/xrpc/com.atproto.sync.listReposByCollection?collection=at.glean.subscription"
	_, err := FetchSubscriberDIDs(context.Background(), url)
	assert.Assert(t, err != nil)
}

func TestFetchSubscriberDIDs_CancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	url := "http://localhost/xrpc/com.atproto.sync.listReposByCollection?collection=at.glean.subscription"
	_, err := FetchSubscriberDIDs(ctx, url)
	assert.Assert(t, err != nil)
}
