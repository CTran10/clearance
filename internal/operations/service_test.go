package operations

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/CTran10/clearance/internal/deadletter"
	"github.com/CTran10/clearance/internal/domain"
	"github.com/segmentio/kafka-go"
)

func TestServiceReplaysExactDeadLetterAndAuditsAttempt(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	record := deadletter.Record{
		ID: "dlq_123", EventID: "evt_123", ConsumerName: "risk-service",
		SourceTopic: "transactions.created", SourcePartition: 1, SourceOffset: 9,
		Key: []byte("acct_123"), Payload: []byte(`{"id":"txn_123"}`),
		Headers: []kafka.Header{{Key: "event_id", Value: []byte("evt_123")}, {Key: "correlation_id", Value: []byte("trace_123")}},
		State:   deadletter.StateOpen, FirstFailedAt: now.Add(-time.Hour), KafkaPublishedAt: now.Add(-time.Hour),
	}
	store := &operationStore{deadLetter: record}
	broker := &recordingBroker{}
	service := NewService(store, broker, Config{ReplayWindow: 14 * 24 * time.Hour, Now: func() time.Time { return now }})

	result, err := service.ReplayDeadLetter(context.Background(), record.ID, "dependency repaired")
	if err != nil {
		t.Fatalf("ReplayDeadLetter returned error: %v", err)
	}
	if result.State != deadletter.StateRepublished || broker.topic != record.SourceTopic {
		t.Fatalf("result/broker = %#v/%#v", result, broker)
	}
	if string(broker.message.Key) != string(record.Key) || string(broker.message.Value) != string(record.Payload) || len(broker.message.Headers) != len(record.Headers) {
		t.Fatalf("replayed message changed: %#v", broker.message)
	}
	if store.replayReason != "dependency repaired" || store.replayResult != ReplayPublished {
		t.Fatalf("replay audit = %q/%q", store.replayReason, store.replayResult)
	}
}

func TestServiceRefusesProcessedOrExpiredReplayAndRequeuesOnlyDeadOutbox(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	store := &operationStore{deadLetter: deadletter.Record{
		ID: "dlq_123", EventID: "evt_123", SourceTopic: "transactions.created",
		State: deadletter.StateOpen, FirstFailedAt: now.Add(-time.Hour),
	}}
	service := NewService(store, &recordingBroker{}, Config{ReplayWindow: 14 * 24 * time.Hour, Now: func() time.Time { return now }})

	store.processed = true
	if _, err := service.ReplayDeadLetter(context.Background(), "dlq_123", "retry"); !errors.Is(err, ErrAlreadyProcessed) {
		t.Fatalf("processed replay error = %v, want ErrAlreadyProcessed", err)
	}
	store.processed = false
	store.deadLetter.FirstFailedAt = now.Add(-15 * 24 * time.Hour)
	if _, err := service.ReplayDeadLetter(context.Background(), "dlq_123", "retry"); !errors.Is(err, ErrReplayWindowExpired) {
		t.Fatalf("expired replay error = %v, want ErrReplayWindowExpired", err)
	}

	store.outboxStatus = domain.OutboxPublished
	if _, err := service.RequeueOutbox(context.Background(), "evt_outbox", "broker repaired"); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("published requeue error = %v, want ErrInvalidState", err)
	}
	store.outboxStatus = domain.OutboxDeadLettered
	if _, err := service.RequeueOutbox(context.Background(), "evt_outbox", "broker repaired"); err != nil {
		t.Fatalf("dead outbox requeue returned error: %v", err)
	}
	if store.outboxStatus != domain.OutboxPending {
		t.Fatalf("outbox status = %q, want PENDING", store.outboxStatus)
	}
}

func TestServiceRejectsInvalidReplayAndRecordsPublishFailure(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	store := &operationStore{deadLetter: deadletter.Record{
		ID: "dlq_123", SourceTopic: "transactions.created", State: deadletter.StateOpen,
		FirstFailedAt: now.Add(-time.Hour),
	}}
	broker := &recordingBroker{err: errors.New("broker unavailable")}
	service := NewService(store, broker, Config{Now: func() time.Time { return now }})

	if _, err := service.ReplayDeadLetter(context.Background(), "dlq_123", ""); !errors.Is(err, ErrInvalidReason) {
		t.Fatalf("blank reason error = %v, want ErrInvalidReason", err)
	}
	if _, err := service.ReplayDeadLetter(context.Background(), "dlq_123", strings.Repeat("x", 257)); !errors.Is(err, ErrInvalidReason) {
		t.Fatalf("oversized reason error = %v, want ErrInvalidReason", err)
	}
	if _, err := service.ReplayDeadLetter(context.Background(), "dlq_123", "broker recovered"); err == nil {
		t.Fatal("broker failure should be returned")
	}
	if store.replayResult != ReplayFailed || store.replayError == "" {
		t.Fatalf("failed replay audit = %q/%q", store.replayResult, store.replayError)
	}

	store.deadLetter.State = deadletter.StateRepublished
	broker.err = nil
	if _, err := service.ReplayDeadLetter(context.Background(), "dlq_123", "retry"); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("non-open replay error = %v, want ErrInvalidState", err)
	}
	store.deadLetter = deadletter.Record{}
	if _, err := service.ReplayDeadLetter(context.Background(), "missing", "retry"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing replay error = %v, want ErrNotFound", err)
	}
}

type operationStore struct {
	deadLetter   deadletter.Record
	processed    bool
	replayReason string
	replayResult ReplayResult
	replayError  string
	outboxStatus domain.OutboxStatus
}

func (s *operationStore) GetDeadLetter(context.Context, string) (deadletter.Record, bool, error) {
	return s.deadLetter, s.deadLetter.ID != "", nil
}

func (s *operationStore) IsEventProcessed(context.Context, string) (bool, error) {
	return s.processed, nil
}

func (s *operationStore) StartDeadLetterReplay(_ context.Context, id, reason string) (string, error) {
	s.replayReason = reason
	return "replay_123", nil
}

func (s *operationStore) FinishDeadLetterReplay(_ context.Context, _, _ string, result ReplayResult, errorMessage string) error {
	s.replayResult = result
	s.replayError = errorMessage
	if result == ReplayPublished {
		s.deadLetter.State = deadletter.StateRepublished
	}
	return nil
}

func (s *operationStore) GetOutboxStatus(context.Context, string) (domain.OutboxStatus, bool, error) {
	return s.outboxStatus, true, nil
}

func (s *operationStore) RequeueOutbox(_ context.Context, _ string, _ string) error {
	s.outboxStatus = domain.OutboxPending
	return nil
}

type recordingBroker struct {
	topic   string
	message kafka.Message
	err     error
}

func (b *recordingBroker) PublishMessage(_ context.Context, topic string, message kafka.Message) error {
	b.topic = topic
	b.message = message
	return b.err
}
