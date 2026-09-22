package feed

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestRetryBackoff_Exponential(t *testing.T) {
	cases := []struct {
		attempt int
		want    time.Duration
	}{
		{1, 1 * time.Second},
		{2, 2 * time.Second},
		{3, 4 * time.Second},
		{4, 8 * time.Second},
	}
	for _, c := range cases {
		if got := retryBackoff(c.attempt, nil); got != c.want {
			t.Fatalf("retryBackoff(%d) = %v, want %v", c.attempt, got, c.want)
		}
	}
}

func TestRetryBackoff_RetryAfterHeaderCapped(t *testing.T) {
	resp := &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Header:     http.Header{"Retry-After": {"3"}},
	}
	if got := retryBackoff(1, resp); got != 3*time.Second {
		t.Fatalf("got %v, want 3s", got)
	}

	// Retry-After beyond the 10s cap is clamped.
	resp.Header.Set("Retry-After", "60")
	if got := retryBackoff(1, resp); got != 10*time.Second {
		t.Fatalf("got %v, want 10s (cap)", got)
	}
}

func TestRetryBackoff_RetryAfterIgnoredForNon429(t *testing.T) {
	resp := &http.Response{
		StatusCode: http.StatusInternalServerError,
		Header:     http.Header{"Retry-After": {"3"}},
	}
	// Not a 429, so falls through to exponential backoff.
	if got := retryBackoff(1, resp); got != 1*time.Second {
		t.Fatalf("got %v, want 1s", got)
	}
}

// The tests above exercise retryBackoff in isolation, with a hand-built
// response. That is how the retry policy came to be wrong while its unit
// tests passed: nothing checked that retryBackoff was ever reached with a
// non-nil response, or that Fetch consulted the status at all.
//
// These drive Fetch against a real server instead, and assert request counts.

func newTestFetcher() *Fetcher {
	return &Fetcher{httpClient: &http.Client{Timeout: 5 * time.Second}}
}

const testFeed = `<?xml version="1.0"?><rss version="2.0"><channel>` +
	`<title>T</title><link>https://x.test</link>` +
	`<item><title>One</title><link>https://x.test/1</link></item>` +
	`</channel></rss>`

func TestFetch_DoesNotRetryPermanentStatus(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		http.Error(w, "nope", http.StatusNotFound)
	}))
	defer srv.Close()

	if _, err := newTestFetcher().Fetch(t.Context(), srv.URL); err == nil {
		t.Fatal("expected an error for a 404")
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("a 404 was requested %d times, want 1", got)
	}
}

func TestFetch_RetriesTransientStatus(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) < 3 {
			http.Error(w, "later", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(testFeed))
	}))
	defer srv.Close()

	result, err := newTestFetcher().Fetch(t.Context(), srv.URL)
	if err != nil {
		t.Fatalf("expected success after retries: %v", err)
	}
	if len(result.Articles) != 1 {
		t.Fatalf("got %d articles, want 1", len(result.Articles))
	}
	if got := atomic.LoadInt32(&calls); got != 3 {
		t.Fatalf("made %d requests, want 3", got)
	}
}

func TestFetch_DoesNotRetryUnparseableBody(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		_, _ = w.Write([]byte("<html><body>not a feed</body></html>"))
	}))
	defer srv.Close()

	// The bytes will be identical on a second read, so one attempt is enough.
	if _, err := newTestFetcher().Fetch(t.Context(), srv.URL); err == nil {
		t.Fatal("expected an error for a non-feed body")
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("an unparseable body was requested %d times, want 1", got)
	}
}

func TestFetch_ReportsStatusRatherThanParseFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "gone", http.StatusGone)
	}))
	defer srv.Close()

	_, err := newTestFetcher().Fetch(t.Context(), srv.URL)
	if err == nil {
		t.Fatal("expected an error")
	}
	// Before the fix an error page was handed to the parser, so a 410 was
	// reported as a parse failure and the real cause never reached the logs.
	if !strings.Contains(err.Error(), "410") {
		t.Fatalf("error should name the status, got: %v", err)
	}
}
