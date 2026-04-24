package server

import (
	"bytes"
	"fmt"
	"net/http"
	"sort"
	"strings"

	io_prometheus_client "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
)

type metricFamily struct {
	Name        string
	Type        string
	Description string
	Labels      map[string]string
	Value       float64
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	metrics, err := s.fetchMetrics()
	if err != nil {
		s.logger.Warn("failed to fetch metrics", "error", err)
		http.Error(w, "Failed to load metrics", http.StatusInternalServerError)
		return
	}

	user := currentUser(r)
	s.render(w, r, "stats.html", map[string]any{
		"User":    user,
		"Metrics": metrics,
	})
}

func (s *Server) fetchMetrics() (map[string][]metricFamily, error) {
	req, err := http.NewRequest(http.MethodGet, "/metrics", nil)
	if err != nil {
		return nil, err
	}

	rr := &responseRecorder{
		headerMap: make(http.Header),
	}
	s.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		return nil, fmt.Errorf("metrics endpoint returned status %d", rr.Code)
	}

	return parseMetrics(rr.body.Bytes())
}

type responseRecorder struct {
	Code      int
	body      bytes.Buffer
	headerMap http.Header
}

func (r *responseRecorder) Header() http.Header         { return r.headerMap }
func (r *responseRecorder) Write(b []byte) (int, error) { return r.body.Write(b) }
func (r *responseRecorder) WriteHeader(code int)        { r.Code = code }

func parseMetrics(data []byte) (map[string][]metricFamily, error) {
	var parser expfmt.TextParser
	families, err := parser.TextToMetricFamilies(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}

	result := make(map[string][]metricFamily)
	for name, family := range families {
		category := categorizeMetric(name)

		metricType := strings.ToLower(family.Type.String())
		description := family.GetHelp()

		for _, m := range family.Metric {
			mf := metricFamily{
				Name:        name,
				Type:        metricType,
				Description: description,
				Labels:      make(map[string]string),
				Value:       getValue(m),
			}

			for _, label := range m.GetLabel() {
				mf.Labels[label.GetName()] = label.GetValue()
			}

			if len(m.GetLabel()) == 0 {
				mf.Labels = nil
			}

			result[category] = append(result[category], mf)
		}
	}

	for _, metrics := range result {
		sort.Slice(metrics, func(i, j int) bool {
			return metrics[i].Name < metrics[j].Name
		})
	}

	return result, nil
}

func getValue(m *io_prometheus_client.Metric) float64 {
	if m.Counter != nil {
		return m.Counter.GetValue()
	}
	if m.Gauge != nil {
		return m.Gauge.GetValue()
	}
	if m.Histogram != nil {
		return float64(m.Histogram.GetSampleCount())
	}
	if m.Summary != nil {
		return float64(m.Summary.GetSampleCount())
	}
	if m.Untyped != nil {
		return m.Untyped.GetValue()
	}
	return 0
}

func categorizeMetric(name string) string {
	if strings.HasPrefix(name, "glean_feed") {
		return "Feeds"
	}
	if strings.HasPrefix(name, "glean_article") {
		return "Articles"
	}
	if strings.Contains(name, "jetstream") {
		return "Jetstream"
	}
	if strings.HasPrefix(name, "atproto") {
		return "ATProto"
	}
	if strings.HasPrefix(name, "glean_http") {
		return "HTTP"
	}
	if strings.HasPrefix(name, "glean_user") {
		return "Users"
	}
	if strings.HasPrefix(name, "glean_cluster") {
		return "Cluster"
	}
	if strings.HasPrefix(name, "glean_pds_sync") {
		return "PDS Sync"
	}
	return "Other"
}
