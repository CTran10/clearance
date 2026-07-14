package ledger

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/CTran10/clearance/internal/consumer"
	"github.com/CTran10/clearance/internal/domain"
)

func TestServicePersistsRiskDeliverySourceMetadata(t *testing.T) {
	t.Parallel()

	store := &ledgerDeliveryStore{}
	service := NewService(store)
	delivery := consumer.Delivery{
		ConsumerName: "ledger-service", EventID: "evt_risk", SourceTopic: "risk.evaluated",
		SourcePartition: 1, SourceOffset: 9,
	}
	payload, err := json.Marshal(domain.RiskEvaluated{
		TransactionID: "txn_123", AccountID: "acct_123", AmountCents: 100, Currency: "USD",
		RiskLevel: domain.RiskLow, Approved: true, Reason: "amount is at or below 500.00", CorrelationID: "trace_123",
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	if err := service.HandleRiskEvaluated(context.Background(), delivery, payload); err != nil {
		t.Fatalf("HandleRiskEvaluated returned error: %v", err)
	}
	if store.delivery != delivery {
		t.Fatalf("delivery = %#v, want %#v", store.delivery, delivery)
	}
}

type ledgerDeliveryStore struct {
	delivery consumer.Delivery
}

func (s *ledgerDeliveryStore) ProcessRiskEvaluated(
	_ context.Context,
	delivery consumer.Delivery,
	_ string,
	_ domain.RiskEvaluated,
) (bool, error) {
	s.delivery = delivery
	return true, nil
}
