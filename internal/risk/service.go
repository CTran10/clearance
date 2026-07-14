package risk

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/CTran10/clearance/internal/consumer"
	"github.com/CTran10/clearance/internal/domain"
)

const ConsumerName = "risk-service"

type Store interface {
	SaveConsumerOutbox(
		ctx context.Context,
		delivery consumer.Delivery,
		payloadHash string,
		event domain.OutboxEvent,
	) (bool, error)
}

type Service struct {
	store Store
}

func NewService(store Store) *Service {
	return &Service{store: store}
}

func (s *Service) HandleTransactionCreated(ctx context.Context, delivery consumer.Delivery, payload []byte) error {
	var transaction domain.Transaction
	if err := json.Unmarshal(payload, &transaction); err != nil {
		return fmt.Errorf("decode transaction created event: %w", err)
	}
	if transaction.ID == "" || transaction.AccountID == "" || transaction.AmountCents <= 0 ||
		transaction.Currency == "" || transaction.CorrelationID == "" {
		return fmt.Errorf("transaction created event is incomplete")
	}

	evaluation := domain.EvaluateRisk(transaction.AmountCents)
	eventPayload, err := json.Marshal(domain.RiskEvaluated{
		TransactionID: transaction.ID,
		AccountID:     transaction.AccountID,
		AmountCents:   transaction.AmountCents,
		Currency:      transaction.Currency,
		RiskLevel:     evaluation.Level,
		Approved:      evaluation.Approved,
		Reason:        evaluation.Reason,
		CorrelationID: transaction.CorrelationID,
	})
	if err != nil {
		return fmt.Errorf("encode risk evaluated event: %w", err)
	}
	outboxEvent := domain.NewOutboxEvent(
		domain.EventRiskEvaluated,
		transaction.ID,
		transaction.AccountID,
		transaction.CorrelationID,
		eventPayload,
	)
	if _, err := s.store.SaveConsumerOutbox(
		ctx,
		delivery,
		hashPayload(payload),
		outboxEvent,
	); err != nil {
		return fmt.Errorf("persist risk evaluation: %w", err)
	}
	return nil
}

func hashPayload(payload []byte) string {
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}
