package deadletter

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/segmentio/kafka-go"
)

func TestRecorderPersistsExactMessageAndPublishesOnce(t *testing.T) {
	t.Parallel()

	store := &memoryStore{}
	publisher := &memoryPublisher{}
	recorder := NewRecorder("risk-service", store, publisher)
	message := kafka.Message{
		Topic: "transactions.created", Partition: 2, Offset: 42,
		Key: []byte("acct_123"), Value: []byte{0xff, 0x00, 0x7b},
		Headers: []kafka.Header{{Key: "event_id", Value: []byte("evt_123")}, {Key: "x", Value: []byte("one")}, {Key: "x", Value: []byte("two")}},
	}
	cause := errors.New("decode transaction: invalid byte 0xff")

	if err := recorder.Move(context.Background(), message, cause); err != nil {
		t.Fatalf("Move returned error: %v", err)
	}
	if err := recorder.Move(context.Background(), message, cause); err != nil {
		t.Fatalf("duplicate Move returned error: %v", err)
	}
	if store.record.ID == "" || store.record.EventID != "evt_123" || store.record.ErrorClass != "decode_error" {
		t.Fatalf("stored record = %#v", store.record)
	}
	if string(store.record.Key) != "acct_123" || string(store.record.Payload) != string(message.Value) || len(store.record.Headers) != 3 {
		t.Fatalf("stored message was not exact: %#v", store.record)
	}
	if publisher.calls != 1 {
		t.Fatalf("publish calls = %d, want 1", publisher.calls)
	}
	if store.marked != store.record.ID {
		t.Fatalf("marked id = %q, want %q", store.marked, store.record.ID)
	}
}

func TestRecorderReturnsStorePublisherAndMarkFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		store     *memoryStore
		publisher *memoryPublisher
		want      string
	}{
		{name: "persist", store: &memoryStore{upsertErr: errors.New("database unavailable")}, publisher: &memoryPublisher{}, want: "persist dead letter"},
		{name: "publish", store: &memoryStore{}, publisher: &memoryPublisher{err: errors.New("broker unavailable")}, want: "publish dead letter"},
		{name: "mark", store: &memoryStore{markErr: errors.New("database unavailable")}, publisher: &memoryPublisher{}, want: "mark dead letter published"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := NewRecorder("risk-service", test.store, test.publisher).Move(
				context.Background(),
				kafka.Message{Topic: "transactions.created", Partition: 0, Offset: 1, Value: []byte("bad")},
				errors.New("handler failed"),
			)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Move error = %v, want %q", err, test.want)
			}
		})
	}
}

type memoryStore struct {
	record    Record
	marked    string
	upsertErr error
	markErr   error
}

func (s *memoryStore) UpsertDeadLetter(_ context.Context, record Record) (Record, error) {
	if s.upsertErr != nil {
		return Record{}, s.upsertErr
	}
	if s.record.ID != "" {
		return s.record, nil
	}
	s.record = record
	return record, nil
}

func (s *memoryStore) MarkDeadLetterPublished(_ context.Context, id string, publishedAt time.Time) error {
	if s.markErr != nil {
		return s.markErr
	}
	s.marked = id
	s.record.KafkaPublishedAt = publishedAt
	return nil
}

type memoryPublisher struct {
	calls int
	err   error
}

func (p *memoryPublisher) Move(_ context.Context, _ kafka.Message) error {
	p.calls++
	return p.err
}
