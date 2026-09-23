package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/CTran10/clearance/internal/domain"
	"github.com/CTran10/clearance/internal/transaction"
	"github.com/jackc/pgx/v5"
)

func (s *Store) FindIdempotency(ctx context.Context, key string) (transaction.IdempotencyRecord, bool, error) {
	var requestHash string
	var responseJSON []byte
	err := s.pool.QueryRow(
		ctx,
		`select request_hash, response_json from idempotency_keys where key = $1`,
		key,
	).Scan(&requestHash, &responseJSON)
	if err != nil {
		if err == pgx.ErrNoRows {
			return transaction.IdempotencyRecord{}, false, nil
		}
		return transaction.IdempotencyRecord{}, false, fmt.Errorf("query idempotency key: %w", err)
	}

	var response transaction.CreateResponse
	if err := json.Unmarshal(responseJSON, &response); err != nil {
		return transaction.IdempotencyRecord{}, false, fmt.Errorf("decode idempotency response: %w", err)
	}
	return transaction.IdempotencyRecord{
		Key:          key,
		RequestHash:  requestHash,
		CreateResult: response,
	}, true, nil
}

func (s *Store) Create(ctx context.Context, record transaction.IdempotencyRecord, event domain.OutboxEvent) error {
	dbtx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin create transaction: %w", err)
	}
	defer func() {
		_ = dbtx.Rollback(ctx)
	}()

	if _, err := dbtx.Exec(
		ctx,
		`insert into transactions
			(id, account_id, merchant_id, amount_cents, currency, status, correlation_id)
		 values ($1, $2, $3, $4, $5, $6, $7)`,
		record.Transaction.ID,
		record.Transaction.AccountID,
		record.Transaction.MerchantID,
		record.Transaction.AmountCents,
		record.Transaction.Currency,
		record.Transaction.Status,
		record.Transaction.CorrelationID,
	); err != nil {
		return fmt.Errorf("insert transaction: %w", err)
	}

	responseJSON, err := json.Marshal(record.CreateResult)
	if err != nil {
		return fmt.Errorf("encode idempotency response: %w", err)
	}
	if _, err := dbtx.Exec(
		ctx,
		`insert into idempotency_keys (key, request_hash, transaction_id, response_json)
		 values ($1, $2, $3, $4)`,
		record.Key,
		record.RequestHash,
		record.Transaction.ID,
		responseJSON,
	); err != nil {
		if isUniqueViolation(err) {
			return transaction.ErrIdempotencyConflict
		}
		return fmt.Errorf("insert idempotency key: %w", err)
	}

	if err := insertOutbox(ctx, dbtx, event); err != nil {
		return fmt.Errorf("insert outbox event: %w", err)
	}

	if err := dbtx.Commit(ctx); err != nil {
		return fmt.Errorf("commit create transaction: %w", err)
	}
	return nil
}
