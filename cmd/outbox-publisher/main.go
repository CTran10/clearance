package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/CTran10/clearance/internal/appenv"
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
		slog.Error("postgres startup failed")
		os.Exit(1)
	}
	defer store.Close()

	publisher := outbox.NewPublisher(
		store,
		kafkabus.NewOutboxBroker(appenv.CSV("KAFKA_BROKERS", []string{"redpanda:9092"})),
		outbox.Config{MaxAttempts: appenv.Int("OUTBOX_MAX_ATTEMPTS", 3)},
	)
	ticker := time.NewTicker(appenv.DurationSeconds("OUTBOX_POLL_SECONDS", time.Second))
	defer ticker.Stop()
	health.Start(ctx, ":"+appenv.String("HEALTH_PORT", "8081"))

	slog.Info("outbox publisher started")
	for {
		if err := publisher.PublishNext(ctx); err != nil {
			slog.Warn("outbox publish attempt failed")
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
