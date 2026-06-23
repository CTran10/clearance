package metrics

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRegistryRendersCountersInPrometheusTextFormat(t *testing.T) {
	t.Parallel()

	registry := NewRegistry()
	registry.Inc("clearance_events_published_total", Labels{
		"topic":  "transactions.created",
		"result": "ok",
	})
	registry.Inc("clearance_events_published_total", Labels{
		"topic":  "transactions.created",
		"result": "ok",
	})

	response := httptest.NewRecorder()
	registry.Handler().ServeHTTP(response, httptest.NewRequest("GET", "/metrics", nil))

	if response.Code != 200 {
		t.Fatalf("status = %d, want 200", response.Code)
	}
	if got := response.Header().Get("Content-Type"); got != "text/plain; version=0.0.4" {
		t.Fatalf("content-type = %q, want prometheus text", got)
	}
	want := `clearance_events_published_total{result="ok",topic="transactions.created"} 2`
	if !strings.Contains(response.Body.String(), want) {
		t.Fatalf("metrics body = %q, want %q", response.Body.String(), want)
	}
}

func TestRegistryEscapesLabelValues(t *testing.T) {
	t.Parallel()

	registry := NewRegistry()
	registry.Inc("clearance_http_requests_total", Labels{"path": `/bad"path`})

	response := httptest.NewRecorder()
	registry.Handler().ServeHTTP(response, httptest.NewRequest("GET", "/metrics", nil))

	if !strings.Contains(response.Body.String(), `path="/bad\"path"`) {
		t.Fatalf("metrics body did not escape label: %q", response.Body.String())
	}
}
