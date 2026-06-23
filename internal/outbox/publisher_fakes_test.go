package outbox

import (
	"context"
	"fmt"
	"sync"

	"github.com/CTran10/clearance/internal/domain"
)

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

	for i := range s.events {
		if s.events[i].Status == domain.OutboxPending {
			s.events[i].Status = domain.OutboxProcessing
			return s.events[i], true, nil
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
			} else {
				s.events[i].Status = domain.OutboxPending
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
