package kafkabus

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/CTran10/clearance/internal/domain"
	"github.com/CTran10/clearance/internal/metrics"
	"github.com/segmentio/kafka-go"
)

const (
	TopicTransactionCreated    = "transactions.created"
	TopicRiskEvaluated         = "risk.evaluated"
	TopicTransactionAuthorized = "transactions.authorized"
	TopicTransactionFailed     = "transactions.failed"
	TopicDeadLetter            = "dead-letter"
)

type Publisher struct {
	writers *topicWriters
}

func NewPublisher(brokers []string) *Publisher {
	return &Publisher{writers: newTopicWriters(brokers)}
}

func (p *Publisher) Publish(ctx context.Context, topic string, key string, correlationID string, payload []byte) error {
	return p.writers.write(ctx, topic, key, correlationID, payload)
}

func (p *Publisher) Move(ctx context.Context, message kafka.Message) error {
	return moveToDeadLetter(ctx, message, p.write)
}

func (p *Publisher) Close() error {
	return p.writers.Close()
}

func (p *Publisher) write(ctx context.Context, key string, correlationID string, payload []byte) error {
	return p.Publish(ctx, TopicDeadLetter, key, correlationID, payload)
}

func moveToDeadLetter(
	ctx context.Context,
	message kafka.Message,
	writeDeadLetter func(context.Context, string, string, []byte) error,
) error {
	return writeDeadLetter(ctx, string(message.Key), correlationID(message.Headers), message.Value)
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

type topicWriters struct {
	brokers []string
	mu      sync.Mutex
	writers map[string]*kafka.Writer
}

func newTopicWriters(brokers []string) *topicWriters {
	return &topicWriters{
		brokers: brokers,
		writers: make(map[string]*kafka.Writer),
	}
}

func (w *topicWriters) write(ctx context.Context, topic string, key string, correlationID string, payload []byte) error {
	writer := w.writer(topic)

	if err := writer.WriteMessages(ctx, kafka.Message{
		Key:   []byte(key),
		Value: payload,
		Headers: []kafka.Header{
			{Key: "correlation_id", Value: []byte(correlationID)},
		},
	}); err != nil {
		metrics.Inc("clearance_kafka_messages_published_total", metrics.Labels{"topic": topic, "result": "error"})
		return fmt.Errorf("write kafka message: %w", err)
	}
	metrics.Inc("clearance_kafka_messages_published_total", metrics.Labels{"topic": topic, "result": "ok"})
	return nil
}

func (w *topicWriters) writer(topic string) *kafka.Writer {
	w.mu.Lock()
	defer w.mu.Unlock()

	writer, ok := w.writers[topic]
	if ok {
		return writer
	}
	writer = &kafka.Writer{
		Addr:         kafka.TCP(w.brokers...),
		Topic:        topic,
		RequiredAcks: kafka.RequireAll,
		Balancer:     &kafka.Hash{},
		BatchTimeout: 10 * time.Millisecond,
	}
	w.writers[topic] = writer
	return writer
}

func (w *topicWriters) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	var err error
	for topic, writer := range w.writers {
		if closeErr := writer.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close kafka writer %s: %w", topic, closeErr))
		}
	}
	return err
}

func TopicFor(eventType domain.EventType) string {
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

func correlationID(headers []kafka.Header) string {
	for _, header := range headers {
		if header.Key == "correlation_id" {
			return string(header.Value)
		}
	}
	return ""
}
