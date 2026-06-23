package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTransactionRouterDoesNotExposeMetricsByDefault(t *testing.T) {
	t.Parallel()

	handler := NewRouter(
		transactionService(newTransactionMemoryStore()),
		newMemoryRateLimiter(10),
		Config{AuthValue: testAuthValue()},
	)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNotFound)
	}
}

func TestTransactionRouterExposesMetricsWhenEnabled(t *testing.T) {
	t.Parallel()

	handler := NewRouter(
		transactionService(newTransactionMemoryStore()),
		newMemoryRateLimiter(10),
		Config{AuthValue: testAuthValue(), MetricsEnabled: true},
	)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if got := response.Header().Get("Content-Type"); got != "text/plain; version=0.0.4" {
		t.Fatalf("content-type = %q, want prometheus text", got)
	}
}

func TestTransactionRouterCountsRequestsByStatus(t *testing.T) {
	t.Parallel()

	handler := NewRouter(
		transactionService(newTransactionMemoryStore()),
		newMemoryRateLimiter(10),
		Config{AuthValue: testAuthValue(), MetricsEnabled: true},
	)
	request := httptest.NewRequest(http.MethodPost, "/transactions", strings.NewReader(`{}`))

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	metricsResponse := httptest.NewRecorder()
	handler.ServeHTTP(metricsResponse, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	want := `clearance_http_requests_total{method="POST",path="/transactions",status="401"}`
	if !strings.Contains(metricsResponse.Body.String(), want) {
		t.Fatalf("metrics body = %q, want counter %q", metricsResponse.Body.String(), want)
	}
}

func TestTransactionRouterCollapsesUnknownMetricPaths(t *testing.T) {
	t.Parallel()

	handler := NewRouter(
		transactionService(newTransactionMemoryStore()),
		newMemoryRateLimiter(10),
		Config{AuthValue: testAuthValue(), MetricsEnabled: true},
	)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/random/scan/path", nil))

	metricsResponse := httptest.NewRecorder()
	handler.ServeHTTP(metricsResponse, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	if strings.Contains(metricsResponse.Body.String(), "/random/scan/path") {
		t.Fatalf("metrics body should not expose raw unknown path: %q", metricsResponse.Body.String())
	}
	want := `clearance_http_requests_total{method="GET",path="/unknown",status="404"}`
	if !strings.Contains(metricsResponse.Body.String(), want) {
		t.Fatalf("metrics body = %q, want counter %q", metricsResponse.Body.String(), want)
	}
}
