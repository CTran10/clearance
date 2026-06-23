package ledger

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/CTran10/clearance/internal/domain"
)

type MemoryStore struct {
	mu           sync.Mutex
	entries      []domain.LedgerEntry
	balances     map[string]int64
	transactions map[string]domain.Transaction
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		balances:     make(map[string]int64),
		transactions: make(map[string]domain.Transaction),
	}
}

func (s *MemoryStore) AddPendingTransaction(transaction domain.Transaction) {
	s.mu.Lock()
	defer s.mu.Unlock()

	transaction.Status = domain.TransactionPending
	s.transactions[transaction.ID] = transaction
}

func (s *MemoryStore) Credit(accountID string, currency string, amountCents int64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.balances[balanceKey(accountID, currency)] += amountCents
}

func (s *MemoryStore) Authorize(_ context.Context, event domain.RiskEvaluated) ([]domain.LedgerEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	transaction, ok := s.transactions[event.TransactionID]
	if !ok {
		return nil, fmt.Errorf("transaction not found")
	}
	if transaction.Status != domain.TransactionPending {
		return nil, fmt.Errorf("transaction is not pending")
	}
	if transaction.AccountID != event.AccountID ||
		transaction.AmountCents != event.AmountCents ||
		transaction.Currency != event.Currency {
		return nil, fmt.Errorf("risk event does not match transaction")
	}
	key := balanceKey(transaction.AccountID, transaction.Currency)
	if s.balances[key] < transaction.AmountCents {
		return nil, domain.ErrInsufficientFunds
	}

	now := time.Now().UTC()
	entries := []domain.LedgerEntry{
		{
			ID:            domain.NewID("le"),
			TransactionID: transaction.ID,
			AccountID:     transaction.AccountID,
			AmountCents:   -transaction.AmountCents,
			Currency:      transaction.Currency,
			CreatedAt:     now,
		},
		{
			ID:            domain.NewID("le"),
			TransactionID: transaction.ID,
			AccountID:     "clearing",
			AmountCents:   transaction.AmountCents,
			Currency:      transaction.Currency,
			CreatedAt:     now,
		},
	}
	s.entries = append(s.entries, entries...)
	s.balances[key] -= transaction.AmountCents
	s.balances[balanceKey("clearing", transaction.Currency)] += transaction.AmountCents
	transaction.Status = domain.TransactionAuthorized
	s.transactions[transaction.ID] = transaction
	return entries, nil
}

func (s *MemoryStore) Fail(_ context.Context, event domain.RiskEvaluated) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	transaction, ok := s.transactions[event.TransactionID]
	if !ok {
		return fmt.Errorf("transaction not found")
	}
	if transaction.Status != domain.TransactionPending {
		return fmt.Errorf("transaction is not pending")
	}
	transaction.Status = domain.TransactionFailed
	s.transactions[transaction.ID] = transaction
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

	return s.transactions[transactionID].Status
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

func balanceKey(accountID string, currency string) string {
	return accountID + "\x00" + currency
}
