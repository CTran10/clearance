//go:build integration

package postgres

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/CTran10/clearance/internal/consumer"
	"github.com/CTran10/clearance/internal/domain"
)

const (
	testPayloadHash  = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	otherPayloadHash = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

func TestSaveConsumerOutboxIsIdempotentAndAtomic(t *testing.T) {
	store := openIntegrationStore(t)
	event := domain.NewOutboxEvent(
		domain.EventRiskEvaluated,
		"txn_risk",
		"acct_risk",
		"trace_risk",
		[]byte(`{"transaction_id":"txn_risk"}`),
	)

	created, err := store.SaveConsumerOutbox(
		context.Background(), integrationDelivery("risk-service", "evt_created"), testPayloadHash, event,
	)
	if err != nil || !created {
		t.Fatalf("first SaveConsumerOutbox = %v, %v; want true, nil", created, err)
	}
	staleLastSeen := time.Now().UTC().Add(-time.Hour)
	setProcessedEventLastSeen(t, store, "risk-service", "evt_created", staleLastSeen)
	created, err = store.SaveConsumerOutbox(
		context.Background(), integrationDelivery("risk-service", "evt_created"), testPayloadHash, event,
	)
	if err != nil || created {
		t.Fatalf("duplicate SaveConsumerOutbox = %v, %v; want false, nil", created, err)
	}
	assertProcessedEventRefreshed(t, store, "risk-service", "evt_created", staleLastSeen)
	if _, err := store.SaveConsumerOutbox(
		context.Background(), integrationDelivery("risk-service", "evt_created"), otherPayloadHash, event,
	); !errors.Is(err, domain.ErrEventIdentityConflict) {
		t.Fatalf("conflicting SaveConsumerOutbox error = %v, want ErrEventIdentityConflict", err)
	}
	assertCount(t, store, "processed_events", 1)
	assertCount(t, store, "outbox_events", 1)
	pending, ok, err := store.NextPending(context.Background())
	if err != nil || !ok {
		t.Fatalf("NextPending = %#v, %v, %v; want event, true, nil", pending, ok, err)
	}
	if pending.AggregateID != "txn_risk" || pending.PartitionKey != "acct_risk" {
		t.Fatalf("pending routing = aggregate %q partition %q", pending.AggregateID, pending.PartitionKey)
	}

	_, err = store.SaveConsumerOutbox(
		context.Background(), integrationDelivery("risk-service", "evt_rollback"), testPayloadHash, event,
	)
	if err == nil {
		t.Fatal("duplicate outbox id should fail")
	}
	var processed int
	if err := store.pool.QueryRow(
		context.Background(),
		`select count(*) from processed_events where event_id = 'evt_rollback'`,
	).Scan(&processed); err != nil {
		t.Fatalf("count rolled-back processed event: %v", err)
	}
	if processed != 0 {
		t.Fatalf("rolled-back processed event count = %d, want 0", processed)
	}
}

func TestProcessRiskEvaluatedProducesOneAtomicLedgerOutcome(t *testing.T) {
	store := openIntegrationStore(t)
	seedPendingTransactionAndFunds(t, store, "txn_ledger", "acct_ledger", 20_000)
	event := domain.RiskEvaluated{
		TransactionID: "txn_ledger",
		AccountID:     "acct_ledger",
		AmountCents:   12_550,
		Currency:      "USD",
		RiskLevel:     domain.RiskLow,
		Approved:      true,
		CorrelationID: "trace_ledger",
	}

	staleLastSeen := time.Now().UTC().Add(-time.Hour)
	for delivery := 1; delivery <= 2; delivery++ {
		created, err := store.ProcessRiskEvaluated(
			context.Background(), integrationDelivery("ledger-service", "evt_risk"), testPayloadHash, event,
		)
		if err != nil {
			t.Fatalf("delivery %d returned error: %v", delivery, err)
		}
		if created != (delivery == 1) {
			t.Fatalf("delivery %d created = %v", delivery, created)
		}
		if delivery == 1 {
			setProcessedEventLastSeen(t, store, "ledger-service", "evt_risk", staleLastSeen)
		}
	}
	assertProcessedEventRefreshed(t, store, "ledger-service", "evt_risk", staleLastSeen)

	var status domain.TransactionStatus
	if err := store.pool.QueryRow(
		context.Background(), `select status from transactions where id = 'txn_ledger'`,
	).Scan(&status); err != nil {
		t.Fatalf("query transaction status: %v", err)
	}
	if status != domain.TransactionAuthorized {
		t.Fatalf("transaction status = %q, want %q", status, domain.TransactionAuthorized)
	}
	var ledgerEntries, outcomeEvents, processedEvents int
	if err := store.pool.QueryRow(
		context.Background(), `select count(*) from ledger_entries where transaction_id = 'txn_ledger'`,
	).Scan(&ledgerEntries); err != nil {
		t.Fatalf("count ledger entries: %v", err)
	}
	if err := store.pool.QueryRow(
		context.Background(), `select count(*) from outbox_events where aggregate_id = 'txn_ledger'`,
	).Scan(&outcomeEvents); err != nil {
		t.Fatalf("count outcome events: %v", err)
	}
	if err := store.pool.QueryRow(
		context.Background(), `select count(*) from processed_events where consumer_name = 'ledger-service' and event_id = 'evt_risk'`,
	).Scan(&processedEvents); err != nil {
		t.Fatalf("count processed events: %v", err)
	}
	if ledgerEntries != 2 || outcomeEvents != 1 || processedEvents != 1 {
		t.Fatalf("ledger/outbox/processed counts = %d/%d/%d, want 2/1/1", ledgerEntries, outcomeEvents, processedEvents)
	}
}

func TestProcessRiskEvaluatedRollsBackStateWhenOutboxInsertFails(t *testing.T) {
	store := openIntegrationStore(t)
	seedPendingTransactionAndFunds(t, store, "txn_rollback", "acct_rollback", 20_000)
	if _, err := store.pool.Exec(context.Background(), `
		create function reject_final_outbox() returns trigger language plpgsql as $$
		begin
			if new.event_type in ('TransactionAuthorized', 'TransactionFailed') then
				raise exception 'forced final outbox failure';
			end if;
			return new;
		end;
		$$;
		create trigger trg_reject_final_outbox before insert on outbox_events
		for each row execute function reject_final_outbox();
	`); err != nil {
		t.Fatalf("install failure trigger: %v", err)
	}
	event := domain.RiskEvaluated{
		TransactionID: "txn_rollback", AccountID: "acct_rollback", AmountCents: 12_550, Currency: "USD",
		RiskLevel: domain.RiskLow, Approved: true, CorrelationID: "trace_rollback",
	}

	if _, err := store.ProcessRiskEvaluated(
		context.Background(), integrationDelivery("ledger-service", "evt_rollback"), testPayloadHash, event,
	); err == nil {
		t.Fatal("forced outbox failure should return an error")
	}

	var status domain.TransactionStatus
	var ledgerEntries, processedEvents int
	if err := store.pool.QueryRow(
		context.Background(), `select status from transactions where id = 'txn_rollback'`,
	).Scan(&status); err != nil {
		t.Fatalf("query transaction status: %v", err)
	}
	if err := store.pool.QueryRow(
		context.Background(), `select count(*) from ledger_entries where transaction_id = 'txn_rollback'`,
	).Scan(&ledgerEntries); err != nil {
		t.Fatalf("count ledger entries: %v", err)
	}
	if err := store.pool.QueryRow(
		context.Background(), `select count(*) from processed_events where event_id = 'evt_rollback'`,
	).Scan(&processedEvents); err != nil {
		t.Fatalf("count processed events: %v", err)
	}
	if status != domain.TransactionPending || ledgerEntries != 0 || processedEvents != 0 {
		t.Fatalf("status/ledger/processed = %q/%d/%d, want PENDING/0/0", status, ledgerEntries, processedEvents)
	}
}

func openIntegrationStore(t *testing.T) *Store {
	t.Helper()
	databaseURL := os.Getenv("CLEARANCE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("CLEARANCE_TEST_DATABASE_URL is not set")
	}
	store, err := Open(context.Background(), databaseURL)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	t.Cleanup(store.Close)
	if _, err := store.pool.Exec(context.Background(), `drop schema public cascade; create schema public`); err != nil {
		t.Fatalf("reset schema: %v", err)
	}
	migrations, err := filepath.Glob("../../migrations/*.sql")
	if err != nil {
		t.Fatalf("list migrations: %v", err)
	}
	sort.Strings(migrations)
	for pass := 1; pass <= 2; pass++ {
		for _, migration := range migrations {
			sql, err := os.ReadFile(migration)
			if err != nil {
				t.Fatalf("read migration %s: %v", migration, err)
			}
			if _, err := store.pool.Exec(context.Background(), string(sql)); err != nil {
				t.Fatalf("apply migration %s (pass %d): %v", migration, pass, err)
			}
		}
	}
	return store
}

func seedPendingTransactionAndFunds(t *testing.T, store *Store, transactionID, accountID string, balance int64) {
	t.Helper()
	_, err := store.pool.Exec(context.Background(), `
		insert into transactions (id, account_id, merchant_id, amount_cents, currency, status, correlation_id)
		values
			('txn_funding', $1, 'funding', 1, 'USD', 'AUTHORIZED', 'trace_funding'),
			($2, $1, 'merchant', 12550, 'USD', 'PENDING', 'trace_pending')
	`, accountID, transactionID)
	if err != nil {
		t.Fatalf("seed transactions: %v", err)
	}
	_, err = store.pool.Exec(context.Background(), `
		insert into ledger_entries (id, transaction_id, account_id, amount_cents, currency)
		values
			('le_funding_account', 'txn_funding', $1, $2, 'USD'),
			('le_funding_clearing', 'txn_funding', 'clearing', -$2, 'USD')
	`, accountID, balance)
	if err != nil {
		t.Fatalf("seed funds: %v", err)
	}
}

func assertCount(t *testing.T, store *Store, table string, want int) {
	t.Helper()
	var got int
	if err := store.pool.QueryRow(context.Background(), "select count(*) from "+table).Scan(&got); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	if got != want {
		t.Fatalf("%s count = %d, want %d", table, got, want)
	}
}

func setProcessedEventLastSeen(t *testing.T, store *Store, consumerName, eventID string, value time.Time) {
	t.Helper()
	if _, err := store.pool.Exec(
		context.Background(),
		`update processed_events set last_seen_at = $3 where consumer_name = $1 and event_id = $2`,
		consumerName,
		eventID,
		value,
	); err != nil {
		t.Fatalf("set processed event last_seen_at: %v", err)
	}
}

func assertProcessedEventRefreshed(t *testing.T, store *Store, consumerName, eventID string, stale time.Time) {
	t.Helper()
	var lastSeen time.Time
	if err := store.pool.QueryRow(
		context.Background(),
		`select last_seen_at from processed_events where consumer_name = $1 and event_id = $2`,
		consumerName,
		eventID,
	).Scan(&lastSeen); err != nil {
		t.Fatalf("query processed event last_seen_at: %v", err)
	}
	if !lastSeen.After(stale) {
		t.Fatalf("processed event last_seen_at = %s, want after stale timestamp %s", lastSeen, stale)
	}
}

func integrationDelivery(consumerName, eventID string) consumer.Delivery {
	topic := "transactions.created"
	if consumerName == "ledger-service" {
		topic = "risk.evaluated"
	}
	return consumer.Delivery{
		ConsumerName: consumerName, EventID: eventID, SourceTopic: topic,
		SourcePartition: 0, SourceOffset: 1,
	}
}
