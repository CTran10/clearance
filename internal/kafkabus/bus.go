package kafkabus

import (
	"context"
	"fmt"
	"time"

	"github.com/CTran10/clearance/internal/domain"
	"github.com/segmentio/kafka-go"
)

const (
	TopicTransactionCreated    = "transactions.created"
	TopicRiskEvaluated         = "risk.evaluated"
	TopicTransactionAuthorized = "transactions.authorized"
	TopicTransactionFailed     = "transactions.failed"
	TopicDeadLetter            = "dead-letter"
)

type OutboxBroker struct {
	brokers []string
}

func NewOutboxBroker(brokers []string) *OutboxBroker {
	return &OutboxBroker{brokers: brokers}
}

func (b *OutboxBroker) Publish(ctx context.Context, event domain.OutboxEvent) error {
	return write(ctx, b.brokers, topicFor(event.Type), event.ID, event.CorrelationID, event.Payload)
}

type EventPublisher struct {
	brokers []string
}

func NewEventPublisher(brokers []string) *EventPublisher {
	return &EventPublisher{brokers: brokers}
}

func (p *EventPublisher) Publish(ctx context.Context, event domain.Event) error {
	return write(ctx, p.brokers, topicFor(event.Type), event.ID, event.CorrelationID, event.Payload)
}

func WriteDeadLetter(ctx context.Context, brokers []string, key string, correlationID string, payload []byte) error {
	return write(ctx, brokers, TopicDeadLetter, key, correlationID, payload)
}

func NewReader(brokers []string, topic string, groupID string) *kafka.Reader {
	return kafka.NewReader(kafka.ReaderConfig{
		Brokers:        brokers,
		Topic:          topic,
		GroupID:        groupID,
		CommitInterval: 0,
		MinBytes:       1,
		MaxBytes:       1e6,
	})
}

func write(ctx context.Context, brokers []string, topic string, key string, correlationID string, payload []byte) error {
	writer := &kafka.Writer{
		Addr:         kafka.TCP(brokers...),
		Topic:        topic,
		RequiredAcks: kafka.RequireAll,
		Balancer:     &kafka.Hash{},
		BatchTimeout: 10 * time.Millisecond,
	}
	defer func() {
		_ = writer.Close()
	}()

	if err := writer.WriteMessages(ctx, kafka.Message{
		Key:   []byte(key),
		Value: payload,
		Headers: []kafka.Header{
			{Key: "correlation_id", Value: []byte(correlationID)},
		},
	}); err != nil {
		return fmt.Errorf("write kafka message: %w", err)
	}
	return nil
}

func topicFor(eventType domain.EventType) string {
	switch eventType {
	case domain.EventTransactionCreated:
		return TopicTransactionCreated
	case domain.EventRiskEvaluated:
		return TopicRiskEvaluated
	case domain.EventTransactionAuthorized:
		return TopicTransactionAuthorized
	case domain.EventTransactionFailed:
		return TopicTransactionFailed
	default:
		return TopicDeadLetter
	}
}
