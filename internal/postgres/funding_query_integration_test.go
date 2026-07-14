//go:build integration

package postgres

import (
	"context"
	"os"
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

func TestFundingMigrationPreservesLegacyReservedAccountRows(t *testing.T) {
	databaseURL := os.Getenv("CLEARANCE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("CLEARANCE_TEST_DATABASE_URL is not set")
	}
	store, err := Open(context.Background(), databaseURL)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	defer store.Close()
	if _, err := store.pool.Exec(context.Background(), `drop schema public cascade; create schema public`); err != nil {
		t.Fatalf("reset schema: %v", err)
	}
	for _, path := range []string{"../../migrations/001_init.sql", "../../migrations/002_consumer_reliability.sql"} {
		migration, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read migration %s: %v", path, err)
		}
		if _, err := store.pool.Exec(context.Background(), string(migration)); err != nil {
			t.Fatalf("apply migration %s: %v", path, err)
		}
	}
	if _, err := store.pool.Exec(context.Background(), `
		insert into transactions (id, account_id, merchant_id, amount_cents, currency, status, correlation_id)
		values ('txn_legacy_reserved', 'clearing', 'legacy-merchant', 100, 'USD', 'AUTHORIZED', 'trace_legacy')
	`); err != nil {
		t.Fatalf("seed legacy reserved account row: %v", err)
	}
	migration, err := os.ReadFile("../../migrations/003_funding_and_queries.sql")
	if err != nil {
		t.Fatalf("read funding migration: %v", err)
	}
	if _, err := store.pool.Exec(context.Background(), string(migration)); err != nil {
		t.Fatalf("apply funding migration over legacy row: %v", err)
	}
	if _, err := store.pool.Exec(context.Background(), `
		insert into transactions
			(id, kind, account_id, merchant_id, amount_cents, currency, status, correlation_id)
		values ('txn_new_reserved', 'PAYMENT', 'clearing', 'merchant', 100, 'USD', 'PENDING', 'trace_new')
	`); err == nil {
		t.Fatal("new payment using reserved account should violate the database constraint")
	}
}
