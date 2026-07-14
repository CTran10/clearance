package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/CTran10/clearance/internal/appenv"
	"github.com/CTran10/clearance/internal/consumer"
	"github.com/CTran10/clearance/internal/deadletter"
	"github.com/CTran10/clearance/internal/health"
	"github.com/CTran10/clearance/internal/kafkabus"
	"github.com/CTran10/clearance/internal/postgres"
	"github.com/CTran10/clearance/internal/risk"
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
	service := risk.NewService(store)

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
	deadLetterer := deadletter.NewRecorder(risk.ConsumerName, store, publisher)
	health.Start(ctx, ":"+appenv.String("HEALTH_PORT", "8082"), appenv.Bool("METRICS_ENABLED", false))

	slog.Info("risk service started")
	consumer.RunLoop(ctx, reader, deadLetterer, consumer.Config{
		Name:           "risk service",
		MaxAttempts:    maxAttempts,
		RetryBaseDelay: 100 * time.Millisecond,
	}, func(ctx context.Context, message kafka.Message) error {
		eventID, err := kafkabus.EventID(message)
		if err != nil {
			return err
		}
		return service.HandleTransactionCreated(ctx, consumer.Delivery{
			ConsumerName: risk.ConsumerName, EventID: eventID, SourceTopic: message.Topic,
			SourcePartition: message.Partition, SourceOffset: message.Offset,
		}, message.Value)
	})
}
