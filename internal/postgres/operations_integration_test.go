//go:build integration

package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/CTran10/clearance/internal/deadletter"
	"github.com/CTran10/clearance/internal/maintenance"
	"github.com/CTran10/clearance/internal/operations"
	"github.com/segmentio/kafka-go"
)

func TestDeadLetterReplayAndProcessedRetentionAreDurable(t *testing.T) {
	store := openIntegrationStore(t)
	publisher := &integrationDLQPublisher{}
	recorder := deadletter.NewRecorder("risk-service", store, publisher)
	message := kafka.Message{
		Topic: "transactions.created", Partition: 1, Offset: 42,
		Key: []byte("acct_123"), Value: []byte{0xff, 0x00},
		Headers: []kafka.Header{{Key: "event_id", Value: []byte("evt_guarded")}},
	}
	if err := recorder.Move(context.Background(), message, errors.New("decode transaction: malformed")); err != nil {
		t.Fatalf("record dead letter: %v", err)
	}
	record, ok, err := store.GetDeadLetter(context.Background(), publisher.deadLetterID)
	if err != nil || !ok || string(record.Payload) != string(message.Value) || record.KafkaPublishedAt.IsZero() {
		t.Fatalf("GetDeadLetter = %#v, %v, %v", record, ok, err)
	}

	replayBroker := &integrationReplayBroker{}
	operationsService := operations.NewService(store, replayBroker, operations.Config{
		ReplayWindow: 14 * 24 * time.Hour,
	})
	if _, err := operationsService.ReplayDeadLetter(context.Background(), record.ID, "decoder fixed"); err != nil {
		t.Fatalf("ReplayDeadLetter: %v", err)
	}
	if replayBroker.topic != message.Topic || string(replayBroker.message.Value) != string(message.Value) {
		t.Fatalf("replayed message = %q %#v", replayBroker.topic, replayBroker.message)
	}

	old := time.Now().UTC().Add(-31 * 24 * time.Hour)
	if _, err := store.pool.Exec(context.Background(), `
		insert into processed_events
			(consumer_name, event_id, payload_sha256, processed_at, last_seen_at, source_topic, source_partition, source_offset)
		values
			('risk-service', 'evt_old', $1, $2, $2, 'transactions.created', 0, 1),
			('risk-service', 'evt_guarded', $1, $2, $2, 'transactions.created', 1, 42)
	`, testPayloadHash, old); err != nil {
		t.Fatalf("seed processed events: %v", err)
	}
	maintenanceService, err := maintenance.NewProcessedEventsService(store, maintenance.Config{
		Retention: 30 * 24 * time.Hour, ReplayWindow: 14 * 24 * time.Hour, BatchSize: 100,
	})
	if err != nil {
		t.Fatalf("create maintenance service: %v", err)
	}
	preview, err := maintenanceService.Preview(context.Background())
	if err != nil || preview.Eligible != 1 {
		t.Fatalf("Preview = %#v, %v; want one unguarded row", preview, err)
	}
	result, err := maintenanceService.Prune(context.Background(), "retention boundary acknowledged")
	if err != nil || result.Deleted != 1 {
		t.Fatalf("Prune = %#v, %v", result, err)
	}
	var guarded int
	if err := store.pool.QueryRow(context.Background(), `select count(*) from processed_events where event_id = 'evt_guarded'`).Scan(&guarded); err != nil {
		t.Fatalf("count guarded event: %v", err)
	}
	if guarded != 1 {
		t.Fatalf("guarded processed event count = %d, want 1", guarded)
	}
}

type integrationDLQPublisher struct {
	deadLetterID string
}

func (p *integrationDLQPublisher) Move(_ context.Context, message kafka.Message) error {
	for _, header := range message.Headers {
		if header.Key == "dead_letter_id" {
			p.deadLetterID = string(header.Value)
		}
	}
	return nil
}

type integrationReplayBroker struct {
	topic   string
	message kafka.Message
}

func (b *integrationReplayBroker) PublishMessage(_ context.Context, topic string, message kafka.Message) error {
	b.topic = topic
	b.message = message
	return nil
}
