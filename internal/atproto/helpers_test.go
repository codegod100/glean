package atproto

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseIntParam(t *testing.T) {
	cases := []struct {
		query      string
		defaultVal int
		maxVal     int
		want       int
	}{
		{"", 10, 100, 10},            // missing -> default
		{"?limit=5", 10, 100, 5},     // valid
		{"?limit=0", 10, 100, 10},    // <1 -> default
		{"?limit=-3", 10, 100, 10},   // negative -> default
		{"?limit=abc", 10, 100, 10},  // non-numeric -> default
		{"?limit=500", 10, 100, 100}, // capped to max
		{"?limit=100", 10, 100, 100}, // exactly max
	}
	for _, c := range cases {
		t.Run(c.query, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/"+c.query, nil)
			if got := parseIntParam(req, "limit", c.defaultVal, c.maxVal); got != c.want {
				t.Fatalf("parseIntParam got %d, want %d", got, c.want)
			}
		})
	}
}

func TestFirstContributor(t *testing.T) {
	cases := []struct {
		name string
		raw  json.RawMessage
		want string
	}{
		{"empty", nil, ""},
		{"valid first", json.RawMessage(`[{"displayName":"Alice"},{"displayName":"Bob"}]`), "Alice"},
		{"empty display name", json.RawMessage(`[{"displayName":""}]`), ""},
		{"empty array", json.RawMessage(`[]`), ""},
		{"invalid json", json.RawMessage(`{`), ""},
		{"unexpected shape", json.RawMessage(`{"foo":"bar"}`), ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := firstContributor(c.raw); got != c.want {
				t.Fatalf("got %q, want %q", got, c.want)
			}
		})
	}
}
