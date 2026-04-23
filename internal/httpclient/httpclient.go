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

var dnsResolver = &net.Resolver{
	PreferGo: true,
	Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
		d := net.Dialer{Timeout: 3 * time.Second}
		return d.DialContext(ctx, "udp", "1.1.1.1:53")
	},
}

func NewTransport() *http.Transport {
	dialer := &net.Dialer{
		Timeout:       5 * time.Second,
		KeepAlive:     15 * time.Second,
		FallbackDelay: 300 * time.Millisecond,
		Resolver:      dnsResolver,
	}

	return &http.Transport{
		DialContext:         dialer.DialContext,
		MaxIdleConns:        50,
		IdleConnTimeout:     10 * time.Second,
		TLSHandshakeTimeout: 5 * time.Second,
		ForceAttemptHTTP2:   true,
	}
}

func SetDefaultHeaders(req *http.Request) {
	req.Header.Set("User-Agent", UserAgent)
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
