package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/CTran10/clearance/internal/domain"
	"github.com/CTran10/clearance/internal/transaction"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func Open(ctx context.Context, databaseURL string) (*Store, error) {
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("connect postgres: %w", err)
	}
	return NewStore(pool), nil
}

func (s *Store) Close() {
	s.pool.Close()
}

func (s *Store) Ping(ctx context.Context) error {
	return s.pool.Ping(ctx)
}

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

	if _, err := dbtx.Exec(
		ctx,
		`insert into outbox_events (id, event_type, aggregate_id, correlation_id, payload, status)
		 values ($1, $2, $3, $4, $5, $6)`,
		event.ID,
		event.Type,
		record.Transaction.ID,
		event.CorrelationID,
		event.Payload,
		event.Status,
	); err != nil {
		return fmt.Errorf("insert outbox event: %w", err)
	}

	if err := dbtx.Commit(ctx); err != nil {
		return fmt.Errorf("commit create transaction: %w", err)
	}
	return nil
}

func (s *Store) NextPending(ctx context.Context) (domain.OutboxEvent, bool, error) {
	// HONESTY HOUR: this assumes ONE outbox-publisher running. it just grabs the oldest pending row.
	// if i ever run two publishers, both could grab the same row and double-publish. the fix is
	// `... for update skip locked` so each worker claims a different row — i know the spell, just don't need it yet
	// for a single publisher. writing it down so future-me doesn't scale this and get a mystery double-event bug
	var event domain.OutboxEvent
	err := s.pool.QueryRow(
		ctx,
		`select id, event_type, correlation_id, payload, status, attempts, created_at
		   from outbox_events
		  where status = $1
		  order by created_at, id
		  limit 1`,
		domain.OutboxPending,
	).Scan(
		&event.ID,
		&event.Type,
		&event.CorrelationID,
		&event.Payload,
		&event.Status,
		&event.Attempts,
		&event.CreatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return domain.OutboxEvent{}, false, nil
		}
		return domain.OutboxEvent{}, false, fmt.Errorf("query pending outbox event: %w", err)
	}
	return event, true, nil
}

func (s *Store) MarkPublished(ctx context.Context, eventID string) error {
	_, err := s.pool.Exec(
		ctx,
		`update outbox_events
		    set status = $2, published_at = now(), updated_at = now()
		  where id = $1`,
		eventID,
		domain.OutboxPublished,
	)
	if err != nil {
		return fmt.Errorf("mark outbox event published: %w", err)
	}
	return nil
}

func (s *Store) MarkFailedAttempt(ctx context.Context, eventID string, maxAttempts int) error {
	_, err := s.pool.Exec(
		ctx,
		`update outbox_events
		    set attempts = attempts + 1,
		        status = case when attempts + 1 >= $2 then $3 else $4 end,
		        last_error = 'publish failed',
		        updated_at = now()
		  where id = $1`,
		eventID,
		maxAttempts,
		domain.OutboxDeadLettered,
		domain.OutboxPending,
	)
	if err != nil {
		return fmt.Errorf("mark outbox event failed: %w", err)
	}
	return nil
}

func (s *Store) Authorize(ctx context.Context, event domain.RiskEvaluated) ([]domain.LedgerEntry, error) {
	dbtx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("begin ledger transaction: %w", err)
	}
	defer func() {
		_ = dbtx.Rollback(ctx)
	}()

	entries := []domain.LedgerEntry{
		{
			ID:            domain.NewID("le"),
			TransactionID: event.TransactionID,
			AccountID:     event.AccountID,
			AmountCents:   -event.AmountCents,
			Currency:      event.Currency,
		},
		{
			ID:            domain.NewID("le"),
			TransactionID: event.TransactionID,
			AccountID:     "clearing",
			AmountCents:   event.AmountCents,
			Currency:      event.Currency,
		},
	}
	for _, entry := range entries {
		if _, err := dbtx.Exec(
			ctx,
			`insert into ledger_entries (id, transaction_id, account_id, amount_cents, currency)
			 values ($1, $2, $3, $4, $5)
			 on conflict (transaction_id, account_id) do nothing`,
			entry.ID,
			entry.TransactionID,
			entry.AccountID,
			entry.AmountCents,
			entry.Currency,
		); err != nil {
			return nil, fmt.Errorf("insert ledger entry: %w", err)
		}
	}

	if _, err := dbtx.Exec(
		ctx,
		`update transactions
		    set status = $2, risk_level = $3, risk_reason = $4, updated_at = now()
		  where id = $1`,
		event.TransactionID,
		domain.TransactionAuthorized,
		event.RiskLevel,
		event.Reason,
	); err != nil {
		return nil, fmt.Errorf("mark transaction authorized: %w", err)
	}

	if err := dbtx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit ledger transaction: %w", err)
	}
	return entries, nil
}

func (s *Store) Fail(ctx context.Context, event domain.RiskEvaluated) error {
	_, err := s.pool.Exec(
		ctx,
		`update transactions
		    set status = $2, risk_level = $3, risk_reason = $4, updated_at = now()
		  where id = $1`,
		event.TransactionID,
		domain.TransactionFailed,
		event.RiskLevel,
		event.Reason,
	)
	if err != nil {
		return fmt.Errorf("mark transaction failed: %w", err)
	}
	return nil
}

func (s *Store) InsertAuditLog(
	ctx context.Context,
	action string,
	transactionID string,
	correlationID string,
	metadata []byte,
) error {
	_, err := s.pool.Exec(
		ctx,
		`insert into audit_logs (action, transaction_id, correlation_id, metadata)
		 values ($1, $2, $3, $4)`,
		action,
		transactionID,
		correlationID,
		metadata,
	)
	if err != nil {
		return fmt.Errorf("insert audit log: %w", err)
	}
	return nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
