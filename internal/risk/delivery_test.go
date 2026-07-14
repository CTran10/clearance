package risk

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/CTran10/clearance/internal/consumer"
	"github.com/CTran10/clearance/internal/domain"
)

func TestServicePersistsBrokerSourceMetadata(t *testing.T) {
	t.Parallel()

	store := &deliveryStore{}
	service := NewService(store)
	delivery := consumer.Delivery{
		ConsumerName: "risk-service", EventID: "evt_source", SourceTopic: "transactions.created",
		SourcePartition: 2, SourceOffset: 42,
	}
	payload, err := json.Marshal(domain.Transaction{
		ID: "txn_123", Kind: domain.TransactionPayment, AccountID: "acct_123", MerchantID: "merchant_123",
		AmountCents: 100, Currency: "USD", Status: domain.TransactionPending, CorrelationID: "trace_123",
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	if err := service.HandleTransactionCreated(context.Background(), delivery, payload); err != nil {
		t.Fatalf("HandleTransactionCreated returned error: %v", err)
	}
	if store.delivery != delivery {
		t.Fatalf("delivery = %#v, want %#v", store.delivery, delivery)
	}
}

type deliveryStore struct {
	delivery consumer.Delivery
}

func (s *deliveryStore) SaveConsumerOutbox(
	_ context.Context,
	delivery consumer.Delivery,
	_ string,
	_ domain.OutboxEvent,
) (bool, error) {
	s.delivery = delivery
	return true, nil
}
