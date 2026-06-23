package transaction

import (
	"context"
	"sync"

	"github.com/CTran10/clearance/internal/domain"
)

type MemoryStore struct {
	mu           sync.Mutex
	idempotent   map[string]IdempotencyRecord
	transactions map[string]domain.Transaction
	outbox       []domain.OutboxEvent
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		idempotent:   make(map[string]IdempotencyRecord),
		transactions: make(map[string]domain.Transaction),
	}
}

func (s *MemoryStore) FindIdempotency(_ context.Context, key string) (IdempotencyRecord, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	record, ok := s.idempotent[key]
	return record, ok, nil
}

func (s *MemoryStore) Create(_ context.Context, record IdempotencyRecord, event domain.OutboxEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if existing, ok := s.idempotent[record.Key]; ok {
		if existing.RequestHash != record.RequestHash {
			return ErrIdempotencyConflict
		}
		return nil
	}
	s.idempotent[record.Key] = record
	s.transactions[record.Transaction.ID] = record.Transaction
	s.outbox = append(s.outbox, event)
	return nil
}

func (s *MemoryStore) OutboxEvents() []domain.OutboxEvent {
	s.mu.Lock()
	defer s.mu.Unlock()

	return append([]domain.OutboxEvent(nil), s.outbox...)
}
