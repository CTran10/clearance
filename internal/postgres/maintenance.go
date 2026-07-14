package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/CTran10/clearance/internal/domain"
	"github.com/CTran10/clearance/internal/maintenance"
	"github.com/jackc/pgx/v5"
)

func (s *Store) ProcessedEventStats(ctx context.Context) (maintenance.Stats, error) {
	var stats maintenance.Stats
	if err := s.pool.QueryRow(
		ctx,
		`select count(*), coalesce(min(last_seen_at), '0001-01-01'::timestamptz)
		   from processed_events`,
	).Scan(&stats.Total, &stats.OldestLastSeenAt); err != nil {
		return maintenance.Stats{}, fmt.Errorf("query processed-event stats: %w", err)
	}
	return stats, nil
}

func (s *Store) PreviewProcessedPrune(ctx context.Context, cutoff time.Time) (int64, error) {
	var count int64
	if err := s.pool.QueryRow(ctx, processedPruneCountSQL, cutoff).Scan(&count); err != nil {
		return 0, fmt.Errorf("preview processed-event prune: %w", err)
	}
	return count, nil
}

func (s *Store) PruneProcessedEvents(
	ctx context.Context,
	cutoff time.Time,
	batchSize int,
	reason string,
) (int64, error) {
	dbtx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return 0, fmt.Errorf("begin processed-event prune: %w", err)
	}
	defer func() { _ = dbtx.Rollback(ctx) }()
	if _, err := dbtx.Exec(ctx, `select pg_advisory_xact_lock(hashtext('processed-event-prune'))`); err != nil {
		return 0, fmt.Errorf("lock processed-event prune: %w", err)
	}
	rows, err := dbtx.Query(
		ctx,
		`with candidates as (
		    select processed.consumer_name, processed.event_id
		      from processed_events processed
		     where processed.last_seen_at < $1
		       and not exists (
		           select 1 from dead_letter_messages dead
		            where dead.event_id = processed.event_id
		              and (dead.state = 'OPEN' or dead.first_failed_at >= $1)
		       )
		     order by processed.last_seen_at, processed.consumer_name, processed.event_id
		     for update skip locked
		     limit $2
		)
		delete from processed_events processed
		 using candidates
		 where processed.consumer_name = candidates.consumer_name
		   and processed.event_id = candidates.event_id
		returning processed.event_id`,
		cutoff,
		batchSize,
	)
	if err != nil {
		return 0, fmt.Errorf("delete processed-event batch: %w", err)
	}
	var deleted int64
	for rows.Next() {
		var ignored string
		if err := rows.Scan(&ignored); err != nil {
			rows.Close()
			return 0, fmt.Errorf("scan pruned processed event: %w", err)
		}
		deleted++
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, fmt.Errorf("iterate pruned processed events: %w", err)
	}
	rows.Close()
	if _, err := dbtx.Exec(
		ctx,
		`insert into operator_actions (id, action_type, target_id, reason)
		 values ($1, 'PROCESSED_PRUNE', $2, $3)`,
		domain.NewID("act"),
		cutoff.UTC().Format(time.RFC3339),
		reason,
	); err != nil {
		return 0, fmt.Errorf("audit processed-event prune: %w", err)
	}
	if err := dbtx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit processed-event prune: %w", err)
	}
	return deleted, nil
}

const processedPruneCountSQL = `
	select count(*)
	  from processed_events processed
	 where processed.last_seen_at < $1
	   and not exists (
	       select 1 from dead_letter_messages dead
	        where dead.event_id = processed.event_id
	          and (dead.state = 'OPEN' or dead.first_failed_at >= $1)
	   )`
