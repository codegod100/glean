package atproto

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"gotest.tools/v3/assert"
)

func TestFetchProfile_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, r.URL.Query().Get("actor"), "did:plc:test123")
		json.NewEncoder(w).Encode(map[string]string{
			"handle":      "test.bsky.social",
			"displayName": "Test User",
			"avatar":      "https://cdn.bsky.app/img/avatar/test.png",
		})
	}))
	defer srv.Close()

	origClient := profileClient
	profileClient = srv.Client()
	defer func() { profileClient = origClient }()

	apiURL := srv.URL + "/xrpc/app.bsky.actor.getProfile?actor=" + url.QueryEscape("did:plc:test123")
	handle, dn, avatar, err := fetchProfileFromURL(context.Background(), apiURL, "did:plc:test123")
	assert.NilError(t, err)
	assert.Equal(t, handle, "test.bsky.social")
	assert.Equal(t, dn, "Test User")
	assert.Equal(t, avatar, "https://cdn.bsky.app/img/avatar/test.png")
}

func TestFetchProfile_EmptyProfile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{})
	}))
	defer srv.Close()

	origClient := profileClient
	profileClient = srv.Client()
	defer func() { profileClient = origClient }()

	handle, dn, avatar, err := fetchProfileFromURL(context.Background(), srv.URL+"/xrpc/app.bsky.actor.getProfile", "did:plc:test123")
	assert.NilError(t, err)
	assert.Equal(t, handle, "")
	assert.Equal(t, dn, "")
	assert.Equal(t, avatar, "")
}

func TestFetchProfile_Non200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	origClient := profileClient
	profileClient = srv.Client()
	defer func() { profileClient = origClient }()

	_, _, _, err := fetchProfileFromURL(context.Background(), srv.URL+"/xrpc/app.bsky.actor.getProfile", "did:plc:test123")
	assert.Assert(t, err != nil)
}
