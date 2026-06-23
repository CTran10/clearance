package ledger

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/CTran10/clearance/internal/domain"
)

type Store interface {
	Authorize(ctx context.Context, event domain.RiskEvaluated) ([]domain.LedgerEntry, error)
	Fail(ctx context.Context, event domain.RiskEvaluated) error
}

type PublishFunc func(ctx context.Context, event domain.Event) error

type Service struct {
	store     Store
	publisher PublishFunc
}

func NewService(store Store, publisher PublishFunc) *Service {
	return &Service{store: store, publisher: publisher}
}

func (s *Service) HandleRiskEvaluated(ctx context.Context, event domain.RiskEvaluated) error {
	if event.Approved && event.RiskLevel != domain.RiskLow {
		return fmt.Errorf("approved risk evaluation must be low risk")
	}
	if !event.Approved && event.RiskLevel != domain.RiskHigh {
		return fmt.Errorf("failed risk evaluation must be high risk")
	}
	if event.Approved {
		if _, err := s.store.Authorize(ctx, event); err != nil {
			if errors.Is(err, domain.ErrInsufficientFunds) {
				failed := event
				failed.Approved = false
				failed.Reason = "insufficient funds"
				if err := s.store.Fail(ctx, failed); err != nil {
					return fmt.Errorf("fail transaction: %w", err)
				}
				return s.publish(ctx, domain.EventTransactionFailed, failed)
			}
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
	if err := s.publisher(ctx, domain.NewEvent(eventType, evaluated.CorrelationID, payload)); err != nil {
		return fmt.Errorf("publish ledger event: %w", err)
	}
	return nil
}
