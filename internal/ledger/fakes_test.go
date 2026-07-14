package ledger

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/CTran10/clearance/internal/consumer"
	"github.com/CTran10/clearance/internal/domain"
)

type MemoryStore struct {
	mu           sync.Mutex
	entries      []domain.LedgerEntry
	balances     map[string]int64
	transactions map[string]domain.Transaction
	processed    map[string]string
	outbox       []domain.OutboxEvent
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		balances:     make(map[string]int64),
		transactions: make(map[string]domain.Transaction),
		processed:    make(map[string]string),
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

func (s *MemoryStore) ProcessRiskEvaluated(
	_ context.Context,
	delivery consumer.Delivery,
	payloadHash string,
	event domain.RiskEvaluated,
) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if existing, ok := s.processed[delivery.EventID]; ok {
		if existing != payloadHash {
			return false, domain.ErrEventIdentityConflict
		}
		return false, nil
	}
	transaction, ok := s.transactions[event.TransactionID]
	if !ok {
		return false, fmt.Errorf("transaction not found")
	}
	if transaction.Status != domain.TransactionPending {
		return false, fmt.Errorf("transaction is not pending")
	}
	if transaction.AccountID != event.AccountID || transaction.AmountCents != event.AmountCents || transaction.Currency != event.Currency {
		return false, fmt.Errorf("risk event does not match transaction")
	}

	outcome := event
	eventType := domain.EventTransactionFailed
	if event.Approved {
		key := balanceKey(transaction.AccountID, transaction.Currency)
		if s.balances[key] >= transaction.AmountCents {
			now := time.Now().UTC()
			entries := []domain.LedgerEntry{
				{ID: domain.NewID("le"), TransactionID: transaction.ID, AccountID: transaction.AccountID, AmountCents: -transaction.AmountCents, Currency: transaction.Currency, CreatedAt: now},
				{ID: domain.NewID("le"), TransactionID: transaction.ID, AccountID: "clearing", AmountCents: transaction.AmountCents, Currency: transaction.Currency, CreatedAt: now},
			}
			s.entries = append(s.entries, entries...)
			s.balances[key] -= transaction.AmountCents
			s.balances[balanceKey("clearing", transaction.Currency)] += transaction.AmountCents
			transaction.Status = domain.TransactionAuthorized
			eventType = domain.EventTransactionAuthorized
		} else {
			transaction.Status = domain.TransactionFailed
			outcome.Approved = false
			outcome.Reason = "insufficient funds"
		}
	} else {
		transaction.Status = domain.TransactionFailed
	}
	payload, err := json.Marshal(outcome)
	if err != nil {
		return false, err
	}
	s.transactions[transaction.ID] = transaction
	s.processed[delivery.EventID] = payloadHash
	s.outbox = append(s.outbox, domain.NewOutboxEvent(
		eventType, transaction.ID, transaction.AccountID, event.CorrelationID, payload,
	))
	return true, nil
}

func (s *MemoryStore) LedgerEntries() []domain.LedgerEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]domain.LedgerEntry(nil), s.entries...)
}

func (s *MemoryStore) OutboxEvents() []domain.OutboxEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]domain.OutboxEvent(nil), s.outbox...)
}

func (s *MemoryStore) TransactionStatus(transactionID string) domain.TransactionStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.transactions[transactionID].Status
}

func balanceKey(accountID string, currency string) string {
	return accountID + "\x00" + currency
}

func ledgerDelivery(eventID string) consumer.Delivery {
	return consumer.Delivery{
		ConsumerName: "ledger-service", EventID: eventID, SourceTopic: "risk.evaluated",
		SourcePartition: 0, SourceOffset: 1,
	}
}
