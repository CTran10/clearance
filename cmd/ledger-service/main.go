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
	"github.com/CTran10/clearance/internal/consumer"
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
		slog.Error("postgres startup failed", "err", err)
		os.Exit(1)
	}
	defer store.Close()

	brokers := appenv.CSV("KAFKA_BROKERS", []string{"redpanda:9092"})
	reader := kafkabus.NewReader(brokers, kafkabus.TopicRiskEvaluated, "ledger-service")
	defer func() {
		_ = reader.Close()
	}()
	publisher := kafkabus.NewPublisher(brokers)
	defer func() {
		_ = publisher.Close()
	}()
	service := ledger.NewService(store, func(ctx context.Context, event domain.Event) error {
		return publisher.Publish(ctx, kafkabus.TopicFor(event.Type), event.ID, event.CorrelationID, event.Payload)
	})
	maxAttempts := appenv.Int("CONSUMER_MAX_ATTEMPTS", 3)
	health.Start(ctx, ":"+appenv.String("HEALTH_PORT", "8083"), appenv.Bool("METRICS_ENABLED", false))

	slog.Info("ledger service started")
	consumer.RunLoop(ctx, reader, publisher, consumer.Config{
		Name:           "ledger service",
		MaxAttempts:    maxAttempts,
		RetryBaseDelay: 100 * time.Millisecond,
	}, func(ctx context.Context, message kafka.Message) error {
		var event domain.RiskEvaluated
		if err := json.Unmarshal(message.Value, &event); err != nil {
			return err
		}
		return service.HandleRiskEvaluated(ctx, event)
	})
}
