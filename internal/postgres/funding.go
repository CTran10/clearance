package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/CTran10/clearance/internal/domain"
	"github.com/CTran10/clearance/internal/funding"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func (s *Store) FindDepositIdempotency(ctx context.Context, key string) (funding.IdempotencyRecord, bool, error) {
	var requestHash string
	var responseJSON []byte
	err := s.pool.QueryRow(
		ctx,
		`select request_hash, response_json from deposit_idempotency_keys where key = $1`,
		key,
	).Scan(&requestHash, &responseJSON)
	if err != nil {
		if err == pgx.ErrNoRows {
			return funding.IdempotencyRecord{}, false, nil
		}
		return funding.IdempotencyRecord{}, false, fmt.Errorf("query deposit idempotency key: %w", err)
	}
	var response funding.DepositResponse
	if err := json.Unmarshal(responseJSON, &response); err != nil {
		return funding.IdempotencyRecord{}, false, fmt.Errorf("decode deposit idempotency response: %w", err)
	}
	return funding.IdempotencyRecord{RequestHash: requestHash, Response: response}, true, nil
}

func (s *Store) CreateDeposit(
	ctx context.Context,
	deposit funding.Deposit,
	event domain.OutboxEvent,
) (funding.DepositResponse, error) {
	dbtx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return funding.DepositResponse{}, fmt.Errorf("begin deposit: %w", err)
	}
	defer func() {
		_ = dbtx.Rollback(ctx)
	}()

	transaction := deposit.Transaction
	if _, err := dbtx.Exec(
		ctx,
		`select pg_advisory_xact_lock(hashtext($1), hashtext($2))`,
		transaction.AccountID,
		transaction.Currency,
	); err != nil {
		return funding.DepositResponse{}, fmt.Errorf("lock deposit account balance: %w", err)
	}

	if _, err := dbtx.Exec(
		ctx,
		`insert into transactions
			(id, kind, account_id, merchant_id, funding_source, external_reference,
			 amount_cents, currency, status, correlation_id, created_at, updated_at)
		 values ($1, $2, $3, null, $4, $5, $6, $7, $8, $9, $10, $10)`,
		transaction.ID,
		transaction.Kind,
		transaction.AccountID,
		transaction.FundingSource,
		transaction.ExternalRef,
		transaction.AmountCents,
		transaction.Currency,
		transaction.Status,
		transaction.CorrelationID,
		transaction.CreatedAt,
	); err != nil {
		if constraintName(err) == "ux_transactions_deposit_source_reference" {
			return funding.DepositResponse{}, funding.ErrExternalReferenceConflict
		}
		return funding.DepositResponse{}, fmt.Errorf("insert deposit transaction: %w", err)
	}

	entries := []domain.LedgerEntry{
		{
			ID: domain.NewID("le"), TransactionID: transaction.ID, AccountID: transaction.AccountID,
			AmountCents: transaction.AmountCents, Currency: transaction.Currency,
		},
		{
			ID: domain.NewID("le"), TransactionID: transaction.ID, AccountID: "external-settlement",
			AmountCents: -transaction.AmountCents, Currency: transaction.Currency,
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
			return funding.DepositResponse{}, fmt.Errorf("insert deposit ledger entry: %w", err)
		}
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
		return funding.DepositResponse{}, fmt.Errorf("query balance after deposit: %w", err)
	}
	response := funding.DepositResponse{
		DepositID: transaction.ID, TransactionID: transaction.ID, Status: transaction.Status,
		AccountID: transaction.AccountID, AmountCents: transaction.AmountCents, Currency: transaction.Currency,
		BalanceAfterCents: balance, CorrelationID: transaction.CorrelationID, CreatedAt: transaction.CreatedAt,
	}
	responseJSON, err := json.Marshal(response)
	if err != nil {
		return funding.DepositResponse{}, fmt.Errorf("encode deposit idempotency response: %w", err)
	}
	if _, err := dbtx.Exec(
		ctx,
		`insert into deposit_idempotency_keys (key, request_hash, transaction_id, response_json)
		 values ($1, $2, $3, $4)`,
		deposit.IdempotencyKey,
		deposit.RequestHash,
		transaction.ID,
		responseJSON,
	); err != nil {
		if isUniqueViolation(err) {
			return funding.DepositResponse{}, funding.ErrIdempotencyConflict
		}
		return funding.DepositResponse{}, fmt.Errorf("insert deposit idempotency key: %w", err)
	}
	if _, err := dbtx.Exec(
		ctx,
		`insert into operator_actions (id, action_type, target_id, reason)
		 values ($1, 'DEPOSIT', $2, $3)`,
		domain.NewID("act"),
		transaction.ID,
		deposit.OperatorReason,
	); err != nil {
		return funding.DepositResponse{}, fmt.Errorf("insert deposit audit action: %w", err)
	}
	if err := insertOutbox(ctx, dbtx, event); err != nil {
		return funding.DepositResponse{}, fmt.Errorf("insert deposit outbox event: %w", err)
	}
	if err := dbtx.Commit(ctx); err != nil {
		return funding.DepositResponse{}, fmt.Errorf("commit deposit: %w", err)
	}
	return response, nil
}

func constraintName(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.ConstraintName
	}
	return ""
}
