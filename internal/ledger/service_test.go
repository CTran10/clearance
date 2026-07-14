package ledger

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/CTran10/clearance/internal/domain"
)

func TestServiceCreatesImmutableLedgerEntriesAndOutboxOnce(t *testing.T) {
	t.Parallel()

	store := NewMemoryStore()
	store.AddPendingTransaction(domain.Transaction{
		ID: "txn_123", AccountID: "acct_123", AmountCents: 12_550, Currency: "USD", Status: domain.TransactionPending,
	})
	store.Credit("acct_123", "USD", 20_000)
	service := NewService(store)
	payload := riskPayload(t, domain.RiskEvaluated{
		TransactionID: "txn_123", AccountID: "acct_123", AmountCents: 12_550, Currency: "USD",
		RiskLevel: domain.RiskLow, Approved: true, CorrelationID: "trace_123",
	})

	for range 2 {
		if err := service.HandleRiskEvaluated(context.Background(), ledgerDelivery("evt_risk"), payload); err != nil {
			t.Fatalf("HandleRiskEvaluated returned error: %v", err)
		}
	}

	entries := store.LedgerEntries()
	if len(entries) != 2 {
		t.Fatalf("ledger entry count = %d, want 2", len(entries))
	}
	if entries[0].AmountCents+entries[1].AmountCents != 0 {
		t.Fatal("ledger entries should balance to zero")
	}
	if got := store.TransactionStatus("txn_123"); got != domain.TransactionAuthorized {
		t.Fatalf("transaction status = %q, want %q", got, domain.TransactionAuthorized)
	}
	events := store.OutboxEvents()
	if len(events) != 1 || events[0].Type != domain.EventTransactionAuthorized {
		t.Fatalf("outbox events = %#v, want one TransactionAuthorized", events)
	}
	if events[0].AggregateID != "txn_123" || events[0].PartitionKey != "acct_123" {
		t.Fatalf("outbox routing = aggregate %q partition %q", events[0].AggregateID, events[0].PartitionKey)
	}
}

func TestServiceAtomicallyFailsWhenFundsAreUnavailable(t *testing.T) {
	t.Parallel()

	store := NewMemoryStore()
	store.AddPendingTransaction(domain.Transaction{
		ID: "txn_empty", AccountID: "acct_empty", AmountCents: 12_550, Currency: "USD", Status: domain.TransactionPending,
	})
	service := NewService(store)
	payload := riskPayload(t, domain.RiskEvaluated{
		TransactionID: "txn_empty", AccountID: "acct_empty", AmountCents: 12_550, Currency: "USD",
		RiskLevel: domain.RiskLow, Approved: true, CorrelationID: "trace_empty",
	})

	if err := service.HandleRiskEvaluated(context.Background(), ledgerDelivery("evt_empty"), payload); err != nil {
		t.Fatalf("HandleRiskEvaluated returned error: %v", err)
	}
	if len(store.LedgerEntries()) != 0 {
		t.Fatal("insufficient funds should not create ledger entries")
	}
	if got := store.TransactionStatus("txn_empty"); got != domain.TransactionFailed {
		t.Fatalf("transaction status = %q, want %q", got, domain.TransactionFailed)
	}
	events := store.OutboxEvents()
	if len(events) != 1 || events[0].Type != domain.EventTransactionFailed {
		t.Fatalf("outbox events = %#v, want one TransactionFailed", events)
	}
	var outcome domain.RiskEvaluated
	if err := json.Unmarshal(events[0].Payload, &outcome); err != nil {
		t.Fatalf("decode outcome: %v", err)
	}
	if outcome.Approved || outcome.Reason != "insufficient funds" {
		t.Fatalf("outcome = %#v, want insufficient-funds failure", outcome)
	}
}

func TestServiceFailsHighRiskEvaluationWithoutLedgerEntries(t *testing.T) {
	t.Parallel()

	store := NewMemoryStore()
	store.AddPendingTransaction(domain.Transaction{
		ID: "txn_high", AccountID: "acct_123", AmountCents: 90_000, Currency: "USD", Status: domain.TransactionPending,
	})
	service := NewService(store)
	payload := riskPayload(t, domain.RiskEvaluated{
		TransactionID: "txn_high", AccountID: "acct_123", AmountCents: 90_000, Currency: "USD",
		RiskLevel: domain.RiskHigh, Approved: false, Reason: "amount is greater than 500.00", CorrelationID: "trace_high",
	})

	if err := service.HandleRiskEvaluated(context.Background(), ledgerDelivery("evt_high"), payload); err != nil {
		t.Fatalf("HandleRiskEvaluated returned error: %v", err)
	}
	if len(store.LedgerEntries()) != 0 {
		t.Fatal("high risk transaction should not create ledger entries")
	}
	if got := store.TransactionStatus("txn_high"); got != domain.TransactionFailed {
		t.Fatalf("transaction status = %q, want %q", got, domain.TransactionFailed)
	}
	if events := store.OutboxEvents(); len(events) != 1 || events[0].Type != domain.EventTransactionFailed {
		t.Fatalf("outbox events = %#v, want one TransactionFailed", events)
	}
}

