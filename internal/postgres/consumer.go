package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/CTran10/clearance/internal/domain"
	"github.com/jackc/pgx/v5"
)

const ledgerConsumerName = "ledger-service"

func (s *Store) SaveConsumerOutbox(
	ctx context.Context,
	consumerName string,
	eventID string,
	payloadHash string,
	event domain.OutboxEvent,
) (bool, error) {
	dbtx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return false, fmt.Errorf("begin consumer transaction: %w", err)
	}
	defer func() {
		_ = dbtx.Rollback(ctx)
	}()

	claimed, err := claimProcessedEvent(ctx, dbtx, consumerName, eventID, payloadHash)
	if err != nil {
		return false, err
	}
	if !claimed {
		return false, nil
	}
	if err := insertOutbox(ctx, dbtx, event); err != nil {
		return false, fmt.Errorf("insert consumer outbox event: %w", err)
	}
	if err := dbtx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit consumer transaction: %w", err)
	}
	return true, nil
}

func (s *Store) ProcessRiskEvaluated(
	ctx context.Context,
	eventID string,
	payloadHash string,
	event domain.RiskEvaluated,
) (bool, error) {
	dbtx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return false, fmt.Errorf("begin ledger transaction: %w", err)
	}
	defer func() {
		_ = dbtx.Rollback(ctx)
	}()

	claimed, err := claimProcessedEvent(ctx, dbtx, ledgerConsumerName, eventID, payloadHash)
	if err != nil {
		return false, err
	}
	if !claimed {
		return false, nil
	}

	transaction, err := lockPendingTransaction(ctx, dbtx, event)
	if err != nil {
		return false, err
	}

	outcome := event
	status := domain.TransactionFailed
	eventType := domain.EventTransactionFailed
	if event.Approved {
		if err := ensureAvailableFunds(ctx, dbtx, transaction); err != nil {
			if !errors.Is(err, domain.ErrInsufficientFunds) {
				return false, err
			}
			outcome.Approved = false
			outcome.Reason = "insufficient funds"
		} else {
			if err := insertLedgerEntries(ctx, dbtx, transaction); err != nil {
				return false, err
			}
			status = domain.TransactionAuthorized
			eventType = domain.EventTransactionAuthorized
		}
	}

	tag, err := dbtx.Exec(
		ctx,
		`update transactions
		    set status = $2, risk_level = $3, risk_reason = $4, updated_at = now()
		  where id = $1 and status = $5`,
		transaction.ID,
		status,
		outcome.RiskLevel,
		outcome.Reason,
		domain.TransactionPending,
	)
	if err != nil {
		return false, fmt.Errorf("finalize transaction: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return false, fmt.Errorf("finalize transaction: transaction is not pending")
	}

	payload, err := json.Marshal(outcome)
	if err != nil {
		return false, fmt.Errorf("marshal ledger outcome: %w", err)
	}
	outboxEvent := domain.NewOutboxEvent(
		eventType,
		transaction.ID,
		transaction.AccountID,
		outcome.CorrelationID,
		payload,
	)
	if err := insertOutbox(ctx, dbtx, outboxEvent); err != nil {
		return false, fmt.Errorf("insert ledger outcome outbox event: %w", err)
	}

	if err := dbtx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit ledger transaction: %w", err)
	}
	return true, nil
}

func claimProcessedEvent(
	ctx context.Context,
	dbtx pgx.Tx,
	consumerName string,
	eventID string,
	payloadHash string,
) (bool, error) {
	tag, err := dbtx.Exec(
		ctx,
		`insert into processed_events (consumer_name, event_id, payload_sha256)
		 values ($1, $2, $3)
		 on conflict (consumer_name, event_id) do nothing`,
		consumerName,
		eventID,
		payloadHash,
	)
	if err != nil {
		return false, fmt.Errorf("claim processed event: %w", err)
	}
	if tag.RowsAffected() == 1 {
		return true, nil
	}

	var existingHash string
	if err := dbtx.QueryRow(
		ctx,
		`select payload_sha256
		   from processed_events
		  where consumer_name = $1 and event_id = $2`,
		consumerName,
		eventID,
	).Scan(&existingHash); err != nil {
		return false, fmt.Errorf("load processed event: %w", err)
	}
	if existingHash != payloadHash {
		return false, domain.ErrEventIdentityConflict
	}
	return false, nil
}

func insertOutbox(ctx context.Context, dbtx pgx.Tx, event domain.OutboxEvent) error {
	_, err := dbtx.Exec(
		ctx,
		`insert into outbox_events
			(id, event_type, aggregate_id, partition_key, correlation_id, payload, status)
		 values ($1, $2, $3, $4, $5, $6, $7)`,
		event.ID,
		event.Type,
		event.AggregateID,
		event.PartitionKey,
		event.CorrelationID,
		event.Payload,
		event.Status,
	)
	return err
}

func insertLedgerEntries(ctx context.Context, dbtx pgx.Tx, transaction domain.Transaction) error {
	// double-entry bookkeeping!! money never just "disappears" from one account — it MOVES. so every transaction
	// is two rows that sum to zero: minus X from the user, plus X into "clearing". if you add up every ledger entry
	// ever and it doesn't total 0, money got invented or destroyed and something is very wrong. accountants have been
	// doing this for ~500 years and i was today years old when i learned why. it makes the books auditable + self-checking
	entries := []domain.LedgerEntry{
		{
			ID:            domain.NewID("le"),
			TransactionID: transaction.ID,
			AccountID:     transaction.AccountID,
			AmountCents:   -transaction.AmountCents, // debit the user
			Currency:      transaction.Currency,
		},
		{
			ID:            domain.NewID("le"),
			TransactionID: transaction.ID,
			AccountID:     "clearing",
			AmountCents:   transaction.AmountCents, // credit clearing — equal + opposite, nets to 0
			Currency:      transaction.Currency,
		},
	}
	for _, entry := range entries {
		if _, err := dbtx.Exec(
			ctx,
			`insert into ledger_entries (id, transaction_id, account_id, amount_cents, currency)
			 values ($1, $2, $3, $4, $5)`,
			entry.ID,
			entry.TransactionID,
			entry.AccountID,
			entry.AmountCents,
			entry.Currency,
		); err != nil {
			return fmt.Errorf("insert ledger entry: %w", err)
		}
	}
	return nil
}
