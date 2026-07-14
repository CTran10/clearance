package metrics

import (
	"context"
	"net/http/httptest"
	"strings"
	"sync/atomic"
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

func TestDefaultRegistryWrappersAndSampler(t *testing.T) {
	Configure("wrapper-service")
	ObserveHTTPRequest("POST", "/transactions", "202", 10*time.Millisecond)
	KafkaPublish("transactions.created", "ok")
	OutboxPublish("published", 5*time.Millisecond)
	ObserveConsumerMessage("risk-service", "transactions.created", "processed", 5*time.Millisecond)
	IncConsumerRetry("risk-service", "transactions.created")
	IncOffsetCommitFailure("risk-service", "transactions.created")
	SetOperationalSnapshot(OperationalSnapshot{DeadLettersOpen: 1})

	ctx, cancel := context.WithCancel(context.Background())
	provider := &snapshotProvider{snapshot: OperationalSnapshot{ProcessedEvents: 7}}
	StartSampler(ctx, time.Hour, provider)
	cancel()
	if provider.calls.Load() != 1 {
		t.Fatalf("sampler calls = %d, want immediate collection", provider.calls.Load())
	}

	response := httptest.NewRecorder()
	Handler().ServeHTTP(response, httptest.NewRequest("GET", "/metrics", nil))
	body := response.Body.String()
	for _, want := range []string{
		`clearance_kafka_messages_published_total{result="ok",service="wrapper-service",topic="transactions.created"} 1`,
		`clearance_outbox_publish_attempts_total{result="published",service="wrapper-service"} 1`,
		`clearance_processed_events{service="wrapper-service"} 7`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("metrics body missing %q", want)
		}
	}
}

type snapshotProvider struct {
	snapshot OperationalSnapshot
	calls    atomic.Int32
}

func (p *snapshotProvider) OperationalMetrics(context.Context) (OperationalSnapshot, error) {
	p.calls.Add(1)
	return p.snapshot, nil
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
