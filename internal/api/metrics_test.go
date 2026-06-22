package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestMetricsHandler_PrometheusDefault(t *testing.T) {
	app := &App{startTime: time.Now().Add(-time.Minute)}
	app.requestCount.Store(7)
	app.errorCount.Store(2)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	app.metricsHandler()(rec, req)

	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Fatalf("default /metrics should be Prometheus text, got Content-Type %q", ct)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"# TYPE engram_http_requests_total counter",
		"engram_http_requests_total 7",
		"engram_http_errors_total 2",
		"engram_uptime_seconds",
		"engram_goroutines",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Prometheus output missing %q\n--- body ---\n%s", want, body)
		}
	}
}

func TestMetricsHandler_JSONOptIn(t *testing.T) {
	app := &App{startTime: time.Now()}

	for _, tc := range []struct {
		name string
		req  *http.Request
	}{
		{"query param", httptest.NewRequest(http.MethodGet, "/metrics?format=json", nil)},
		{"accept header", acceptJSON(httptest.NewRequest(http.MethodGet, "/metrics", nil))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			app.metricsHandler()(rec, tc.req)

			if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
				t.Fatalf("expected JSON content type, got %q", ct)
			}
			var payload map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
				t.Fatalf("body is not valid JSON: %v", err)
			}
			if _, ok := payload["uptime_seconds"]; !ok {
				t.Errorf("JSON metrics missing uptime_seconds: %v", payload)
			}
		})
	}
}

func TestLivenessHandler(t *testing.T) {
	rec := httptest.NewRecorder()
	livenessHandler()(rec, httptest.NewRequest(http.MethodGet, "/livez", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("livez should always be 200, got %d", rec.Code)
	}
}

func acceptJSON(r *http.Request) *http.Request {
	r.Header.Set("Accept", "application/json")
	return r
}
