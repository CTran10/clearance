package funding

import (
	"context"
	"errors"
	"testing"

	"github.com/CTran10/clearance/internal/domain"
)

func TestServiceCreatesIdempotentDepositEvent(t *testing.T) {
	t.Parallel()

	store := newMemoryStore()
	service := NewService(store, Config{MaxAmountCents: 1_000_000})
	request := DepositRequest{
		AccountID:         "acct_123",
		AmountCents:       25_000,
		Currency:          "usd",
		FundingSource:     "demo-operator",
		ExternalReference: "bank-transfer-123",
	}
	metadata := RequestMetadata{
		IdempotencyKey: "fund-123",
		CorrelationID:  "trace-123",
		OperatorReason: "seed demo account",
	}

	first, err := service.Deposit(context.Background(), request, metadata)
	if err != nil {
		t.Fatalf("Deposit returned error: %v", err)
	}
	second, err := service.Deposit(context.Background(), request, metadata)
	if err != nil {
		t.Fatalf("Deposit replay returned error: %v", err)
	}
	if first != second {
		t.Fatalf("replay response = %#v, want %#v", second, first)
	}
	if first.DepositID == "" || first.Status != domain.TransactionAuthorized || first.BalanceAfterCents != 25_000 {
		t.Fatalf("response = %#v", first)
	}
	if store.creates != 1 {
		t.Fatalf("create calls = %d, want 1", store.creates)
	}
	if store.deposit.Transaction.Kind != domain.TransactionDeposit || store.deposit.Transaction.MerchantID != "" {
		t.Fatalf("deposit transaction = %#v", store.deposit.Transaction)
	}
	if store.event.Type != domain.EventFundsDeposited || store.event.PartitionKey != "acct_123" {
		t.Fatalf("deposit event = %#v", store.event)
	}
}

func TestServiceRejectsInvalidAndConflictingDeposits(t *testing.T) {
	t.Parallel()

	store := newMemoryStore()
	service := NewService(store, Config{MaxAmountCents: 10_000})
	valid := DepositRequest{
		AccountID: "acct_123", AmountCents: 5_000, Currency: "USD",
		FundingSource: "demo-operator", ExternalReference: "transfer-123",
	}
	metadata := RequestMetadata{IdempotencyKey: "fund-123", CorrelationID: "trace-123", OperatorReason: "demo funds"}

	invalid := valid
	invalid.AccountID = "external-settlement"
	if _, err := service.Deposit(context.Background(), invalid, metadata); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("reserved account error = %v, want ErrInvalidRequest", err)
	}
	invalid = valid
	invalid.AmountCents = 10_001
	if _, err := service.Deposit(context.Background(), invalid, metadata); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("oversized amount error = %v, want ErrInvalidRequest", err)
	}

	if _, err := service.Deposit(context.Background(), valid, metadata); err != nil {
		t.Fatalf("first Deposit returned error: %v", err)
	}
	changed := valid
	changed.AmountCents = 4_000
	if _, err := service.Deposit(context.Background(), changed, metadata); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("changed replay error = %v, want ErrIdempotencyConflict", err)
	}

	store.createErr = ErrExternalReferenceConflict
	metadata.IdempotencyKey = "fund-456"
	if _, err := service.Deposit(context.Background(), valid, metadata); !errors.Is(err, ErrExternalReferenceConflict) {
		t.Fatalf("source reference error = %v, want ErrExternalReferenceConflict", err)
	}
}

type memoryStore struct {
	records   map[string]IdempotencyRecord
	deposit   Deposit
	event     domain.OutboxEvent
	creates   int
	createErr error
}

func newMemoryStore() *memoryStore {
	return &memoryStore{records: make(map[string]IdempotencyRecord)}
}

func (s *memoryStore) FindDepositIdempotency(_ context.Context, key string) (IdempotencyRecord, bool, error) {
	record, ok := s.records[key]
	return record, ok, nil
}

func (s *memoryStore) CreateDeposit(_ context.Context, deposit Deposit, event domain.OutboxEvent) (DepositResponse, error) {
	if s.createErr != nil {
		return DepositResponse{}, s.createErr
	}
	s.creates++
	s.deposit = deposit
	s.event = event
	response := DepositResponse{
		DepositID:         deposit.Transaction.ID,
		TransactionID:     deposit.Transaction.ID,
		Status:            deposit.Transaction.Status,
		AccountID:         deposit.Transaction.AccountID,
		AmountCents:       deposit.Transaction.AmountCents,
		Currency:          deposit.Transaction.Currency,
		BalanceAfterCents: deposit.Transaction.AmountCents,
		CorrelationID:     deposit.Transaction.CorrelationID,
		CreatedAt:         deposit.Transaction.CreatedAt,
	}
	s.records[deposit.IdempotencyKey] = IdempotencyRecord{RequestHash: deposit.RequestHash, Response: response}
	return response, nil
}
