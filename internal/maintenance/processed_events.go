package maintenance

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

var ErrUnsafeRetention = errors.New("processed-event retention is shorter than replay window")

type Config struct {
	Retention    time.Duration
	ReplayWindow time.Duration
	BatchSize    int
	Now          func() time.Time
}

type Stats struct {
	Total            int64     `json:"total"`
	OldestLastSeenAt time.Time `json:"oldest_last_seen_at,omitempty"`
}

type Preview struct {
	Cutoff   time.Time `json:"cutoff"`
	Eligible int64     `json:"eligible"`
}

type Result struct {
	Cutoff  time.Time `json:"cutoff"`
	Deleted int64     `json:"deleted"`
}

type Store interface {
	ProcessedEventStats(ctx context.Context) (Stats, error)
	PreviewProcessedPrune(ctx context.Context, cutoff time.Time) (int64, error)
	PruneProcessedEvents(ctx context.Context, cutoff time.Time, batchSize int, reason string) (int64, error)
}

type ProcessedEventsService struct {
	store     Store
	retention time.Duration
	batchSize int
	now       func() time.Time
}

func NewProcessedEventsService(store Store, config Config) (*ProcessedEventsService, error) {
	retention := config.Retention
	if retention <= 0 {
		retention = 30 * 24 * time.Hour
	}
	replayWindow := config.ReplayWindow
	if replayWindow <= 0 {
		replayWindow = 14 * 24 * time.Hour
	}
	if retention < replayWindow {
		return nil, ErrUnsafeRetention
	}
	batchSize := config.BatchSize
	if batchSize <= 0 {
		batchSize = 1_000
	}
	if batchSize > 10_000 {
		batchSize = 10_000
	}
	now := config.Now
	if now == nil {
		now = time.Now
	}
	return &ProcessedEventsService{store: store, retention: retention, batchSize: batchSize, now: now}, nil
}

func (s *ProcessedEventsService) Stats(ctx context.Context) (Stats, error) {
	stats, err := s.store.ProcessedEventStats(ctx)
	if err != nil {
		return Stats{}, fmt.Errorf("processed-event stats: %w", err)
	}
	return stats, nil
}

func (s *ProcessedEventsService) Preview(ctx context.Context) (Preview, error) {
	cutoff := s.now().UTC().Add(-s.retention)
	eligible, err := s.store.PreviewProcessedPrune(ctx, cutoff)
	if err != nil {
		return Preview{}, fmt.Errorf("preview processed-event prune: %w", err)
	}
	return Preview{Cutoff: cutoff, Eligible: eligible}, nil
}

func (s *ProcessedEventsService) Prune(ctx context.Context, reason string) (Result, error) {
	reason = strings.TrimSpace(reason)
	if len(reason) == 0 || len(reason) > 256 {
		return Result{}, fmt.Errorf("invalid prune reason")
	}
	cutoff := s.now().UTC().Add(-s.retention)
	deleted, err := s.store.PruneProcessedEvents(ctx, cutoff, s.batchSize, reason)
	if err != nil {
		return Result{}, fmt.Errorf("prune processed events: %w", err)
	}
	return Result{Cutoff: cutoff, Deleted: deleted}, nil
}
