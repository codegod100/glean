package httpclient

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"
)

const (
	UserAgent = "Glean/1.0 (RSS Reader)"

	AcceptFeed = "application/xml,application/atom+xml,application/rss+xml,application/rdf+xml,application/feed+json,text/html;q=0.9"
	AcceptHTML = "text/html,application/xhtml+xml;q=0.9"
)

func NewTransport() *http.Transport {
	return &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 15 * time.Second,
		}).DialContext,
		MaxIdleConns:        50,
		IdleConnTimeout:     10 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
		ForceAttemptHTTP2:   true,
	}
}

func SetDefaultHeaders(req *http.Request) {
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Accept-Encoding", "br,gzip")
	req.Header.Set("Connection", "close")
}

func ParseRetryAfter(v string) time.Duration {
	if d, err := strconv.Atoi(v); err == nil {
		return time.Duration(d) * time.Second
	}
	if t, err := http.ParseTime(v); err == nil {
		return time.Until(t)
	}
	return 0
}

func SleepWithContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

type StatusError struct {
	StatusCode int
	RetryAfter time.Duration
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("HTTP %d", e.StatusCode)
}

func IsRetryable(statusCode int) bool {
	return statusCode == http.StatusTooManyRequests || statusCode >= 500
}
