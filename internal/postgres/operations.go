package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/CTran10/clearance/internal/deadletter"
	"github.com/CTran10/clearance/internal/domain"
	"github.com/CTran10/clearance/internal/operations"
	"github.com/jackc/pgx/v5"
	"github.com/segmentio/kafka-go"
)

type storedHeader struct {
	Key   string `json:"key"`
	Value []byte `json:"value"`
}

func (s *Store) UpsertDeadLetter(ctx context.Context, record deadletter.Record) (deadletter.Record, error) {
	headersJSON, err := encodeHeaders(record.Headers)
	if err != nil {
		return deadletter.Record{}, fmt.Errorf("encode dead letter headers: %w", err)
	}
	var stored deadletter.Record
	var storedHeaders []byte
	var eventID *string
	var kafkaPublishedAt *time.Time
	err = s.pool.QueryRow(
		ctx,
		`insert into dead_letter_messages
			(id, consumer_name, event_id, source_topic, source_partition, source_offset,
			 message_key, headers, payload, payload_sha256, error_class, error_message,
			 state, first_failed_at, last_failed_at)
		 values ($1, $2, nullif($3, ''), $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $14)
		 on conflict (consumer_name, source_topic, source_partition, source_offset)
		 do update set last_failed_at = excluded.last_failed_at,
		               error_class = excluded.error_class,
		               error_message = excluded.error_message,
		               version = dead_letter_messages.version + 1
		 returning id, consumer_name, event_id, source_topic, source_partition, source_offset,
		           message_key, headers, payload, payload_sha256, error_class, error_message,
		           state, first_failed_at, last_failed_at, kafka_published_at, replay_count`,
		record.ID,
		record.ConsumerName,
		record.EventID,
		record.SourceTopic,
		record.SourcePartition,
		record.SourceOffset,
		record.Key,
		headersJSON,
		record.Payload,
		record.PayloadSHA256,
		record.ErrorClass,
		record.ErrorMessage,
		record.State,
		record.FirstFailedAt,
	).Scan(
		&stored.ID,
		&stored.ConsumerName,
		&eventID,
		&stored.SourceTopic,
		&stored.SourcePartition,
		&stored.SourceOffset,
		&stored.Key,
		&storedHeaders,
		&stored.Payload,
		&stored.PayloadSHA256,
		&stored.ErrorClass,
		&stored.ErrorMessage,
		&stored.State,
		&stored.FirstFailedAt,
		&stored.LastFailedAt,
		&kafkaPublishedAt,
		&stored.ReplayCount,
	)
	if err != nil {
		return deadletter.Record{}, fmt.Errorf("upsert dead letter: %w", err)
	}
	if eventID != nil {
		stored.EventID = *eventID
	}
	if kafkaPublishedAt != nil {
		stored.KafkaPublishedAt = *kafkaPublishedAt
	}
	stored.Headers, err = decodeHeaders(storedHeaders)
	if err != nil {
		return deadletter.Record{}, fmt.Errorf("decode stored dead letter headers: %w", err)
	}
	return stored, nil
}

func (s *Store) MarkDeadLetterPublished(ctx context.Context, id string, publishedAt time.Time) error {
	tag, err := s.pool.Exec(
		ctx,
		`update dead_letter_messages
		    set kafka_published_at = coalesce(kafka_published_at, $2), version = version + 1
		  where id = $1`,
		id,
		publishedAt,
	)
	if err != nil {
		return fmt.Errorf("mark dead letter published: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("mark dead letter published: not found")
	}
	return nil
}

func (s *Store) GetDeadLetter(ctx context.Context, id string) (deadletter.Record, bool, error) {
	var record deadletter.Record
	var headersJSON []byte
	var eventID *string
	var kafkaPublishedAt *time.Time
	err := s.pool.QueryRow(
		ctx,
		`select id, consumer_name, event_id, source_topic, source_partition, source_offset,
		        message_key, headers, payload, payload_sha256, error_class, error_message,
		        state, first_failed_at, last_failed_at, kafka_published_at, replay_count
		   from dead_letter_messages
		  where id = $1`,
		id,
	).Scan(
		&record.ID,
		&record.ConsumerName,
		&eventID,
		&record.SourceTopic,
		&record.SourcePartition,
		&record.SourceOffset,
		&record.Key,
		&headersJSON,
		&record.Payload,
		&record.PayloadSHA256,
		&record.ErrorClass,
		&record.ErrorMessage,
		&record.State,
		&record.FirstFailedAt,
		&record.LastFailedAt,
		&kafkaPublishedAt,
		&record.ReplayCount,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return deadletter.Record{}, false, nil
		}
		return deadletter.Record{}, false, fmt.Errorf("get dead letter: %w", err)
	}
	if eventID != nil {
		record.EventID = *eventID
	}
	if kafkaPublishedAt != nil {
		record.KafkaPublishedAt = *kafkaPublishedAt
	}
	record.Headers, err = decodeHeaders(headersJSON)
	if err != nil {
		return deadletter.Record{}, false, fmt.Errorf("decode dead letter headers: %w", err)
	}
	return record, true, nil
}

