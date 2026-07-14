package outbox

import (
	"context"
	"errors"
	"testing"

	"github.com/CTran10/clearance/internal/domain"
)

func TestPublisherMarksEventPublishedOnSuccess(t *testing.T) {
	t.Parallel()

	store := NewMemoryStore()
	event := domain.NewOutboxEvent(domain.EventTransactionCreated, "txn_123", "acct_123", "trace_123", []byte(`{"id":"txn_123"}`))
	store.AddPending(event)
	broker := NewRecordingBroker()
	publisher := NewPublisher(store, broker.Publish, Config{MaxAttempts: 3})

	found, err := publisher.PublishNext(context.Background())
	if err != nil {
		t.Fatalf("PublishNext returned error: %v", err)
	}
	if !found {
		t.Fatal("PublishNext should report pending work")
	}

	events := broker.Events()
	if len(events) != 1 || events[0].ID != event.ID {
		t.Fatalf("published events = %#v, want event %s", events, event.ID)
	}
	if got := store.EventStatus(event.ID); got != domain.OutboxPublished {
		t.Fatalf("event status = %q, want %q", got, domain.OutboxPublished)
	}
}

func TestPublisherRetriesThenMovesEventToDeadLetter(t *testing.T) {
	t.Parallel()

	store := NewMemoryStore()
	event := domain.NewOutboxEvent(domain.EventTransactionCreated, "txn_123", "acct_123", "trace_123", []byte(`{"id":"txn_123"}`))
	store.AddPending(event)
	broker := NewFailingBroker(errors.New("broker unavailable"))
	publisher := NewPublisher(store, broker.Publish, Config{MaxAttempts: 3})

	for range 3 {
		if _, err := publisher.PublishNext(context.Background()); err == nil {
			t.Fatal("PublishNext should return the broker error before DLQ")
		}
	}

	if got := store.EventStatus(event.ID); got != domain.OutboxDeadLettered {
		t.Fatalf("event status = %q, want %q", got, domain.OutboxDeadLettered)
	}
	if got := store.Attempts(event.ID); got != 3 {
		t.Fatalf("attempts = %d, want 3", got)
	}
}

func TestPublisherDrainsAllAvailableEvents(t *testing.T) {
	t.Parallel()

	store := NewMemoryStore()
	for _, transactionID := range []string{"txn_1", "txn_2", "txn_3"} {
		store.AddPending(domain.NewOutboxEvent(
			domain.EventTransactionCreated,
			transactionID,
			"acct_123",
			"trace_123",
			[]byte(`{"id":"`+transactionID+`"}`),
		))
	}
	broker := NewRecordingBroker()
	publisher := NewPublisher(store, broker.Publish, Config{MaxAttempts: 3})

	if err := publisher.PublishAvailable(context.Background()); err != nil {
		t.Fatalf("PublishAvailable returned error: %v", err)
	}
	if got := len(broker.Events()); got != 3 {
		t.Fatalf("published event count = %d, want 3", got)
	}
}

func TestPublisherStopsDrainingAfterPublishFailure(t *testing.T) {
	t.Parallel()

	store := NewMemoryStore()
	event := domain.NewOutboxEvent(domain.EventTransactionCreated, "txn_1", "acct_123", "trace_123", []byte(`{"id":"txn_1"}`))
	store.AddPending(event)
	publisher := NewPublisher(store, NewFailingBroker(errors.New("broker unavailable")).Publish, Config{MaxAttempts: 3})

	if err := publisher.PublishAvailable(context.Background()); err == nil {
		t.Fatal("PublishAvailable should return the broker error")
	}
	if got := store.Attempts(event.ID); got != 1 {
		t.Fatalf("attempts = %d, want one attempt before the next poll", got)
	}
}
