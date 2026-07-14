//go:build integration

package postgres

import (
	"context"
	"testing"

	"github.com/CTran10/clearance/internal/domain"
	"github.com/CTran10/clearance/internal/funding"
	"github.com/CTran10/clearance/internal/transaction"
)

func TestCreateDepositIsBalancedIdempotentAndQueryable(t *testing.T) {
	store := openIntegrationStore(t)
	service := funding.NewService(store, funding.Config{MaxAmountCents: 1_000_000})
	request := funding.DepositRequest{
		AccountID: "acct_funded", AmountCents: 25_000, Currency: "USD",
		FundingSource: "demo-operator", ExternalReference: "transfer-123",
	}
	metadata := funding.RequestMetadata{
		IdempotencyKey: "fund-123", CorrelationID: "trace-fund", OperatorReason: "seed integration account",
	}

	first, err := service.Deposit(context.Background(), request, metadata)
	if err != nil {
		t.Fatalf("Deposit returned error: %v", err)
	}
	second, err := service.Deposit(context.Background(), request, metadata)
	if err != nil || second.DepositID != first.DepositID {
		t.Fatalf("Deposit replay = %#v, %v; want id %s", second, err, first.DepositID)
	}

	var entries, idempotency, outbox, actions int
	var balance int64
	for table, target := range map[string]*int{
		"ledger_entries": &entries, "deposit_idempotency_keys": &idempotency,
		"outbox_events": &outbox, "operator_actions": &actions,
	} {
		if err := store.pool.QueryRow(context.Background(), "select count(*) from "+table).Scan(target); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
	}
	if err := store.pool.QueryRow(context.Background(), `select sum(amount_cents) from ledger_entries`).Scan(&balance); err != nil {
		t.Fatalf("sum ledger: %v", err)
	}
	if entries != 2 || idempotency != 1 || outbox != 1 || actions != 1 || balance != 0 {
		t.Fatalf("entries/idempotency/outbox/actions/balance = %d/%d/%d/%d/%d", entries, idempotency, outbox, actions, balance)
	}

	queries := transaction.NewQueryService(store)
	detail, err := queries.Get(context.Background(), first.TransactionID)
	if err != nil {
		t.Fatalf("Get deposit transaction: %v", err)
	}
	if detail.Kind != domain.TransactionDeposit || detail.Status != domain.TransactionAuthorized {
		t.Fatalf("deposit detail = %#v", detail)
	}
	page, err := queries.List(context.Background(), transaction.ListFilter{AccountID: "acct_funded", Limit: 25})
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("List deposits = %#v, %v", page, err)
	}
}
