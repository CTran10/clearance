package domain

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
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
	OutboxPublished    OutboxStatus = "PUBLISHED"
	OutboxDeadLettered OutboxStatus = "DEAD_LETTERED"
)

type RiskEvaluation struct {
	Level    RiskLevel
	Approved bool
	Reason   string
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
	ID            string
	AccountID     string
	MerchantID    string
	AmountCents   int64
	Currency      string
	Status        TransactionStatus
	CorrelationID string
	CreatedAt     time.Time
}

type OutboxEvent struct {
	ID            string
	Type          EventType
	CorrelationID string
	Payload       []byte
	Status        OutboxStatus
	Attempts      int
	CreatedAt     time.Time
}

func NewOutboxEvent(eventType EventType, correlationID string, payload []byte) OutboxEvent {
	return OutboxEvent{
		ID:            NewID("evt"),
		Type:          eventType,
		CorrelationID: correlationID,
		Payload:       append([]byte(nil), payload...),
		Status:        OutboxPending,
		CreatedAt:     time.Now().UTC(),
	}
}

type RiskEvaluated struct {
	TransactionID string
	AccountID     string
	AmountCents   int64
	Currency      string
	RiskLevel     RiskLevel
	Approved      bool
	Reason        string
	CorrelationID string
}

type LedgerEntry struct {
	ID            string
	TransactionID string
	AccountID     string
	AmountCents   int64
	Currency      string
	CreatedAt     time.Time
}

type Event struct {
	ID            string
	Type          EventType
	CorrelationID string
	Payload       []byte
}

func NewEvent(eventType EventType, correlationID string, payload []byte) Event {
	return Event{
		ID:            NewID("msg"),
		Type:          eventType,
		CorrelationID: correlationID,
		Payload:       append([]byte(nil), payload...),
	}
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
