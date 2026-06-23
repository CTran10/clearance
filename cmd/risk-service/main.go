package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"os/signal"
	"syscall"
	"time"

	"github.com/CTran10/clearance/internal/appenv"
	"github.com/CTran10/clearance/internal/consumer"
	"github.com/CTran10/clearance/internal/domain"
	"github.com/CTran10/clearance/internal/health"
	"github.com/CTran10/clearance/internal/kafkabus"
	"github.com/segmentio/kafka-go"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	brokers := appenv.CSV("KAFKA_BROKERS", []string{"redpanda:9092"})
	reader := kafkabus.NewReader(brokers, kafkabus.TopicTransactionCreated, "risk-service")
	defer func() {
		_ = reader.Close()
	}()
	publisher := kafkabus.NewPublisher(brokers)
	defer func() {
		_ = publisher.Close()
	}()
	maxAttempts := appenv.Int("CONSUMER_MAX_ATTEMPTS", 3)
	health.Start(ctx, ":"+appenv.String("HEALTH_PORT", "8082"), appenv.Bool("METRICS_ENABLED", false))

	slog.Info("risk service started")
	consumer.RunLoop(ctx, reader, publisher, consumer.Config{
		Name:           "risk service",
		MaxAttempts:    maxAttempts,
		RetryBaseDelay: 100 * time.Millisecond,
	}, func(ctx context.Context, message kafka.Message) error {
		return handle(ctx, publisher, message.Value)
	})
}

func handle(ctx context.Context, publisher *kafkabus.Publisher, payload []byte) error {
	var transaction domain.Transaction
	if err := json.Unmarshal(payload, &transaction); err != nil {
		return err
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
		return err
	}
	event := domain.NewEvent(domain.EventRiskEvaluated, transaction.CorrelationID, eventPayload)
	return publisher.Publish(ctx, kafkabus.TopicFor(event.Type), event.ID, event.CorrelationID, event.Payload)
}
