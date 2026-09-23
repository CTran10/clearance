package domain

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"
)

var (
	ErrInsufficientFunds = errors.New("insufficient funds")
	ErrEventIdentityConflict = errors.New("event id reused with different payload")
)

type TransactionStatus string

const (
	TransactionPending TransactionStatus = "PENDING"
	TransactionAuthorized TransactionStatus = "AUTHORIZED"
	TransactionFailed TransactionStatus = "FAILED"
)

type TransactionKind string

const (
	TransactionPayment TransactionKind = "PAYMENT"
	TransactionDeposit TransactionKind = "DEPOSIT"
)

type RiskLevel string

const (
	RiskLow RiskLevel = "LOW"
	RiskHigh RiskLevel = "HIGH"
)

type EventType string

const (
	EventTransactionCreated EventType = "TransactionCreated"
	EventRiskEvaluated EventType = "RiskEvaluated"
	EventTransactionAuthorized EventType = "TransactionAuthorized"
	EventTransactionFailed EventType = "TransactionFailed"
	EventFundsDeposited EventType = "FundsDeposited"
)

type OutboxStatus string

const (
	OutboxPending OutboxStatus = "PENDING"
	OutboxProcessing OutboxStatus = "PROCESSING"
	OutboxPublished OutboxStatus = "PUBLISHED"
	OutboxDeadLettered OutboxStatus = "DEAD_LETTERED"
)

type RiskEvaluation struct {
	Level RiskLevel `json:"level"`
	Approved bool `json:"approved"`
	Reason string `json:"reason"`
}

func EvaluateRisk(amountCents int64) RiskEvaluation {
	if amountCents > 50_000 {
		return RiskEvaluation{
			Level: RiskHigh,
			Approved: false,
			Reason: "amount is greater than 500.00",
		}
	}

	return RiskEvaluation{
		Level: RiskLow,
		Approved: true,
		Reason: "amount is at or below 500.00",
	}
}

type Transaction struct {
	ID string `json:"id"`
	Kind TransactionKind `json:"kind"`
	AccountID string `json:"account_id"`
	MerchantID string `json:"merchant_id,omitempty"`
	FundingSource string `json:"funding_source,omitempty"`
	ExternalRef string `json:"external_reference,omitempty"`
	AmountCents int64 `json:"amount_cents"`
	Currency string `json:"currency"`
	Status TransactionStatus `json:"status"`
	RiskLevel RiskLevel `json:"risk_level,omitempty"`
	RiskReason string `json:"risk_reason,omitempty"`
	CorrelationID string `json:"correlation_id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type OutboxEvent struct {
	ID string `json:"id"`
	Type EventType `json:"type"`
	AggregateID string `json:"aggregate_id"`
	PartitionKey string `json:"partition_key"`
	CorrelationID string `json:"correlation_id"`
	Payload []byte `json:"payload"`
	Status OutboxStatus `json:"status"`
	Attempts int `json:"attempts"`
	LastError string `json:"last_error,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

func NewOutboxEvent(
	eventType EventType,
	aggregateID string,
	partitionKey string,
	correlationID string,
	payload []byte,
) OutboxEvent {
	return OutboxEvent{
		ID: NewID("evt"),
		Type: eventType,
		AggregateID: aggregateID,
		PartitionKey: partitionKey,
		CorrelationID: correlationID,
		Payload: append([]byte(nil), payload...),
		Status: OutboxPending,
		CreatedAt: time.Now().UTC(),
	}
}

type RiskEvaluated struct {
	TransactionID string `json:"transaction_id"`
	AccountID string `json:"account_id"`
	AmountCents int64 `json:"amount_cents"`
	Currency string `json:"currency"`
	RiskLevel RiskLevel `json:"risk_level"`
	Approved bool `json:"approved"`
	Reason string `json:"reason"`
	CorrelationID string `json:"correlation_id"`
}

type FundsDeposited struct {
	DepositID string `json:"deposit_id"`
	AccountID string `json:"account_id"`
	AmountCents int64 `json:"amount_cents"`
	Currency string `json:"currency"`
	FundingSource string `json:"funding_source"`
	ExternalReference string `json:"external_reference"`
	CorrelationID string `json:"correlation_id"`
}

type LedgerEntry struct {
	ID string `json:"id"`
	TransactionID string `json:"transaction_id"`
	AccountID string `json:"account_id"`
	AmountCents int64 `json:"amount_cents"`
	Currency string `json:"currency"`
	CreatedAt time.Time `json:"created_at"`
}

func NewID(prefix string) string {
	var bytes [16]byte
	rand.Read(bytes[:])
	return prefix + "_" + hex.EncodeToString(bytes[:])
}
