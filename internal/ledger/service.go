package ledger

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/CTran10/clearance/internal/domain"
)

type Store interface {
	Authorize(ctx context.Context, event domain.RiskEvaluated) ([]domain.LedgerEntry, error)
	Fail(ctx context.Context, event domain.RiskEvaluated) error
}

type Publisher interface {
	Publish(ctx context.Context, event domain.Event) error
}

type Service struct {
	store     Store
	publisher Publisher
}

func NewService(store Store, publisher Publisher) *Service {
	return &Service{store: store, publisher: publisher}
}

func (s *Service) HandleRiskEvaluated(ctx context.Context, event domain.RiskEvaluated) error {
	if event.Approved && event.RiskLevel == domain.RiskLow {
		if _, err := s.store.Authorize(ctx, event); err != nil {
			return fmt.Errorf("authorize transaction: %w", err)
		}
		return s.publish(ctx, domain.EventTransactionAuthorized, event)
	}

	if err := s.store.Fail(ctx, event); err != nil {
		return fmt.Errorf("fail transaction: %w", err)
	}
	return s.publish(ctx, domain.EventTransactionFailed, event)
}

func (s *Service) publish(ctx context.Context, eventType domain.EventType, evaluated domain.RiskEvaluated) error {
	payload, err := json.Marshal(evaluated)
	if err != nil {
		return fmt.Errorf("marshal ledger event: %w", err)
	}
	if err := s.publisher.Publish(ctx, domain.NewEvent(eventType, evaluated.CorrelationID, payload)); err != nil {
		return fmt.Errorf("publish ledger event: %w", err)
	}
	return nil
}

type MemoryStore struct {
	mu      sync.Mutex
	entries []domain.LedgerEntry
	status  map[string]domain.TransactionStatus
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{status: make(map[string]domain.TransactionStatus)}
}

func (s *MemoryStore) Authorize(_ context.Context, event domain.RiskEvaluated) ([]domain.LedgerEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	entries := []domain.LedgerEntry{
		{
			ID:            domain.NewID("le"),
			TransactionID: event.TransactionID,
			AccountID:     event.AccountID,
			AmountCents:   -event.AmountCents,
			Currency:      event.Currency,
			CreatedAt:     now,
		},
		{
			ID:            domain.NewID("le"),
			TransactionID: event.TransactionID,
			AccountID:     "clearing",
			AmountCents:   event.AmountCents,
			Currency:      event.Currency,
			CreatedAt:     now,
		},
	}
	s.entries = append(s.entries, entries...)
	s.status[event.TransactionID] = domain.TransactionAuthorized
	return entries, nil
}

func (s *MemoryStore) Fail(_ context.Context, event domain.RiskEvaluated) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.status[event.TransactionID] = domain.TransactionFailed
	return nil
}

func (s *MemoryStore) LedgerEntries() []domain.LedgerEntry {
	s.mu.Lock()
	defer s.mu.Unlock()

	return append([]domain.LedgerEntry(nil), s.entries...)
}

func (s *MemoryStore) TransactionStatus(transactionID string) domain.TransactionStatus {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.status[transactionID]
}

type RecordingPublisher struct {
	mu     sync.Mutex
	events []domain.Event
}

func NewRecordingPublisher() *RecordingPublisher {
	return &RecordingPublisher{}
}

func (p *RecordingPublisher) Publish(_ context.Context, event domain.Event) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.events = append(p.events, event)
	return nil
}

func (p *RecordingPublisher) Events() []domain.Event {
	p.mu.Lock()
	defer p.mu.Unlock()

	return append([]domain.Event(nil), p.events...)
}
