package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/CTran10/clearance/internal/consumer"
	"github.com/CTran10/clearance/internal/domain"
	"github.com/jackc/pgx/v5"
)

func (s *Store) ProcessRiskEvaluated(
	ctx context.Context,
	delivery consumer.Delivery,
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

	claimed, err := claimProcessedEvent(ctx, dbtx, delivery, payloadHash)
	if err != nil {
		return false, err
	}
	if !claimed {
		if err := dbtx.Commit(ctx); err != nil {
			return false, fmt.Errorf("commit duplicate ledger event: %w", err)
		}
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

func ensureAvailableFunds(ctx context.Context, dbtx pgx.Tx, transaction domain.Transaction) error {
	if _, err := dbtx.Exec(
		ctx,
		`select pg_advisory_xact_lock(hashtext($1), hashtext($2))`,
		transaction.AccountID,
		transaction.Currency,
	); err != nil {
		return fmt.Errorf("lock account balance: %w", err)
	}

	var balance int64
	if err := dbtx.QueryRow(
		ctx,
		`select coalesce(sum(amount_cents), 0)
		   from ledger_entries
		  where account_id = $1 and currency = $2`,
		transaction.AccountID,
		transaction.Currency,
	).Scan(&balance); err != nil {
		return fmt.Errorf("query account balance: %w", err)
	}
	if balance < transaction.AmountCents {
		return domain.ErrInsufficientFunds
	}
	return nil
}

func lockPendingTransaction(ctx context.Context, dbtx pgx.Tx, event domain.RiskEvaluated) (domain.Transaction, error) {
	var transaction domain.Transaction
	err := dbtx.QueryRow(
		ctx,
		`select id, account_id, amount_cents, currency, status, correlation_id
		   from transactions
		  where id = $1
		  for update`,
		event.TransactionID,
	).Scan(
		&transaction.ID,
		&transaction.AccountID,
		&transaction.AmountCents,
		&transaction.Currency,
		&transaction.Status,
		&transaction.CorrelationID,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return domain.Transaction{}, fmt.Errorf("transaction not found")
		}
		return domain.Transaction{}, fmt.Errorf("lock transaction: %w", err)
	}
	if transaction.Status != domain.TransactionPending {
		return domain.Transaction{}, fmt.Errorf("transaction is not pending")
	}
	if transaction.AccountID != event.AccountID ||
		transaction.AmountCents != event.AmountCents ||
		transaction.Currency != event.Currency {
		return domain.Transaction{}, fmt.Errorf("risk event does not match transaction")
	}
	return transaction, nil
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
