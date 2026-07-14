package maintenance

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestProcessedEventsServiceFailsClosedOnUnsafeRetention(t *testing.T) {
	t.Parallel()

	_, err := NewProcessedEventsService(&maintenanceStore{}, Config{
		Retention: 7 * 24 * time.Hour, ReplayWindow: 14 * 24 * time.Hour, BatchSize: 100,
	})
	if !errors.Is(err, ErrUnsafeRetention) {
		t.Fatalf("NewProcessedEventsService error = %v, want ErrUnsafeRetention", err)
	}
}

func TestProcessedEventsPreviewAndPruneAreBounded(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	store := &maintenanceStore{eligible: 250}
	service, err := NewProcessedEventsService(store, Config{
		Retention: 30 * 24 * time.Hour, ReplayWindow: 14 * 24 * time.Hour, BatchSize: 100,
		Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("NewProcessedEventsService returned error: %v", err)
	}
	preview, err := service.Preview(context.Background())
	if err != nil || preview.Eligible != 250 || preview.Cutoff != now.Add(-30*24*time.Hour) {
		t.Fatalf("Preview = %#v, %v", preview, err)
	}
	result, err := service.Prune(context.Background(), "retention boundary acknowledged")
	if err != nil {
		t.Fatalf("Prune returned error: %v", err)
	}
	if result.Deleted != 100 || store.batchSize != 100 || store.reason == "" {
		t.Fatalf("Prune result/store = %#v/%#v", result, store)
	}
}

type maintenanceStore struct {
	eligible  int64
	batchSize int
	reason    string
}

func (s *maintenanceStore) ProcessedEventStats(context.Context) (Stats, error) {
	return Stats{Total: s.eligible}, nil
}

func (s *maintenanceStore) PreviewProcessedPrune(context.Context, time.Time) (int64, error) {
	return s.eligible, nil
}

func (s *maintenanceStore) PruneProcessedEvents(_ context.Context, _ time.Time, batchSize int, reason string) (int64, error) {
	s.batchSize = batchSize
	s.reason = reason
	deleted := int64(batchSize)
	if s.eligible < deleted {
		deleted = s.eligible
	}
	s.eligible -= deleted
	return deleted, nil
}
