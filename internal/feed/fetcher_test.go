package feed

import (
	"net/http"
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
