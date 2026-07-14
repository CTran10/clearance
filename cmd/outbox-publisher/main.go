package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/CTran10/clearance/internal/appenv"
	"github.com/CTran10/clearance/internal/domain"
	"github.com/CTran10/clearance/internal/health"
	"github.com/CTran10/clearance/internal/kafkabus"
	"github.com/CTran10/clearance/internal/outbox"
	"github.com/CTran10/clearance/internal/postgres"
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

	broker := kafkabus.NewPublisher(appenv.CSV("KAFKA_BROKERS", []string{"redpanda:9092"}))
	defer func() {
		_ = broker.Close()
	}()
	publisher := outbox.NewPublisher(
		store,
		func(ctx context.Context, event domain.OutboxEvent) error {
			return broker.Publish(
				ctx,
				kafkabus.TopicFor(event.Type),
				event.PartitionKey,
				event.ID,
				event.CorrelationID,
				event.Payload,
			)
		},
		outbox.Config{MaxAttempts: appenv.Int("OUTBOX_MAX_ATTEMPTS", 3)},
	)
	ticker := time.NewTicker(appenv.DurationSeconds("OUTBOX_POLL_SECONDS", time.Second))
	defer ticker.Stop()
	health.Start(ctx, ":"+appenv.String("HEALTH_PORT", "8081"), appenv.Bool("METRICS_ENABLED", false))

	slog.Info("outbox publisher started")
	for {
		if err := publisher.PublishAvailable(ctx); err != nil {
			slog.Warn("outbox publish attempt failed", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
