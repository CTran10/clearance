package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"os/signal"
	"syscall"
	"time"

	"github.com/CTran10/clearance/internal/appenv"
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
	publisher := kafkabus.NewEventPublisher(brokers)
	maxAttempts := appenv.Int("CONSUMER_MAX_ATTEMPTS", 3)
	health.Start(ctx, ":"+appenv.String("HEALTH_PORT", "8082"))

	slog.Info("risk service started")
	for {
		message, err := reader.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			slog.Warn("risk service fetch failed")
			continue
		}
		err = retry(ctx, maxAttempts, func() error {
			return handle(ctx, publisher, message.Value)
		})
		if err != nil {
			_ = kafkabus.WriteDeadLetter(ctx, brokers, string(message.Key), correlationID(message.Headers), message.Value)
			slog.Warn("risk service moved message to dead letter")
		}
		if err := reader.CommitMessages(ctx, message); err != nil {
			slog.Warn("risk service commit failed")
		}
	}
}

func handle(ctx context.Context, publisher *kafkabus.EventPublisher, payload []byte) error {
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
	return publisher.Publish(ctx, domain.NewEvent(domain.EventRiskEvaluated, transaction.CorrelationID, eventPayload))
}

func retry(ctx context.Context, maxAttempts int, fn func() error) error {
	var err error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if err = fn(); err == nil {
			return nil
		}
		timer := time.NewTimer(time.Duration(attempt) * 100 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return err
}

func correlationID(headers []kafka.Header) string {
	for _, header := range headers {
		if header.Key == "correlation_id" {
			return string(header.Value)
		}
	}
	return ""
}