func (s *Store) ListDeadLetters(ctx context.Context, state deadletter.State, limit int) ([]deadletter.Record, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := s.pool.Query(
		ctx,
		`select id from dead_letter_messages
		  where ($1 = '' or state = $1)
		  order by first_failed_at desc, id desc
		  limit $2`,
		state,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list dead letters: %w", err)
	}
	defer rows.Close()
	ids := make([]string, 0, limit)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan dead letter id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate dead letter ids: %w", err)
	}
	items := make([]deadletter.Record, 0, len(ids))
	for _, id := range ids {
		item, ok, err := s.GetDeadLetter(ctx, id)
		if err != nil {
			return nil, err
		}
		if ok {
			items = append(items, item)
		}
	}
	return items, nil
}

func (s *Store) ListDeadOutbox(ctx context.Context, limit int) ([]domain.OutboxEvent, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := s.pool.Query(
		ctx,
		`select id, event_type, aggregate_id, partition_key, correlation_id, payload,
		        status, attempts, coalesce(last_error, ''), created_at
		   from outbox_events
		  where status = $1
		  order by created_at desc, id desc
		  limit $2`,
		domain.OutboxDeadLettered,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list dead-lettered outbox events: %w", err)
	}
	defer rows.Close()
	items := make([]domain.OutboxEvent, 0, limit)
	for rows.Next() {
		var event domain.OutboxEvent
		if err := rows.Scan(
			&event.ID,
			&event.Type,
			&event.AggregateID,
			&event.PartitionKey,
			&event.CorrelationID,
			&event.Payload,
			&event.Status,
			&event.Attempts,
			&event.LastError,
			&event.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan dead-lettered outbox event: %w", err)
		}
		items = append(items, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate dead-lettered outbox events: %w", err)
	}
	return items, nil
}

func (s *Store) GetOutboxEvent(ctx context.Context, id string) (domain.OutboxEvent, bool, error) {
	var event domain.OutboxEvent
	err := s.pool.QueryRow(
		ctx,
		`select id, event_type, aggregate_id, partition_key, correlation_id, payload,
		        status, attempts, coalesce(last_error, ''), created_at
		   from outbox_events
		  where id = $1`,
		id,
	).Scan(
		&event.ID,
		&event.Type,
		&event.AggregateID,
		&event.PartitionKey,
		&event.CorrelationID,
		&event.Payload,
		&event.Status,
		&event.Attempts,
		&event.LastError,
		&event.CreatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return domain.OutboxEvent{}, false, nil
		}
		return domain.OutboxEvent{}, false, fmt.Errorf("get outbox event: %w", err)
	}
	return event, true, nil
}

func (s *Store) IsEventProcessed(ctx context.Context, eventID string) (bool, error) {
	var processed bool
	if err := s.pool.QueryRow(
		ctx,
		`select exists(select 1 from processed_events where event_id = $1)`,
		eventID,
	).Scan(&processed); err != nil {
		return false, fmt.Errorf("check processed event: %w", err)
	}
	return processed, nil
}

