package kafkabus

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
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
	TopicFundsDeposited        = "funding.deposited"
	TopicDeadLetter            = "dead-letter"
)

type Publisher struct {
	writers *topicWriters
}

func NewPublisher(brokers []string) *Publisher {
	return &Publisher{writers: newTopicWriters(brokers)}
}

func (p *Publisher) Publish(
	ctx context.Context,
	topic string,
	partitionKey string,
	eventID string,
	correlationID string,
	payload []byte,
) error {
	return p.writers.write(ctx, topic, partitionKey, eventID, correlationID, payload)
}

func (p *Publisher) Move(ctx context.Context, message kafka.Message) error {
	return moveToDeadLetter(ctx, message, p.writers.writeMessage)
}

func (p *Publisher) Close() error {
	return p.writers.Close()
}

func moveToDeadLetter(
	ctx context.Context,
	message kafka.Message,
	writeDeadLetter func(context.Context, string, kafka.Message) error,
) error {
	deadLetter := kafka.Message{
		Key:     append([]byte(nil), message.Key...),
		Value:   append([]byte(nil), message.Value...),
		Headers: cloneHeaders(message.Headers),
	}
	deadLetter.Headers = append(deadLetter.Headers,
		kafka.Header{Key: "source_topic", Value: []byte(message.Topic)},
		kafka.Header{Key: "source_partition", Value: []byte(strconv.Itoa(message.Partition))},
		kafka.Header{Key: "source_offset", Value: []byte(strconv.FormatInt(message.Offset, 10))},
	)
	return writeDeadLetter(ctx, TopicDeadLetter, deadLetter)
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

func (w *topicWriters) write(
	ctx context.Context,
	topic string,
	partitionKey string,
	eventID string,
	correlationID string,
	payload []byte,
) error {
	return w.writeMessage(ctx, topic, newMessage(partitionKey, eventID, correlationID, payload))
}

func (w *topicWriters) writeMessage(ctx context.Context, topic string, message kafka.Message) error {
	if err := w.writer(topic).WriteMessages(ctx, message); err != nil {
		metrics.Inc("clearance_kafka_messages_published_total", metrics.Labels{"topic": topic, "result": "error"})
		return fmt.Errorf("write kafka message: %w", err)
	}
	metrics.Inc("clearance_kafka_messages_published_total", metrics.Labels{"topic": topic, "result": "ok"})
	return nil
}

func newMessage(partitionKey string, eventID string, correlationID string, payload []byte) kafka.Message {
	return kafka.Message{
		Key:   []byte(partitionKey),
		Value: append([]byte(nil), payload...),
		Headers: []kafka.Header{
			{Key: "event_id", Value: []byte(eventID)},
			{Key: "correlation_id", Value: []byte(correlationID)},
		},
	}
}

func EventID(message kafka.Message) (string, error) {
	for _, header := range message.Headers {
		if header.Key == "event_id" {
			if len(header.Value) == 0 {
				return "", fmt.Errorf("event_id header is empty")
			}
			return string(header.Value), nil
		}
	}
	legacyKey := string(message.Key)
	if strings.HasPrefix(legacyKey, "evt_") || strings.HasPrefix(legacyKey, "msg_") {
		return legacyKey, nil
	}
	return "", fmt.Errorf("event_id header is required")
}

func cloneHeaders(headers []kafka.Header) []kafka.Header {
	cloned := make([]kafka.Header, len(headers))
	for i, header := range headers {
		cloned[i] = kafka.Header{Key: header.Key, Value: append([]byte(nil), header.Value...)}
	}
	return cloned
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
	case domain.EventFundsDeposited:
		return TopicFundsDeposited
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
