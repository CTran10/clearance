package outbox

import (
	"context"
	"fmt"
	"sync"

	"github.com/CTran10/clearance/internal/domain"
)

type Config struct {
	MaxAttempts int
}

type Store interface {
	NextPending(ctx context.Context) (domain.OutboxEvent, bool, error)
	MarkPublished(ctx context.Context, eventID string) error
	MarkFailedAttempt(ctx context.Context, eventID string, maxAttempts int) error
}

type Broker interface {
	Publish(ctx context.Context, event domain.OutboxEvent) error
}

type Publisher struct {
	store       Store
	broker      Broker
	maxAttempts int
}

func NewPublisher(store Store, broker Broker, config Config) *Publisher {
	maxAttempts := config.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 3
	}
	return &Publisher{store: store, broker: broker, maxAttempts: maxAttempts}
}

func (p *Publisher) PublishNext(ctx context.Context) error {
	event, ok, err := p.store.NextPending(ctx)
	if err != nil {
		return fmt.Errorf("load pending outbox event: %w", err)
	}
	if !ok {
		return nil
	}

	// if kafka's having a moment and publish fails, we DON'T just retry forever — that's how one poison event
	// jams the whole queue. MarkFailedAttempt bumps a counter and once it hits maxAttempts the event gets
	// "dead lettered" (parked aside) so the line keeps moving. learned the term "poison message" from this exact problem
	if err := p.broker.Publish(ctx, event); err != nil {
		if markErr := p.store.MarkFailedAttempt(ctx, event.ID, p.maxAttempts); markErr != nil {
			return fmt.Errorf("mark failed outbox event: %w", markErr)
		}
		// note the %w — wrapping the error keeps the original cause attached so callers can errors.Is/As it later.
		// took me a bit to stop just doing fmt.Errorf("...%v") and losing the actual error underneath
		return fmt.Errorf("publish outbox event: %w", err)
	}

	if err := p.store.MarkPublished(ctx, event.ID); err != nil {
		return fmt.Errorf("mark published outbox event: %w", err)
	}
	return nil
}

type MemoryStore struct {
	mu     sync.Mutex
	events []domain.OutboxEvent
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{}
}

func (s *MemoryStore) AddPending(event domain.OutboxEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()

	event.Status = domain.OutboxPending
	s.events = append(s.events, event)
}

func (s *MemoryStore) NextPending(_ context.Context) (domain.OutboxEvent, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, event := range s.events {
		if event.Status == domain.OutboxPending {
			return event, true, nil
		}
	}
	return domain.OutboxEvent{}, false, nil
}

func (s *MemoryStore) MarkPublished(_ context.Context, eventID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.events {
		if s.events[i].ID == eventID {
			s.events[i].Status = domain.OutboxPublished
			return nil
		}
	}
	return fmt.Errorf("outbox event %s not found", eventID)
}

func (s *MemoryStore) MarkFailedAttempt(_ context.Context, eventID string, maxAttempts int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.events {
		if s.events[i].ID == eventID {
			s.events[i].Attempts++
			if s.events[i].Attempts >= maxAttempts {
				s.events[i].Status = domain.OutboxDeadLettered
			}
			return nil
		}
	}
	return fmt.Errorf("outbox event %s not found", eventID)
}

func (s *MemoryStore) EventStatus(eventID string) domain.OutboxStatus {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, event := range s.events {
		if event.ID == eventID {
			return event.Status
		}
	}
	return ""
}

func (s *MemoryStore) Attempts(eventID string) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, event := range s.events {
		if event.ID == eventID {
			return event.Attempts
		}
	}
	return 0
}

type RecordingBroker struct {
	mu     sync.Mutex
	events []domain.OutboxEvent
}

func NewRecordingBroker() *RecordingBroker {
	return &RecordingBroker{}
}

func (b *RecordingBroker) Publish(_ context.Context, event domain.OutboxEvent) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.events = append(b.events, event)
	return nil
}

func (b *RecordingBroker) Events() []domain.OutboxEvent {
	b.mu.Lock()
	defer b.mu.Unlock()

	return append([]domain.OutboxEvent(nil), b.events...)
}

type FailingBroker struct {
	err error
}

func NewFailingBroker(err error) *FailingBroker {
	return &FailingBroker{err: err}
}

func (b *FailingBroker) Publish(context.Context, domain.OutboxEvent) error {
	return b.err
}
