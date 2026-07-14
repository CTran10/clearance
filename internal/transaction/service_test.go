package transaction

import (
	"context"
	"errors"
	"testing"

	"github.com/CTran10/clearance/internal/domain"
)

// writing this test BEFORE the code exists feels backwards but it's the contract i want the new Go service to honor.
// the big new idea moving off the python monolith: the "transactional outbox". instead of saving the txn AND
// firing an event to kafka separately (where the 2nd one can fail and you lose the event forever), you write the
// event into a plain db table in the SAME transaction as the txn. either both land or neither does. a separate
// publisher drains that table later. took me a few diagrams to believe it but it kills the "saved but never published" ghost
func TestServiceCreatesPendingTransactionAndOutboxEvent(t *testing.T) {
	t.Parallel() // go runs t.Parallel() tests concurrently — caught a shared-state bug in my store this way, 10/10 recommend

	store := NewMemoryStore()
	service := NewService(store)
	request := CreateRequest{
		AccountID:   "acct_123",
		MerchantID:  "merchant_123",
		AmountCents: 12_550, // MONEY AS INTEGER CENTS. never floats. 0.1 + 0.2 != 0.3 in float land and that's a lawsuit waiting to happen
		Currency:    "USD",
	}
	metadata := RequestMetadata{IdempotencyKey: "idem-123", CorrelationID: "trace-123"}

	response, err := service.Create(context.Background(), request, metadata)
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	if response.Status != domain.TransactionPending {
		t.Fatalf("status = %q, want %q", response.Status, domain.TransactionPending)
	}
	if response.CorrelationID != metadata.CorrelationID {
		t.Fatal("correlation id should be propagated")
	}

	events := store.OutboxEvents()
	if len(events) != 1 {
		t.Fatalf("outbox event count = %d, want 1", len(events))
	}
	if events[0].Type != domain.EventTransactionCreated {
		t.Fatalf("event type = %q, want %q", events[0].Type, domain.EventTransactionCreated)
	}
	if events[0].CorrelationID != metadata.CorrelationID {
		t.Fatal("outbox event should carry correlation id")
	}
	if events[0].AggregateID != response.TransactionID {
		t.Fatalf("aggregate id = %q, want %q", events[0].AggregateID, response.TransactionID)
	}
	if events[0].PartitionKey != request.AccountID {
		t.Fatalf("partition key = %q, want account id %q", events[0].PartitionKey, request.AccountID)
	}
}

func TestServiceReplaysSameIdempotencyKeyAndRejectsPayloadMismatch(t *testing.T) {
	t.Parallel()

	store := NewMemoryStore()
	service := NewService(store)
	request := CreateRequest{
		AccountID:   "acct_123",
		MerchantID:  "merchant_123",
		AmountCents: 12_550,
		Currency:    "USD",
	}
	metadata := RequestMetadata{IdempotencyKey: "idem-123", CorrelationID: "trace-123"}

	first, err := service.Create(context.Background(), request, metadata)
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	replayed, err := service.Create(context.Background(), request, metadata)
	if err != nil {
		t.Fatalf("Create replay returned error: %v", err)
	}
	if replayed.TransactionID != first.TransactionID {
		t.Fatalf("replayed id = %q, want %q", replayed.TransactionID, first.TransactionID)
	}
	if len(store.OutboxEvents()) != 1 {
		t.Fatal("replayed idempotency key should not create another outbox event")
	}

	changed := request
	changed.AmountCents = 12_551
	if _, err := service.Create(context.Background(), changed, metadata); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("Create changed payload error = %v, want ErrIdempotencyConflict", err)
	}
}

func TestServiceReplaysSamePayloadAfterConcurrentIdempotencyInsert(t *testing.T) {
	t.Parallel()

	request := CreateRequest{
		AccountID:   "acct_123",
		MerchantID:  "merchant_123",
		AmountCents: 12_550,
		Currency:    "USD",
	}
	metadata := RequestMetadata{IdempotencyKey: "idem-123", CorrelationID: "trace-123"}
	existing := IdempotencyRecord{
		Key:         metadata.IdempotencyKey,
		RequestHash: hashRequest(request),
		CreateResult: CreateResponse{
			TransactionID: "txn_existing",
			Status:        domain.TransactionPending,
			CorrelationID: metadata.CorrelationID,
		},
	}
	service := NewService(&concurrentIdempotencyStore{existing: existing})

	response, err := service.Create(context.Background(), request, metadata)
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if response.TransactionID != existing.CreateResult.TransactionID {
		t.Fatalf("transaction_id = %q, want replayed %q", response.TransactionID, existing.CreateResult.TransactionID)
	}
}

func TestServiceValidatesTrustedInput(t *testing.T) {
	t.Parallel()

	service := NewService(NewMemoryStore())

	tests := []struct {
		name     string
		request  CreateRequest
		metadata RequestMetadata
	}{
		{
			name: "missing idempotency key",
			request: CreateRequest{
				AccountID:   "acct_123",
				MerchantID:  "merchant_123",
				AmountCents: 100,
				Currency:    "USD",
			},
			metadata: RequestMetadata{CorrelationID: "trace_123"},
		},
		{
			name: "invalid amount",
			request: CreateRequest{
				AccountID:   "acct_123",
				MerchantID:  "merchant_123",
				AmountCents: 0,
				Currency:    "USD",
			},
			metadata: RequestMetadata{IdempotencyKey: "idem_123", CorrelationID: "trace_123"},
		},
		{
			name: "invalid currency",
			request: CreateRequest{
				AccountID:   "acct_123",
				MerchantID:  "merchant_123",
				AmountCents: 100,
				Currency:    "US1",
			},
			metadata: RequestMetadata{IdempotencyKey: "idem_123", CorrelationID: "trace_123"},
		},
		{
			name: "reserved settlement account",
			request: CreateRequest{
				AccountID:   "external-settlement",
				MerchantID:  "merchant_123",
				AmountCents: 100,
				Currency:    "USD",
			},
			metadata: RequestMetadata{IdempotencyKey: "idem_123", CorrelationID: "trace_123"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if _, err := service.Create(context.Background(), tt.request, tt.metadata); !errors.Is(err, ErrInvalidRequest) {
				t.Fatalf("Create error = %v, want ErrInvalidRequest", err)
			}
		})
	}
}

type concurrentIdempotencyStore struct {
	existing IdempotencyRecord
	lookups  int
}

func (s *concurrentIdempotencyStore) FindIdempotency(context.Context, string) (IdempotencyRecord, bool, error) {
	s.lookups++
	if s.lookups == 1 {
		return IdempotencyRecord{}, false, nil
	}
	return s.existing, true, nil
}

func (s *concurrentIdempotencyStore) Create(context.Context, IdempotencyRecord, domain.OutboxEvent) error {
	return ErrIdempotencyConflict
}
