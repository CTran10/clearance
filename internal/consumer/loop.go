package consumer

import (
	"context"
	"log/slog"
	"time"

	"github.com/CTran10/clearance/internal/metrics"
	"github.com/segmentio/kafka-go"
)

type Reader interface {
	FetchMessage(ctx context.Context) (kafka.Message, error)
	CommitMessages(ctx context.Context, msgs ...kafka.Message) error
}

type DeadLetterer interface {
	Move(ctx context.Context, message kafka.Message, cause error) error
}

type Delivery struct {
	ConsumerName string
	EventID string
	SourceTopic string
	SourcePartition int
	SourceOffset int64
}

type Config struct {
	Name string
	MaxAttempts int
	RetryBaseDelay time.Duration
}

func RunLoop(ctx context.Context, reader Reader, deadLetterer DeadLetterer, config Config, handle func(context.Context, kafka.Message) error) {
	if config.MaxAttempts <= 0 {
		config.MaxAttempts = 3
	}
	for {
		message, err := reader.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			slog.Warn(config.Name+" fetch failed", "err", err)
			continue
		}
		started := time.Now()
		err = retry(ctx, config.MaxAttempts, config.RetryBaseDelay, func() error {
			return handle(ctx, message)
		}, func() {
			metrics.IncConsumerRetry(config.Name, message.Topic)
		})
		result := "processed"
		if err != nil {
			if dlqErr := deadLetterer.Move(ctx, message, err); dlqErr != nil {
				slog.Warn(config.Name+" dead letter publish failed", "err", dlqErr)
				metrics.ObserveConsumerMessage(config.Name, message.Topic, "dlq_error", time.Since(started))

				continue
			}
			result = "dlq"
			slog.Warn(config.Name+" moved message to dead letter", "err", err)
		}
		// Commit only after handling or durable dead-letter delivery succeeds.
		if err := reader.CommitMessages(ctx, message); err != nil {
			metrics.IncOffsetCommitFailure(config.Name, message.Topic)
			slog.Warn(config.Name+" commit failed", "err", err)
		}
		metrics.ObserveConsumerMessage(config.Name, message.Topic, result, time.Since(started))
	}
}

func retry(ctx context.Context, maxAttempts int, baseDelay time.Duration, fn func() error, onRetry func()) error {
	var err error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if err = fn(); err == nil {
			return nil
		}
		if attempt < maxAttempts && onRetry != nil {
			onRetry()
		}
		delay := time.Duration(attempt) * baseDelay // backoff grows each attempt so we don't hammer a struggling downstream
		if delay <= 0 {
			continue
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return err
}
