package risk

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"github.com/CTran10/clearance/internal/domain"
)

func TestServicePersistsOneRiskOutboxEventForDuplicateDelivery(t *testing.T) {
	t.Parallel()

	store := newMemoryStore()
	service := NewService(store)
	payload := transactionPayload(t, domain.Transaction{
		ID:            "txn_123",
		AccountID:     "acct_123",
		AmountCents:   12_550,
		Currency:      "USD",
		CorrelationID: "trace_123",
	})

	for range 2 {
		if err := service.HandleTransactionCreated(context.Background(), "evt_created", payload); err != nil {
			t.Fatalf("HandleTransactionCreated returned error: %v", err)
		}
	}

	events := store.eventsSnapshot()
	if len(events) != 1 {
		t.Fatalf("outbox event count = %d, want 1", len(events))
	}
	if events[0].Type != domain.EventRiskEvaluated {
		t.Fatalf("event type = %q, want %q", events[0].Type, domain.EventRiskEvaluated)
	}
	if events[0].AggregateID != "txn_123" || events[0].PartitionKey != "acct_123" {
		t.Fatalf("outbox routing = aggregate %q partition %q", events[0].AggregateID, events[0].PartitionKey)
	}

	var evaluated domain.RiskEvaluated
	if err := json.Unmarshal(events[0].Payload, &evaluated); err != nil {
		t.Fatalf("decode risk payload: %v", err)
	}
	if !evaluated.Approved || evaluated.RiskLevel != domain.RiskLow {
		t.Fatalf("evaluation = %#v, want approved LOW", evaluated)
	}
}

func TestServiceRejectsEventIDReusedWithDifferentPayload(t *testing.T) {
	t.Parallel()

	store := newMemoryStore()
	service := NewService(store)
	first := transactionPayload(t, domain.Transaction{
		ID: "txn_123", AccountID: "acct_123", AmountCents: 12_550, Currency: "USD", CorrelationID: "trace_123",
	})
	changed := transactionPayload(t, domain.Transaction{
		ID: "txn_123", AccountID: "acct_123", AmountCents: 12_551, Currency: "USD", CorrelationID: "trace_123",
	})

	if err := service.HandleTransactionCreated(context.Background(), "evt_created", first); err != nil {
		t.Fatalf("first delivery returned error: %v", err)
	}
	if err := service.HandleTransactionCreated(context.Background(), "evt_created", changed); !errors.Is(err, domain.ErrEventIdentityConflict) {
		t.Fatalf("changed delivery error = %v, want ErrEventIdentityConflict", err)
	}
	if got := len(store.eventsSnapshot()); got != 1 {
		t.Fatalf("outbox event count = %d, want 1", got)
	}
}

func TestServiceDoesNotPersistMalformedPayload(t *testing.T) {
	t.Parallel()

	store := newMemoryStore()
	service := NewService(store)

	if err := service.HandleTransactionCreated(context.Background(), "evt_bad", []byte(`{"id":`)); err == nil {
		t.Fatal("malformed payload should return an error")
	}
	if got := len(store.eventsSnapshot()); got != 0 {
		t.Fatalf("outbox event count = %d, want 0", got)
	}
}

func transactionPayload(t *testing.T, transaction domain.Transaction) []byte {
	t.Helper()
	payload, err := json.Marshal(transaction)
	if err != nil {
		t.Fatalf("marshal transaction: %v", err)
	}
	return payload
}

type memoryStore struct {
	mu        sync.Mutex
	processed map[string]string
	events    []domain.OutboxEvent
}

func newMemoryStore() *memoryStore {
	return &memoryStore{processed: make(map[string]string)}
}

func (s *memoryStore) SaveConsumerOutbox(
	_ context.Context,
	_ string,
	eventID string,
	payloadHash string,
	event domain.OutboxEvent,
) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if existing, ok := s.processed[eventID]; ok {
		if existing != payloadHash {
			return false, domain.ErrEventIdentityConflict
		}
		return false, nil
	}
	s.processed[eventID] = payloadHash
	s.events = append(s.events, event)
	return true, nil
}

func (s *memoryStore) eventsSnapshot() []domain.OutboxEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]domain.OutboxEvent(nil), s.events...)
}
