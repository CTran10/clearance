package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/CTran10/clearance/internal/appenv"
	"github.com/CTran10/clearance/internal/domain"
	"github.com/CTran10/clearance/internal/health"
	"github.com/CTran10/clearance/internal/kafkabus"
	"github.com/CTran10/clearance/internal/ledger"
	"github.com/CTran10/clearance/internal/postgres"
	"github.com/segmentio/kafka-go"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	store, err := postgres.Open(ctx, appenv.Must("DATABASE_URL"))
	if err != nil {
		slog.Error("postgres startup failed")
		os.Exit(1)
	}
	defer store.Close()

	brokers := appenv.CSV("KAFKA_BROKERS", []string{"redpanda:9092"})
	reader := kafkabus.NewReader(brokers, kafkabus.TopicRiskEvaluated, "ledger-service")
	defer func() {
		_ = reader.Close()
	}()
	service := ledger.NewService(store, kafkabus.NewEventPublisher(brokers))
	maxAttempts := appenv.Int("CONSUMER_MAX_ATTEMPTS", 3)
	health.Start(ctx, ":"+appenv.String("HEALTH_PORT", "8083"))

	slog.Info("ledger service started")
	for {
		message, err := reader.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			slog.Warn("ledger service fetch failed")
			continue
		}
		err = retry(ctx, maxAttempts, func() error {
			var event domain.RiskEvaluated
			if err := json.Unmarshal(message.Value, &event); err != nil {
				return err
			}
			return service.HandleRiskEvaluated(ctx, event)
		})
		if err != nil {
			_ = kafkabus.WriteDeadLetter(ctx, brokers, string(message.Key), correlationID(message.Headers), message.Value)
			slog.Warn("ledger service moved message to dead letter")
		}
		if err := reader.CommitMessages(ctx, message); err != nil {
			slog.Warn("ledger service commit failed")
		}
	}
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
