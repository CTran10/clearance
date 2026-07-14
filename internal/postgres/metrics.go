package postgres

import (
	"context"
	"fmt"

	"github.com/CTran10/clearance/internal/metrics"
)

func (s *Store) OperationalMetrics(ctx context.Context) (metrics.OperationalSnapshot, error) {
	var snapshot metrics.OperationalSnapshot
	var outboxPending int64
	var outboxDeadLettered int64
	var deadLettersOpen int64
	var processedEvents int64
	if err := s.pool.QueryRow(ctx, `
		select
			count(*) filter (where status = 'PENDING'),
			count(*) filter (where status = 'DEAD_LETTERED'),
			coalesce(extract(epoch from now() - min(created_at) filter (where status = 'PENDING')), 0)
		from outbox_events
	`).Scan(&outboxPending, &outboxDeadLettered, &snapshot.OutboxOldestPendingAgeSeconds); err != nil {
		return metrics.OperationalSnapshot{}, fmt.Errorf("query outbox metrics: %w", err)
	}
	if err := s.pool.QueryRow(ctx, `select count(*) from dead_letter_messages where state = 'OPEN'`).Scan(&deadLettersOpen); err != nil {
		return metrics.OperationalSnapshot{}, fmt.Errorf("query dead-letter metrics: %w", err)
	}
	if err := s.pool.QueryRow(ctx, `select count(*) from processed_events`).Scan(&processedEvents); err != nil {
		return metrics.OperationalSnapshot{}, fmt.Errorf("query processed-event metrics: %w", err)
	}

	poolStats := s.pool.Stat()
	snapshot.OutboxPending = float64(outboxPending)
	snapshot.OutboxDeadLettered = float64(outboxDeadLettered)
	snapshot.DeadLettersOpen = float64(deadLettersOpen)
	snapshot.ProcessedEvents = float64(processedEvents)
	snapshot.PostgresPoolOpen = float64(poolStats.TotalConns())
	snapshot.PostgresPoolIdle = float64(poolStats.IdleConns())
	snapshot.PostgresPoolInUse = float64(poolStats.AcquiredConns())
	return snapshot, nil
}
