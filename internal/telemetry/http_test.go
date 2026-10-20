package telemetry

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestMiddlewareSkipsOperatorProbes guards the exclusion that keeps a scraper
// from inflating the totals it reads: /metrics and /healthz must not reach the
// monitor, while a real API call still must.
func TestMiddlewareSkipsOperatorProbes(t *testing.T) {
	monitor := Monitor()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := Middleware(monitor, logger, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for _, path := range []string{"/healthz", "/metrics"} {
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, path, nil))
	}
	if total := monitor.Snapshot().Lifetime.Total; total != 0 {
		t.Fatalf("operator probes were counted: requests_total = %d, want 0", total)
	}

	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil))
	if total := monitor.Snapshot().Lifetime.Total; total != 1 {
		t.Fatalf("a routed request was not counted: requests_total = %d, want 1", total)
	}
}
