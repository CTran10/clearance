package kafkabus

import (
	"context"
	"errors"
	"testing"

	"github.com/segmentio/kafka-go"
)

func TestMoveToDeadLetterReturnsPublishError(t *testing.T) {
	t.Parallel()

	want := errors.New("broker unavailable")
	message := kafka.Message{
		Key:   []byte("txn_123"),
		Value: []byte(`{"id":"txn_123"}`),
		Headers: []kafka.Header{
			{Key: "correlation_id", Value: []byte("trace_123")},
		},
	}

	err := moveToDeadLetter(context.Background(), message, func(
		context.Context,
		string,
		string,
		[]byte,
	) error {
		return want
	})

	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}
}
