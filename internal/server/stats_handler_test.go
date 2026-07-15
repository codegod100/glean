package server

import (
	"testing"

	io_prometheus_client "github.com/prometheus/client_model/go"
)

func TestCategorizeMetric(t *testing.T) {
	cases := []struct {
		name string
		want string
	}{
		{"glean_feed_total", "Feeds"},
		{"glean_article_count", "Articles"},
		{"atproto_jetstream_events", "Jetstream"},
		{"atproto_sync_records", "ATProto"},
		{"glean_http_requests", "HTTP"},
		{"glean_user_total", "Users"},
		{"glean_cluster_size", "Cluster"},
		{"glean_pds_sync_ok", "PDS Sync"},
		{"something_else", "Other"},
		{"", "Other"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := categorizeMetric(tc.name); got != tc.want {
				t.Fatalf("categorizeMetric(%q) = %q, want %q", tc.name, got, tc.want)
			}
		})
	}
}

func TestCategorizeMetric_PrefixPriority(t *testing.T) {
	// "atproto_jetstream_*" should be categorized as Jetstream before ATProto.
	if got := categorizeMetric("atproto_jetstream_lag"); got != "Jetstream" {
		t.Fatalf("got %q, want Jetstream", got)
	}
}

func TestGetValue(t *testing.T) {
	counter := 42.0
	gauge := 7.5
	t.Run("counter", func(t *testing.T) {
		m := &io_prometheus_client.Metric{Counter: &io_prometheus_client.Counter{Value: &counter}}
		if got := getValue(m); got != counter {
			t.Fatalf("got %v, want %v", got, counter)
		}
	})
	t.Run("gauge", func(t *testing.T) {
		m := &io_prometheus_client.Metric{Gauge: &io_prometheus_client.Gauge{Value: &gauge}}
		if got := getValue(m); got != gauge {
			t.Fatalf("got %v, want %v", got, gauge)
		}
	})
	t.Run("histogram uses sample count", func(t *testing.T) {
		var sc uint64 = 99
		m := &io_prometheus_client.Metric{Histogram: &io_prometheus_client.Histogram{SampleCount: &sc}}
		if got := getValue(m); got != 99 {
			t.Fatalf("got %v, want 99", got)
		}
	})
	t.Run("summary uses sample count", func(t *testing.T) {
		var sc uint64 = 5
		m := &io_prometheus_client.Metric{Summary: &io_prometheus_client.Summary{SampleCount: &sc}}
		if got := getValue(m); got != 5 {
			t.Fatalf("got %v, want 5", got)
		}
	})
	t.Run("untyped", func(t *testing.T) {
		m := &io_prometheus_client.Metric{Untyped: &io_prometheus_client.Untyped{Value: &gauge}}
		if got := getValue(m); got != gauge {
			t.Fatalf("got %v, want %v", got, gauge)
		}
	})
	t.Run("empty returns zero", func(t *testing.T) {
		if got := getValue(&io_prometheus_client.Metric{}); got != 0 {
			t.Fatalf("got %v, want 0", got)
		}
	})
}

func TestParseMetrics(t *testing.T) {
	// Minimal Prometheus text format payload covering a counter and a labeled gauge.
	data := []byte(`# HELP glean_feed_total Total feeds.
# TYPE glean_feed_total counter
glean_feed_total 10
# HELP glean_http_requests HTTP requests.
# TYPE glean_http_requests gauge
glean_http_requests{code="200"} 3
`)
	got, err := parseMetrics(data)
	if err != nil {
		t.Fatalf("parseMetrics error: %v", err)
	}
	if _, ok := got["Feeds"]; !ok {
		t.Fatalf("missing Feeds category, got: %v", got)
	}
	if _, ok := got["HTTP"]; !ok {
		t.Fatalf("missing HTTP category, got: %v", got)
	}
	if len(got["Feeds"]) != 1 || got["Feeds"][0].Value != 10 {
		t.Fatalf("unexpected Feeds entry: %+v", got["Feeds"])
	}
	httpEntry := got["HTTP"][0]
	if httpEntry.Value != 3 {
		t.Fatalf("unexpected HTTP value: %v", httpEntry.Value)
	}
	if httpEntry.Labels["code"] != "200" {
		t.Fatalf("unexpected HTTP labels: %v", httpEntry.Labels)
	}
}

func TestParseMetrics_Invalid(t *testing.T) {
	if _, err := parseMetrics([]byte("not valid prometheus {{{")); err == nil {
		t.Fatal("expected error for invalid input")
	}
}