func (s *Store) StartDeadLetterReplay(ctx context.Context, id, reason string) (string, error) {
	attemptID := domain.NewID("replay")
	if _, err := s.pool.Exec(
		ctx,
		`insert into dead_letter_replay_attempts (id, dead_letter_id, reason)
		 values ($1, $2, $3)`,
		attemptID,
		id,
		reason,
	); err != nil {
		return "", fmt.Errorf("insert dead letter replay attempt: %w", err)
	}
	return attemptID, nil
}

func (s *Store) FinishDeadLetterReplay(
	ctx context.Context,
	attemptID string,
	deadLetterID string,
	result operations.ReplayResult,
	errorMessage string,
) error {
	dbtx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin finish dead letter replay: %w", err)
	}
	defer func() { _ = dbtx.Rollback(ctx) }()
	var reason string
	if err := dbtx.QueryRow(
		ctx,
		`update dead_letter_replay_attempts
		    set result = $2, error_message = nullif($3, ''), completed_at = now()
		  where id = $1 and result = 'PENDING'
		  returning reason`,
		attemptID,
		result,
		errorMessage,
	).Scan(&reason); err != nil {
		return fmt.Errorf("finish dead letter replay attempt: %w", err)
	}
	if result == operations.ReplayPublished {
		if _, err := dbtx.Exec(
			ctx,
			`update dead_letter_messages
			    set state = 'REPUBLISHED', replay_count = replay_count + 1, version = version + 1
			  where id = $1 and state = 'OPEN'`,
			deadLetterID,
		); err != nil {
			return fmt.Errorf("mark dead letter republished: %w", err)
		}
		if _, err := dbtx.Exec(
			ctx,
			`insert into operator_actions (id, action_type, target_id, reason)
			 values ($1, 'DLQ_REPLAY', $2, $3)`,
			domain.NewID("act"),
			deadLetterID,
			reason,
		); err != nil {
			return fmt.Errorf("audit dead letter replay: %w", err)
		}
	}
	if err := dbtx.Commit(ctx); err != nil {
		return fmt.Errorf("commit finish dead letter replay: %w", err)
	}
	return nil
}

func (s *Store) GetOutboxStatus(ctx context.Context, id string) (domain.OutboxStatus, bool, error) {
	var status domain.OutboxStatus
	err := s.pool.QueryRow(ctx, `select status from outbox_events where id = $1`, id).Scan(&status)
	if err != nil {
		if err == pgx.ErrNoRows {
			return "", false, nil
		}
		return "", false, fmt.Errorf("get outbox status: %w", err)
	}
	return status, true, nil
}

func (s *Store) RequeueOutbox(ctx context.Context, id, reason string) error {
	dbtx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin requeue outbox: %w", err)
	}
	defer func() { _ = dbtx.Rollback(ctx) }()
	tag, err := dbtx.Exec(
		ctx,
		`update outbox_events
		    set status = $2, attempts = 0, last_error = null, updated_at = now()
		  where id = $1 and status = $3`,
		id,
		domain.OutboxPending,
		domain.OutboxDeadLettered,
	)
	if err != nil {
		return fmt.Errorf("requeue outbox event: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return operations.ErrInvalidState
	}
	if _, err := dbtx.Exec(
		ctx,
		`insert into operator_actions (id, action_type, target_id, reason)
		 values ($1, 'OUTBOX_REQUEUE', $2, $3)`,
		domain.NewID("act"),
		id,
		reason,
	); err != nil {
		return fmt.Errorf("audit outbox requeue: %w", err)
	}
	if err := dbtx.Commit(ctx); err != nil {
		return fmt.Errorf("commit outbox requeue: %w", err)
	}
	return nil
}

func encodeHeaders(headers []kafka.Header) ([]byte, error) {
	stored := make([]storedHeader, len(headers))
	for index, header := range headers {
		stored[index] = storedHeader{Key: header.Key, Value: append([]byte(nil), header.Value...)}
	}
	return json.Marshal(stored)
}

func decodeHeaders(payload []byte) ([]kafka.Header, error) {
	var stored []storedHeader
	if err := json.Unmarshal(payload, &stored); err != nil {
		return nil, err
	}
	headers := make([]kafka.Header, len(stored))
	for index, header := range stored {
		headers[index] = kafka.Header{Key: header.Key, Value: append([]byte(nil), header.Value...)}
	}
	return headers, nil
}
