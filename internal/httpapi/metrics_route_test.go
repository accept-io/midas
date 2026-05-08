package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// stubMetricsHandler builds a minimal Prometheus handler over a fresh
// registry so the route test does not depend on internal/metrics. Keeping the
// httpapi package independent of internal/metrics avoids an import cycle if
// the metrics package ever needs to reference httpapi.
func stubMetricsHandler() (http.Handler, *prometheus.Registry) {
	reg := prometheus.NewRegistry()
	reg.MustRegister(prometheus.NewCounter(prometheus.CounterOpts{
		Name: "midas_test_metric_total",
		Help: "stub metric for route test",
	}))
	return promhttp.HandlerFor(reg, promhttp.HandlerOpts{}), reg
}

func TestMetrics_RouteRegisteredAtConfiguredPath(t *testing.T) {
	srv := NewServer(&mockOrchestrator{})
	handler, _ := stubMetricsHandler()
	srv.WithMetrics(handler, "/metrics")

	rec := performRequest(t, srv, http.MethodGet, "/metrics", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("/metrics: want 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "midas_test_metric_total") {
		t.Errorf("/metrics body should contain stub metric name, got:\n%s", rec.Body.String())
	}
}

func TestMetrics_NotRegisteredWhenDisabled(t *testing.T) {
	// No WithMetrics call simulates metrics disabled — route should not exist.
	srv := NewServer(&mockOrchestrator{})

	rec := performRequest(t, srv, http.MethodGet, "/metrics", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("/metrics with metrics disabled: want 404, got %d", rec.Code)
	}
}

func TestMetrics_NonGetReturns405(t *testing.T) {
	srv := NewServer(&mockOrchestrator{})
	handler, _ := stubMetricsHandler()
	srv.WithMetrics(handler, "/metrics")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/metrics", nil)
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST /metrics: want 405, got %d", rec.Code)
	}
	if got := rec.Header().Get("Allow"); got != http.MethodGet {
		t.Errorf("Allow header: want %q, got %q", http.MethodGet, got)
	}
}

func TestMetrics_CustomPath(t *testing.T) {
	srv := NewServer(&mockOrchestrator{})
	handler, _ := stubMetricsHandler()
	srv.WithMetrics(handler, "/internal/metrics")

	rec := performRequest(t, srv, http.MethodGet, "/internal/metrics", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("/internal/metrics: want 200, got %d", rec.Code)
	}

	// /metrics (default) must NOT be registered when a custom path was used.
	rec2 := performRequest(t, srv, http.MethodGet, "/metrics", nil)
	if rec2.Code == http.StatusOK {
		t.Errorf("/metrics should not exist when custom path configured, got 200")
	}
}

func TestMetrics_WithMetricsNilHandler_NoRoute(t *testing.T) {
	srv := NewServer(&mockOrchestrator{})
	srv.WithMetrics(nil, "/metrics")

	rec := performRequest(t, srv, http.MethodGet, "/metrics", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("/metrics with nil handler: want 404, got %d", rec.Code)
	}
}

func TestMetrics_WithMetricsEmptyPath_NoRoute(t *testing.T) {
	srv := NewServer(&mockOrchestrator{})
	handler, _ := stubMetricsHandler()
	srv.WithMetrics(handler, "")

	rec := performRequest(t, srv, http.MethodGet, "/metrics", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("/metrics with empty path: want 404, got %d", rec.Code)
	}
}
