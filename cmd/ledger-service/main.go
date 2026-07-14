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
	"github.com/CTran10/clearance/internal/ledger"
	"github.com/CTran10/clearance/internal/metrics"
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
	metricsEnabled := appenv.Bool("METRICS_ENABLED", false)
	metrics.Configure(ledger.ConsumerName)
	if metricsEnabled {
		metrics.StartSampler(ctx, appenv.DurationSeconds("METRICS_SAMPLE_SECONDS", 15*time.Second), store)
	}

	brokers := appenv.CSV("KAFKA_BROKERS", []string{"redpanda:9092"})
	reader := kafkabus.NewReader(brokers, kafkabus.TopicRiskEvaluated, "ledger-service")
	defer func() {
		_ = reader.Close()
	}()
	publisher := kafkabus.NewPublisher(brokers)
	defer func() {
		_ = publisher.Close()
	}()
	service := ledger.NewService(store)
	maxAttempts := appenv.Int("CONSUMER_MAX_ATTEMPTS", 3)
	deadLetterer := deadletter.NewRecorder(ledger.ConsumerName, store, publisher)
	health.Start(ctx, ":"+appenv.String("HEALTH_PORT", "8083"), metricsEnabled)

	slog.Info("ledger service started")
	consumer.RunLoop(ctx, reader, deadLetterer, consumer.Config{
		Name:           ledger.ConsumerName,
		MaxAttempts:    maxAttempts,
		RetryBaseDelay: 100 * time.Millisecond,
	}, func(ctx context.Context, message kafka.Message) error {
		eventID, err := kafkabus.EventID(message)
		if err != nil {
			return err
		}
		return service.HandleRiskEvaluated(ctx, consumer.Delivery{
			ConsumerName: ledger.ConsumerName, EventID: eventID, SourceTopic: message.Topic,
			SourcePartition: message.Partition, SourceOffset: message.Offset,
		}, message.Value)
	})
}