func TestServiceRejectsInvalidRiskCombinationsBeforePersistence(t *testing.T) {
	t.Parallel()

	tests := []domain.RiskEvaluated{
		{TransactionID: "txn_1", AccountID: "acct_1", AmountCents: 1, Currency: "USD", RiskLevel: domain.RiskLow, Approved: false},
		{TransactionID: "txn_1", AccountID: "acct_1", AmountCents: 1, Currency: "USD", RiskLevel: domain.RiskHigh, Approved: true},
	}
	for _, event := range tests {
		store := NewMemoryStore()
		store.AddPendingTransaction(domain.Transaction{
			ID: "txn_1", AccountID: "acct_1", AmountCents: 1, Currency: "USD", Status: domain.TransactionPending,
		})
		service := NewService(store)
		if err := service.HandleRiskEvaluated(context.Background(), ledgerDelivery("evt_invalid"), riskPayload(t, event)); err == nil {
			t.Fatalf("invalid evaluation %#v should fail", event)
		}
		if len(store.OutboxEvents()) != 0 {
			t.Fatal("invalid evaluation should not persist an outbox event")
		}
	}
}

func TestServiceRejectsEventIDReusedWithDifferentPayload(t *testing.T) {
	t.Parallel()

	store := NewMemoryStore()
	store.AddPendingTransaction(domain.Transaction{
		ID: "txn_123", AccountID: "acct_123", AmountCents: 12_550, Currency: "USD", Status: domain.TransactionPending,
	})
	store.Credit("acct_123", "USD", 20_000)
	service := NewService(store)
	first := riskPayload(t, domain.RiskEvaluated{
		TransactionID: "txn_123", AccountID: "acct_123", AmountCents: 12_550, Currency: "USD", RiskLevel: domain.RiskLow, Approved: true,
	})
	changed := riskPayload(t, domain.RiskEvaluated{
		TransactionID: "txn_123", AccountID: "acct_123", AmountCents: 12_551, Currency: "USD", RiskLevel: domain.RiskLow, Approved: true,
	})

	if err := service.HandleRiskEvaluated(context.Background(), ledgerDelivery("evt_risk"), first); err != nil {
		t.Fatalf("first delivery returned error: %v", err)
	}
	if err := service.HandleRiskEvaluated(context.Background(), ledgerDelivery("evt_risk"), changed); !errors.Is(err, domain.ErrEventIdentityConflict) {
		t.Fatalf("changed delivery error = %v, want ErrEventIdentityConflict", err)
	}
}

func TestServiceRejectsTamperedEvaluation(t *testing.T) {
	t.Parallel()

	store := NewMemoryStore()
	store.AddPendingTransaction(domain.Transaction{
		ID: "txn_tampered", AccountID: "acct_123", AmountCents: 12_550, Currency: "USD", Status: domain.TransactionPending,
	})
	store.Credit("acct_123", "USD", 20_000)
	service := NewService(store)
	payload := riskPayload(t, domain.RiskEvaluated{
		TransactionID: "txn_tampered", AccountID: "acct_attacker", AmountCents: 1, Currency: "USD",
		RiskLevel: domain.RiskLow, Approved: true,
	})

	if err := service.HandleRiskEvaluated(context.Background(), ledgerDelivery("evt_tampered"), payload); err == nil {
		t.Fatal("tampered evaluation should fail")
	}
	if got := store.TransactionStatus("txn_tampered"); got != domain.TransactionPending {
		t.Fatalf("transaction status = %q, want %q", got, domain.TransactionPending)
	}
	if len(store.LedgerEntries()) != 0 || len(store.OutboxEvents()) != 0 {
		t.Fatal("tampered evaluation should have no database effects")
	}
}

func riskPayload(t *testing.T, event domain.RiskEvaluated) []byte {
	t.Helper()
	payload, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal risk evaluation: %v", err)
	}
	return payload
}
