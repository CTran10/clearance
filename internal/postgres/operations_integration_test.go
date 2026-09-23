//go:build integration

package postgres

import (
	"context"
	"errors"
	"reflect"
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

func TestDeadLetterListPreservesRecordsOrderingAndStateFilter(t *testing.T) {
	store := openIntegrationStore(t)
	ctx := context.Background()
	empty, err := store.ListDeadLetters(ctx, "", 0)
	if err != nil || empty == nil || len(empty) != 0 {
		t.Fatalf("empty list = %#v, %v", empty, err)
	}
	record := deadletter.Record{
		ConsumerName: "risk-service", SourceTopic: "transactions.created",
		Key: []byte{0x00, 0xff}, Payload: []byte{0xff, 0x00},
		Headers: []kafka.Header{{Key: "test", Value: []byte{0x00, 0xff}}},
		PayloadSHA256: testPayloadHash, ErrorClass: "handler_error", ErrorMessage: "invalid payload",
		State: deadletter.StateOpen,
	}
	var records []deadletter.Record
	now := time.Now().UTC()
	for index, id := range []string{"dlq_a", "dlq_b", "dlq_c"} {
		record.ID = id
		record.SourceOffset = int64(index)
		record.FirstFailedAt = now.Add(time.Duration(index) * time.Second)
		if index == 2 {
			record.State = deadletter.StateRepublished
			record.EventID = "evt_c"
		}
		stored, err := store.UpsertDeadLetter(ctx, record)
		if err != nil {
			t.Fatalf("seed dead letter: %v", err)
		}
		records = append(records, stored)
	}
	if err := store.MarkDeadLetterPublished(ctx, records[1].ID, now); err != nil {
		t.Fatal(err)
	}
	records[1], _, err = store.GetDeadLetter(ctx, records[1].ID)
	if err != nil {
		t.Fatal(err)
	}
	items, err := store.ListDeadLetters(ctx, "", 2)
	if err != nil || !reflect.DeepEqual(items, []deadletter.Record{records[2], records[1]}) {
		t.Fatalf("limited list = %#v, %v", items, err)
	}
	items, err = store.ListDeadLetters(ctx, deadletter.StateOpen, 100)
	if err != nil || !reflect.DeepEqual(items, []deadletter.Record{records[1], records[0]}) {
		t.Fatalf("open list = %#v, %v", items, err)
	}
	if items[1].EventID != "" || !items[1].KafkaPublishedAt.IsZero() || items[0].KafkaPublishedAt.IsZero() {
		t.Fatalf("nullable fields changed: %#v", items)
	}
}

func TestProcessedPruneCountsBatchAndEmptyDeletes(t *testing.T) {
	store := openIntegrationStore(t)
	ctx := context.Background()
	cutoff := time.Now().UTC().Add(-30 * 24 * time.Hour)
	if _, err := store.pool.Exec(ctx, `
		insert into processed_events (consumer_name, event_id, payload_sha256, processed_at, last_seen_at)
		select 'risk-service', 'evt_' || value, $1, $2::timestamptz, $2::timestamptz
		from generate_series(1, 3) value`, testPayloadHash, cutoff.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	for _, want := range []int64{2, 1, 0} {
		deleted, err := store.PruneProcessedEvents(ctx, cutoff, 2, "expired records")
		if err != nil || deleted != want {
			t.Fatalf("prune = %d, %v; want %d", deleted, err, want)
		}
	}
	assertCount(t, store, "processed_events", 0)
	assertCount(t, store, "operator_actions", 3)
}

func TestDeadLetterAllowsOnlyOnePendingReplayAttempt(t *testing.T) {
	store := openIntegrationStore(t)
	now := time.Now().UTC()
	record, err := store.UpsertDeadLetter(context.Background(), deadletter.Record{
		ID: "dlq_pending", ConsumerName: "risk-service", EventID: "evt_pending",
		SourceTopic: "transactions.created", SourcePartition: 0, SourceOffset: 7,
		Key: []byte("acct_123"), Headers: []kafka.Header{}, Payload: []byte(`{"bad":true}`),
		PayloadSHA256: testPayloadHash, ErrorClass: "handler_error", ErrorMessage: "dependency unavailable",
		State: deadletter.StateOpen, FirstFailedAt: now, LastFailedAt: now,
	})
	if err != nil {
		t.Fatalf("seed dead letter: %v", err)
	}
	firstAttempt, err := store.StartDeadLetterReplay(context.Background(), record.ID, "first operator")
	if err != nil {
		t.Fatalf("start first replay: %v", err)
	}
	if _, err := store.StartDeadLetterReplay(context.Background(), record.ID, "second operator"); !errors.Is(err, operations.ErrInvalidState) {
		t.Fatalf("second replay error = %v, want ErrInvalidState", err)
	}
	if err := store.FinishDeadLetterReplay(
		context.Background(), firstAttempt, record.ID, operations.ReplayFailed, "broker unavailable",
	); err != nil {
		t.Fatalf("finish failed replay: %v", err)
	}
	if _, err := store.StartDeadLetterReplay(context.Background(), record.ID, "retry after failure"); err != nil {
		t.Fatalf("start replay after failed attempt: %v", err)
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
	topic string
	message kafka.Message
}

func (b *integrationReplayBroker) PublishMessage(_ context.Context, topic string, message kafka.Message) error {
	b.topic = topic
	b.message = message
	return nil
}
