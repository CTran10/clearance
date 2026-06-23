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

func Open(ctx context.Context, databaseURL string) (*Store, error) {
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("connect postgres: %w", err)
	}
	return &Store{pool: pool}, nil
}

func (s *Store) Close() {
	s.pool.Close()
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
	// UPDATE TO PAST-ME: remember when i said i knew the "for update skip locked" spell but didn't need it yet? we need it.
	// this now does the real thing: SKIP LOCKED lets multiple publishers each grab a DIFFERENT pending row instead of
	// fighting over the same one (no double-publish). plus it flips the row to PROCESSING so a crashed worker doesn't
	// strand events forever — anything stuck PROCESSING for 5 min gets reclaimed. the CTE-then-update is one atomic
	// "claim a job" move. genuinely proud of this one, it took three rewrites to get right
	var event domain.OutboxEvent
	err := s.pool.QueryRow(
		ctx,
		`with next_event as (
		    select id
		      from outbox_events
		     where status = $1
		        or (status = $2 and updated_at < now() - interval '5 minutes')
		     order by created_at, id
		     for update skip locked
		     limit 1
		)
		update outbox_events
		   set status = $2, updated_at = now()
		  from next_event
		 where outbox_events.id = next_event.id
		returning outbox_events.id,
		          outbox_events.event_type,
		          outbox_events.correlation_id,
		          outbox_events.payload,
		          outbox_events.status,
		          outbox_events.attempts,
		          outbox_events.created_at`,
		domain.OutboxPending,
		domain.OutboxProcessing,
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
		  where id = $1
		    and status = $3`,
		eventID,
		domain.OutboxPublished,
		domain.OutboxProcessing,
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
		  where id = $1
		    and status = $5`,
		eventID,
		maxAttempts,
		domain.OutboxDeadLettered,
		domain.OutboxPending,
		domain.OutboxProcessing,
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

	transaction, err := lockPendingTransaction(ctx, dbtx, event)
	if err != nil {
		return nil, err
	}
	if err := ensureAvailableFunds(ctx, dbtx, transaction); err != nil {
		return nil, err
	}

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
			return nil, fmt.Errorf("insert ledger entry: %w", err)
		}
	}

	tag, err := dbtx.Exec(
		ctx,
		`update transactions
		    set status = $2, risk_level = $3, risk_reason = $4, updated_at = now()
		  where id = $1 and status = $5`,
		transaction.ID,
		domain.TransactionAuthorized,
		event.RiskLevel,
		event.Reason,
		domain.TransactionPending,
	)
	if err != nil {
		return nil, fmt.Errorf("mark transaction authorized: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return nil, fmt.Errorf("mark transaction authorized: transaction is not pending")
	}

	if err := dbtx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit ledger transaction: %w", err)
	}
	return entries, nil
}

func (s *Store) Fail(ctx context.Context, event domain.RiskEvaluated) error {
	dbtx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin fail transaction: %w", err)
	}
	defer func() {
		_ = dbtx.Rollback(ctx)
	}()

	transaction, err := lockPendingTransaction(ctx, dbtx, event)
	if err != nil {
		return err
	}

	tag, err := dbtx.Exec(
		ctx,
		`update transactions
		    set status = $2, risk_level = $3, risk_reason = $4, updated_at = now()
		  where id = $1 and status = $5`,
		transaction.ID,
		domain.TransactionFailed,
		event.RiskLevel,
		event.Reason,
		domain.TransactionPending,
	)
	if err != nil {
		return fmt.Errorf("mark transaction failed: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("mark transaction failed: transaction is not pending")
	}
	if err := dbtx.Commit(ctx); err != nil {
		return fmt.Errorf("commit fail transaction: %w", err)
	}
	return nil
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

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
