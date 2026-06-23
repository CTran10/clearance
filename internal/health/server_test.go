package health

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandlerDoesNotExposeMetricsByDefault(t *testing.T) {
	t.Parallel()

	response := httptest.NewRecorder()
	handler(false).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNotFound)
	}
}

func TestHandlerExposesMetricsWhenEnabled(t *testing.T) {
	t.Parallel()

	response := httptest.NewRecorder()
	handler(true).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
}
