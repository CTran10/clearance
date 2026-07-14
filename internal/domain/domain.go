package domain

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

var (
	ErrInsufficientFunds     = errors.New("insufficient funds")
	ErrEventIdentityConflict = errors.New("event id reused with different payload")
)

type TransactionStatus string

const (
	TransactionPending    TransactionStatus = "PENDING"
	TransactionAuthorized TransactionStatus = "AUTHORIZED"
	TransactionFailed     TransactionStatus = "FAILED"
)

type RiskLevel string

const (
	RiskLow  RiskLevel = "LOW"
	RiskHigh RiskLevel = "HIGH"
)

type EventType string

const (
	EventTransactionCreated    EventType = "TransactionCreated"
	EventRiskEvaluated         EventType = "RiskEvaluated"
	EventTransactionAuthorized EventType = "TransactionAuthorized"
	EventTransactionFailed     EventType = "TransactionFailed"
)

type OutboxStatus string

const (
	OutboxPending      OutboxStatus = "PENDING"
	OutboxProcessing   OutboxStatus = "PROCESSING"
	OutboxPublished    OutboxStatus = "PUBLISHED"
	OutboxDeadLettered OutboxStatus = "DEAD_LETTERED"
)

type RiskEvaluation struct {
	Level    RiskLevel `json:"level"`
	Approved bool      `json:"approved"`
	Reason   string    `json:"reason"`
}

func EvaluateRisk(amountCents int64) RiskEvaluation {
	// 50_000 cents = $500.00. go lets you put _ in numbers purely so your eyeballs can find the comma — it's
	// the same as 50000, the underscore is invisible to the compiler. (i kept reading it as 50k DOLLARS at first lol)
	if amountCents > 50_000 {
		return RiskEvaluation{
			Level:    RiskHigh,
			Approved: false,
			Reason:   "amount is greater than 500.00",
		}
	}

	return RiskEvaluation{
		Level:    RiskLow,
		Approved: true,
		Reason:   "amount is at or below 500.00",
	}
}

type Transaction struct {
	ID            string            `json:"id"`
	AccountID     string            `json:"account_id"`
	MerchantID    string            `json:"merchant_id"`
	AmountCents   int64             `json:"amount_cents"`
	Currency      string            `json:"currency"`
	Status        TransactionStatus `json:"status"`
	CorrelationID string            `json:"correlation_id"`
	CreatedAt     time.Time         `json:"created_at"`
}

type OutboxEvent struct {
	ID            string       `json:"id"`
	Type          EventType    `json:"type"`
	AggregateID   string       `json:"aggregate_id"`
	PartitionKey  string       `json:"partition_key"`
	CorrelationID string       `json:"correlation_id"`
	Payload       []byte       `json:"payload"`
	Status        OutboxStatus `json:"status"`
	Attempts      int          `json:"attempts"`
	CreatedAt     time.Time    `json:"created_at"`
}

func NewOutboxEvent(
	eventType EventType,
	aggregateID string,
	partitionKey string,
	correlationID string,
	payload []byte,
) OutboxEvent {
	return OutboxEvent{
		ID:            NewID("evt"),
		Type:          eventType,
		AggregateID:   aggregateID,
		PartitionKey:  partitionKey,
		CorrelationID: correlationID,
		Payload:       append([]byte(nil), payload...), // defensive copy! go slices share their backing array, so if i just stored `payload` and the caller mutated their copy later, MY event would silently change too. append-onto-nil = fresh array nobody else holds
		Status:        OutboxPending,
		CreatedAt:     time.Now().UTC(),
	}
}

type RiskEvaluated struct {
	TransactionID string    `json:"transaction_id"`
	AccountID     string    `json:"account_id"`
	AmountCents   int64     `json:"amount_cents"`
	Currency      string    `json:"currency"`
	RiskLevel     RiskLevel `json:"risk_level"`
	Approved      bool      `json:"approved"`
	Reason        string    `json:"reason"`
	CorrelationID string    `json:"correlation_id"`
}

type LedgerEntry struct {
	ID            string    `json:"id"`
	TransactionID string    `json:"transaction_id"`
	AccountID     string    `json:"account_id"`
	AmountCents   int64     `json:"amount_cents"`
	Currency      string    `json:"currency"`
	CreatedAt     time.Time `json:"created_at"`
}

func NewID(prefix string) string {
	// "crypto/rand" not "math/rand"!! math/rand is predictable — seed it the same and you get the same "random"
	// numbers, which for ids people might guess is a disaster. crypto/rand is the real unpredictable stuff.
	// the import names are almost identical so this is an easy footgun. 16 random bytes = basically zero collision odds
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		// rand.Read basically never fails, but Go makes me handle the error anyway, so: if entropy somehow dies,
		// fall back to a timestamp. ugly and technically guessable but better than crashing the whole service
		return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
	}
	return prefix + "_" + hex.EncodeToString(bytes[:])
}
