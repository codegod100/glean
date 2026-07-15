package server

import (
	"testing"
)

func TestValidateHTTPURL(t *testing.T) {
	valid := []string{
		"http://example.com/feed",
		"https://example.com/feed",
		"https://example.com:8080/path?x=1",
	}
	for _, u := range valid {
		t.Run("valid/"+u, func(t *testing.T) {
			if err := validateHTTPURL(u); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
	invalid := []string{
		"ftp://example.com/feed", // wrong scheme
		"file:///etc/passwd",     // wrong scheme
		"https://",               // no host
		"://malformed",           // unparseable
	}
	for _, u := range invalid {
		t.Run("invalid/"+u, func(t *testing.T) {
			if err := validateHTTPURL(u); err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}

func TestOriginOf(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"not a url", ""},
		{"https://example.com/path", "https://example.com"},
		{"http://example.com:8080/a/b", "http://example.com:8080"},
		{"example.com/path", ""}, // no scheme/host -> empty
		{"https:///nohost", ""},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			if got := originOf(c.in); got != c.want {
				t.Fatalf("originOf(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestBuildNavSuffix(t *testing.T) {
	cases := []struct {
		name    string
		feedURL string
		liked   bool
		status  string
		want    string
	}{
		{"empty", "", false, "", ""},
		{"feed only", "https://a.com/feed", false, "", "?from_feed=https%3A%2F%2Fa.com%2Ffeed"},
		{"liked only", "", true, "", "?liked=1"},
		{"status only", "", false, "unread", "?status=unread"},
		{"all", "https://a.com/feed", true, "unread", "?from_feed=https%3A%2F%2Fa.com%2Ffeed&liked=1&status=unread"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := buildNavSuffix(c.feedURL, c.liked, c.status); got != c.want {
				t.Fatalf("buildNavSuffix got %q, want %q", got, c.want)
			}
		})
	}
}
