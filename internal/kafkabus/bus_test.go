package kafkabus

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/segmentio/kafka-go"
)

func TestMoveToDeadLetterReturnsPublishError(t *testing.T) {
	t.Parallel()

	want := errors.New("broker unavailable")
	message := kafka.Message{
		Topic:     TopicRiskEvaluated,
		Partition: 3,
		Offset:    42,
		Key:       []byte("txn_123"),
		Value:     []byte(`{"id":"txn_123"}`),
		Headers: []kafka.Header{
			{Key: "event_id", Value: []byte("evt_123")},
			{Key: "correlation_id", Value: []byte("trace_123")},
		},
	}

	var moved kafka.Message
	err := moveToDeadLetter(context.Background(), message, func(
		context.Context,
		string,
		kafka.Message,
	) error {
		moved = message
		return want
	})

	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}
	if !reflect.DeepEqual(moved.Key, message.Key) || !reflect.DeepEqual(moved.Value, message.Value) {
		t.Fatal("dead-letter message should preserve the source key and value")
	}
	if got, err := EventID(moved); err != nil || got != "evt_123" {
		t.Fatalf("dead-letter event id = %q, %v; want evt_123", got, err)
	}
}

func TestMessageSeparatesPartitionKeyFromStableEventIdentity(t *testing.T) {
	t.Parallel()

	message := newMessage("acct_123", "evt_123", "trace_123", []byte(`{"id":"txn_123"}`))

	if got := string(message.Key); got != "acct_123" {
		t.Fatalf("message key = %q, want account id", got)
	}
	if got, err := EventID(message); err != nil || got != "evt_123" {
		t.Fatalf("event id = %q, %v; want evt_123", got, err)
	}
	if got := correlationID(message.Headers); got != "trace_123" {
		t.Fatalf("correlation id = %q, want trace_123", got)
	}
}

func TestEventIDSupportsOnlyExplicitHeaderOrLegacyEventKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		message kafka.Message
		want    string
		wantErr bool
	}{
		{
			name: "header",
			message: kafka.Message{Key: []byte("acct_123"), Headers: []kafka.Header{
				{Key: "event_id", Value: []byte("evt_header")},
			}},
			want: "evt_header",
		},
		{name: "legacy outbox key", message: kafka.Message{Key: []byte("evt_legacy")}, want: "evt_legacy"},
		{name: "legacy direct key", message: kafka.Message{Key: []byte("msg_legacy")}, want: "msg_legacy"},
		{name: "new account key without header", message: kafka.Message{Key: []byte("acct_123")}, wantErr: true},
		{name: "empty header", message: kafka.Message{Key: []byte("acct_123"), Headers: []kafka.Header{{Key: "event_id"}}}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := EventID(tt.message)
			if tt.wantErr && err == nil {
				t.Fatalf("EventID() = %q, nil; want error", got)
			}
			if !tt.wantErr && (err != nil || got != tt.want) {
				t.Fatalf("EventID() = %q, %v; want %q", got, err, tt.want)
			}
		})
	}
}
