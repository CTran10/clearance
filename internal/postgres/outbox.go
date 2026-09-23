package postgres

import (
	"context"
	"fmt"

	"github.com/CTran10/clearance/internal/domain"
	"github.com/CTran10/clearance/internal/operations"
	"github.com/jackc/pgx/v5"
)

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
		          outbox_events.aggregate_id,
		          outbox_events.partition_key,
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
		&event.AggregateID,
		&event.PartitionKey,
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

func (s *Store) ListDeadOutbox(ctx context.Context, limit int) ([]domain.OutboxEvent, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := s.pool.Query(
		ctx,
		`select id, event_type, aggregate_id, partition_key, correlation_id, payload,
		        status, attempts, coalesce(last_error, ''), created_at
		   from outbox_events
		  where status = $1
		  order by created_at desc, id desc
		  limit $2`,
		domain.OutboxDeadLettered,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list dead-lettered outbox events: %w", err)
	}
	defer rows.Close()
	items := make([]domain.OutboxEvent, 0, limit)
	for rows.Next() {
		var event domain.OutboxEvent
		if err := rows.Scan(
			&event.ID,
			&event.Type,
			&event.AggregateID,
			&event.PartitionKey,
			&event.CorrelationID,
			&event.Payload,
			&event.Status,
			&event.Attempts,
			&event.LastError,
			&event.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan dead-lettered outbox event: %w", err)
		}
		items = append(items, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate dead-lettered outbox events: %w", err)
	}
	return items, nil
}

func (s *Store) GetOutboxEvent(ctx context.Context, id string) (domain.OutboxEvent, bool, error) {
	var event domain.OutboxEvent
	err := s.pool.QueryRow(
		ctx,
		`select id, event_type, aggregate_id, partition_key, correlation_id, payload,
		        status, attempts, coalesce(last_error, ''), created_at
		   from outbox_events
		  where id = $1`,
		id,
	).Scan(
		&event.ID,
		&event.Type,
		&event.AggregateID,
		&event.PartitionKey,
		&event.CorrelationID,
		&event.Payload,
		&event.Status,
		&event.Attempts,
		&event.LastError,
		&event.CreatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return domain.OutboxEvent{}, false, nil
		}
		return domain.OutboxEvent{}, false, fmt.Errorf("get outbox event: %w", err)
	}
	return event, true, nil
}

func (s *Store) GetOutboxStatus(ctx context.Context, id string) (domain.OutboxStatus, bool, error) {
	var status domain.OutboxStatus
	err := s.pool.QueryRow(ctx, `select status from outbox_events where id = $1`, id).Scan(&status)
	if err != nil {
		if err == pgx.ErrNoRows {
			return "", false, nil
		}
		return "", false, fmt.Errorf("get outbox status: %w", err)
	}
	return status, true, nil
}

func (s *Store) RequeueOutbox(ctx context.Context, id, reason string) error {
	dbtx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin requeue outbox: %w", err)
	}
	defer func() { _ = dbtx.Rollback(ctx) }()
	tag, err := dbtx.Exec(
		ctx,
		`update outbox_events
		    set status = $2, attempts = 0, last_error = null, updated_at = now()
		  where id = $1 and status = $3`,
		id,
		domain.OutboxPending,
		domain.OutboxDeadLettered,
	)
	if err != nil {
		return fmt.Errorf("requeue outbox event: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return operations.ErrInvalidState
	}
	if _, err := dbtx.Exec(
		ctx,
		`insert into operator_actions (id, action_type, target_id, reason)
		 values ($1, 'OUTBOX_REQUEUE', $2, $3)`,
		domain.NewID("act"),
		id,
		reason,
	); err != nil {
		return fmt.Errorf("audit outbox requeue: %w", err)
	}
	if err := dbtx.Commit(ctx); err != nil {
		return fmt.Errorf("commit outbox requeue: %w", err)
	}
	return nil
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
