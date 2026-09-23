package postgres

import (
	"context"
	"fmt"

	"github.com/CTran10/clearance/internal/consumer"
	"github.com/CTran10/clearance/internal/domain"
	"github.com/jackc/pgx/v5"
)

func (s *Store) SaveConsumerOutbox(
	ctx context.Context,
	delivery consumer.Delivery,
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

	claimed, err := claimProcessedEvent(ctx, dbtx, delivery, payloadHash)
	if err != nil {
		return false, err
	}
	if !claimed {
		if err := dbtx.Commit(ctx); err != nil {
			return false, fmt.Errorf("commit duplicate consumer event: %w", err)
		}
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

func (s *Store) IsEventProcessed(ctx context.Context, eventID string) (bool, error) {
	var processed bool
	if err := s.pool.QueryRow(
		ctx,
		`select exists(select 1 from processed_events where event_id = $1)`,
		eventID,
	).Scan(&processed); err != nil {
		return false, fmt.Errorf("check processed event: %w", err)
	}
	return processed, nil
}

func claimProcessedEvent(
	ctx context.Context,
	dbtx pgx.Tx,
	delivery consumer.Delivery,
	payloadHash string,
) (bool, error) {
	tag, err := dbtx.Exec(
		ctx,
		`insert into processed_events
			(consumer_name, event_id, payload_sha256, source_topic, source_partition, source_offset, last_seen_at)
		 values ($1, $2, $3, nullif($4, ''), $5, $6, now())
		 on conflict (consumer_name, event_id) do nothing`,
		delivery.ConsumerName,
		delivery.EventID,
		payloadHash,
		delivery.SourceTopic,
		delivery.SourcePartition,
		delivery.SourceOffset,
	)
	if err != nil {
		return false, fmt.Errorf("claim processed event: %w", err)
	}
	if tag.RowsAffected() == 1 {
		return true, nil
	}
	if _, err := dbtx.Exec(
		ctx,
		`update processed_events
		    set last_seen_at = now()
		  where consumer_name = $1 and event_id = $2`,
		delivery.ConsumerName,
		delivery.EventID,
	); err != nil {
		return false, fmt.Errorf("refresh processed event: %w", err)
	}

	var existingHash string
	if err := dbtx.QueryRow(
		ctx,
		`select payload_sha256
		   from processed_events
		  where consumer_name = $1 and event_id = $2`,
		delivery.ConsumerName,
		delivery.EventID,
	).Scan(&existingHash); err != nil {
		return false, fmt.Errorf("load processed event: %w", err)
	}
	if existingHash != payloadHash {
		return false, domain.ErrEventIdentityConflict
	}
	return false, nil
}
