package metrics

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRegistryExposesTypedApplicationAndRuntimeMetrics(t *testing.T) {
	t.Parallel()

	registry := NewRegistry("test-service")
	registry.ObserveHTTPRequest("GET", "/transactions/{id}", "200", 75*time.Millisecond)
	registry.SetOperationalSnapshot(OperationalSnapshot{
		OutboxPending: 3, OutboxDeadLettered: 1, OutboxOldestPendingAgeSeconds: 12,
		DeadLettersOpen: 2, ProcessedEvents: 9,
		PostgresPoolOpen: 4, PostgresPoolIdle: 3, PostgresPoolInUse: 1,
	})

	response := httptest.NewRecorder()
	registry.Handler().ServeHTTP(response, httptest.NewRequest("GET", "/metrics", nil))
	body := response.Body.String()

	for _, want := range []string{
		`# TYPE clearance_http_requests_total counter`,
		`clearance_http_requests_total{method="GET",path="/transactions/{id}",service="test-service",status="200"} 1`,
		`# TYPE clearance_http_request_duration_seconds histogram`,
		`clearance_http_request_duration_seconds_bucket{method="GET",path="/transactions/{id}",service="test-service",le="0.1"} 1`,
		`clearance_outbox_events{service="test-service",status="PENDING"} 3`,
		`clearance_dead_letters_open{service="test-service"} 2`,
		`clearance_postgres_pool_connections{service="test-service",state="in_use"} 1`,
		`go_goroutines`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("metrics body missing %q:\n%s", want, body)
		}
	}
}

func TestRegistryTracksConsumerOutcomesAndRejectsRawCardinality(t *testing.T) {
	t.Parallel()

	registry := NewRegistry("risk-service")
	registry.ObserveConsumerMessage("risk-service", "transactions.created", "processed", 20*time.Millisecond)
	registry.ObserveConsumerMessage("risk-service", "transactions.created", "dlq", 30*time.Millisecond)
	registry.IncConsumerRetry("risk-service", "transactions.created")
	registry.IncOffsetCommitFailure("risk-service", "transactions.created")

	response := httptest.NewRecorder()
	registry.Handler().ServeHTTP(response, httptest.NewRequest("GET", "/metrics", nil))
	body := response.Body.String()
	for _, want := range []string{
		`clearance_consumer_messages_total{consumer="risk-service",result="processed",service="risk-service",topic="transactions.created"} 1`,
		`clearance_consumer_messages_total{consumer="risk-service",result="dlq",service="risk-service",topic="transactions.created"} 1`,
		`clearance_consumer_retries_total{consumer="risk-service",service="risk-service",topic="transactions.created"} 1`,
		`clearance_consumer_offset_commit_failures_total{consumer="risk-service",service="risk-service",topic="transactions.created"} 1`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("metrics body missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "evt_") || strings.Contains(body, "acct_") || strings.Contains(body, "trace_") {
		t.Fatalf("metrics body contains a high-cardinality identifier: %s", body)
	}
}
