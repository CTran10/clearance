package ledger

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/CTran10/clearance/internal/domain"
)

type Store interface {
	ProcessRiskEvaluated(
		ctx context.Context,
		eventID string,
		payloadHash string,
		event domain.RiskEvaluated,
	) (bool, error)
}

type Service struct {
	store Store
}

func NewService(store Store) *Service {
	return &Service{store: store}
}

func (s *Service) HandleRiskEvaluated(ctx context.Context, eventID string, payload []byte) error {
	var event domain.RiskEvaluated
	if err := json.Unmarshal(payload, &event); err != nil {
		return fmt.Errorf("decode risk evaluated event: %w", err)
	}
	if event.TransactionID == "" || event.AccountID == "" || event.AmountCents <= 0 || event.Currency == "" {
		return fmt.Errorf("risk evaluated event is incomplete")
	}
	if event.Approved && event.RiskLevel != domain.RiskLow {
		return fmt.Errorf("approved risk evaluation must be low risk")
	}
	if !event.Approved && event.RiskLevel != domain.RiskHigh {
		return fmt.Errorf("failed risk evaluation must be high risk")
	}

	sum := sha256.Sum256(payload)
	if _, err := s.store.ProcessRiskEvaluated(ctx, eventID, hex.EncodeToString(sum[:]), event); err != nil {
		return fmt.Errorf("process risk evaluation: %w", err)
	}
	return nil
}
