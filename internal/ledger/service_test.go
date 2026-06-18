package ledger

import (
	"context"
	"testing"

	"github.com/CTran10/clearance/internal/domain"
)

func TestServiceCreatesImmutableLedgerEntriesForLowRiskEvaluation(t *testing.T) {
	t.Parallel()

	store := NewMemoryStore()
	publisher := NewRecordingPublisher()
	service := NewService(store, publisher)

	event := domain.RiskEvaluated{
		TransactionID: "txn_123",
		AccountID:     "acct_123",
		AmountCents:   12_550,
		Currency:      "USD",
		RiskLevel:     domain.RiskLow,
		Approved:      true,
		CorrelationID: "trace_123",
	}

	if err := service.HandleRiskEvaluated(context.Background(), event); err != nil {
		t.Fatalf("HandleRiskEvaluated returned error: %v", err)
	}

	entries := store.LedgerEntries()
	if len(entries) != 2 {
		t.Fatalf("ledger entry count = %d, want 2", len(entries))
	}
	if entries[0].TransactionID != event.TransactionID || entries[1].TransactionID != event.TransactionID {
		t.Fatal("ledger entries should reference the evaluated transaction")
	}
	if entries[0].AmountCents+entries[1].AmountCents != 0 {
		t.Fatal("ledger entries should balance to zero")
	}
	if got := store.TransactionStatus(event.TransactionID); got != domain.TransactionAuthorized {
		t.Fatalf("transaction status = %q, want %q", got, domain.TransactionAuthorized)
	}

	published := publisher.Events()
	if len(published) != 1 || published[0].Type != domain.EventTransactionAuthorized {
		t.Fatalf("published events = %#v, want one TransactionAuthorized", published)
	}
	if published[0].CorrelationID != event.CorrelationID {
		t.Fatal("correlation id should be propagated")
	}
}

func TestServiceFailsHighRiskEvaluationWithoutLedgerEntries(t *testing.T) {
	t.Parallel()

	store := NewMemoryStore()
	publisher := NewRecordingPublisher()
	service := NewService(store, publisher)

	event := domain.RiskEvaluated{
		TransactionID: "txn_high",
		AccountID:     "acct_123",
		AmountCents:   90_000,
		Currency:      "USD",
		RiskLevel:     domain.RiskHigh,
		Approved:      false,
		CorrelationID: "trace_high",
	}

	if err := service.HandleRiskEvaluated(context.Background(), event); err != nil {
		t.Fatalf("HandleRiskEvaluated returned error: %v", err)
	}

	if len(store.LedgerEntries()) != 0 {
		t.Fatal("high risk transaction should not create ledger entries")
	}
	if got := store.TransactionStatus(event.TransactionID); got != domain.TransactionFailed {
		t.Fatalf("transaction status = %q, want %q", got, domain.TransactionFailed)
	}

	published := publisher.Events()
	if len(published) != 1 || published[0].Type != domain.EventTransactionFailed {
		t.Fatalf("published events = %#v, want one TransactionFailed", published)
	}
}
