package deadletter

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/segmentio/kafka-go"
)

type State string

const (
	StateOpen        State = "OPEN"
	StateRepublished State = "REPUBLISHED"
	StateDiscarded   State = "DISCARDED"
)

type Record struct {
	ID               string         `json:"id"`
	ConsumerName     string         `json:"consumer_name"`
	EventID          string         `json:"event_id,omitempty"`
	SourceTopic      string         `json:"source_topic"`
	SourcePartition  int            `json:"source_partition"`
	SourceOffset     int64          `json:"source_offset"`
	Key              []byte         `json:"message_key"`
	Headers          []kafka.Header `json:"headers"`
	Payload          []byte         `json:"payload"`
	PayloadSHA256    string         `json:"payload_sha256"`
	ErrorClass       string         `json:"error_class"`
	ErrorMessage     string         `json:"error_message"`
	State            State          `json:"state"`
	FirstFailedAt    time.Time      `json:"first_failed_at"`
	LastFailedAt     time.Time      `json:"last_failed_at"`
	KafkaPublishedAt time.Time      `json:"kafka_published_at,omitempty"`
	ReplayCount      int            `json:"replay_count"`
}

type Store interface {
	UpsertDeadLetter(ctx context.Context, record Record) (Record, error)
	MarkDeadLetterPublished(ctx context.Context, id string, publishedAt time.Time) error
}

type Publisher interface {
	Move(ctx context.Context, message kafka.Message) error
}

type Recorder struct {
	consumerName string
	store        Store
	publisher    Publisher
	now          func() time.Time
}

func NewRecorder(consumerName string, store Store, publisher Publisher) *Recorder {
	return &Recorder{consumerName: consumerName, store: store, publisher: publisher, now: time.Now}
}

func (r *Recorder) Move(ctx context.Context, message kafka.Message, cause error) error {
	record := recordFromMessage(r.consumerName, message, cause, r.now().UTC())
	stored, err := r.store.UpsertDeadLetter(ctx, record)
	if err != nil {
		return fmt.Errorf("persist dead letter: %w", err)
	}
	if !stored.KafkaPublishedAt.IsZero() {
		return nil
	}

	deadLetter := cloneMessage(message)
	deadLetter.Headers = append(deadLetter.Headers,
		kafka.Header{Key: "dead_letter_id", Value: []byte(record.ID)},
		kafka.Header{Key: "failed_consumer", Value: []byte(record.ConsumerName)},
		kafka.Header{Key: "failure_reason", Value: []byte(record.ErrorMessage)},
	)
	if err := r.publisher.Move(ctx, deadLetter); err != nil {
		return fmt.Errorf("publish dead letter: %w", err)
	}
	publishedAt := r.now().UTC()
	if err := r.store.MarkDeadLetterPublished(ctx, record.ID, publishedAt); err != nil {
		return fmt.Errorf("mark dead letter published: %w", err)
	}
	return nil
}

func recordFromMessage(consumerName string, message kafka.Message, cause error, now time.Time) Record {
	payloadHash := sha256.Sum256(message.Value)
	return Record{
		ID:              deterministicID(consumerName, message.Topic, message.Partition, message.Offset),
		ConsumerName:    consumerName,
		EventID:         headerValue(message.Headers, "event_id"),
		SourceTopic:     message.Topic,
		SourcePartition: message.Partition,
		SourceOffset:    message.Offset,
		Key:             append([]byte(nil), message.Key...),
		Headers:         cloneHeaders(message.Headers),
		Payload:         append([]byte(nil), message.Value...),
		PayloadSHA256:   hex.EncodeToString(payloadHash[:]),
		ErrorClass:      classify(cause),
		ErrorMessage:    sanitize(cause),
		State:           StateOpen,
		FirstFailedAt:   now,
		LastFailedAt:    now,
	}
}

func deterministicID(consumerName, topic string, partition int, offset int64) string {
	sum := sha256.Sum256([]byte(consumerName + "\x00" + topic + "\x00" + strconv.Itoa(partition) + "\x00" + strconv.FormatInt(offset, 10)))
	return "dlq_" + hex.EncodeToString(sum[:16])
}

func classify(err error) string {
	if err == nil {
		return "handler_error"
	}
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "decode") || strings.Contains(message, "unmarshal") || strings.Contains(message, "invalid byte"):
		return "decode_error"
	case strings.Contains(message, "incomplete") || strings.Contains(message, "must be") || strings.Contains(message, "does not match"):
		return "validation_error"
	default:
		return "handler_error"
	}
}

func sanitize(err error) string {
	if err == nil {
		return "handler failed"
	}
	values := []rune(strings.TrimSpace(err.Error()))
	if len(values) > 512 {
		values = values[:512]
	}
	for index, value := range values {
		if unicode.IsControl(value) {
			values[index] = ' '
		}
	}
	return string(values)
}

func headerValue(headers []kafka.Header, key string) string {
	for _, header := range headers {
		if header.Key == key {
			return string(header.Value)
		}
	}
	return ""
}

func cloneHeaders(headers []kafka.Header) []kafka.Header {
	cloned := make([]kafka.Header, len(headers))
	for index, header := range headers {
		cloned[index] = kafka.Header{Key: header.Key, Value: append([]byte(nil), header.Value...)}
	}
	return cloned
}

func cloneMessage(message kafka.Message) kafka.Message {
	return kafka.Message{
		Topic: message.Topic, Partition: message.Partition, Offset: message.Offset,
		Key: append([]byte(nil), message.Key...), Value: append([]byte(nil), message.Value...),
		Headers: cloneHeaders(message.Headers),
	}
}
