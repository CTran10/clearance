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
	event := domain.NewOutboxEvent(domain.EventTransactionCreated, "trace_123", []byte(`{"id":"txn_123"}`))
	store.AddPending(event)
	broker := NewRecordingBroker()
	publisher := NewPublisher(store, broker.Publish, Config{MaxAttempts: 3})

	if err := publisher.PublishNext(context.Background()); err != nil {
		t.Fatalf("PublishNext returned error: %v", err)
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
	event := domain.NewOutboxEvent(domain.EventTransactionCreated, "trace_123", []byte(`{"id":"txn_123"}`))
	store.AddPending(event)
	broker := NewFailingBroker(errors.New("broker unavailable"))
	publisher := NewPublisher(store, broker.Publish, Config{MaxAttempts: 3})

	for range 3 {
		if err := publisher.PublishNext(context.Background()); err == nil {
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
