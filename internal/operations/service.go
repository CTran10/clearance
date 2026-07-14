package operations

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/CTran10/clearance/internal/deadletter"
	"github.com/CTran10/clearance/internal/domain"
	"github.com/segmentio/kafka-go"
)

var (
	ErrNotFound            = errors.New("operation target not found")
	ErrAlreadyProcessed    = errors.New("event was already processed")
	ErrReplayWindowExpired = errors.New("replay window expired")
	ErrInvalidState        = errors.New("operation target is in an invalid state")
	ErrInvalidReason       = errors.New("operator reason is invalid")
)

type ReplayResult string

const (
	ReplayPublished ReplayResult = "PUBLISHED"
	ReplayFailed    ReplayResult = "FAILED"
)

type Config struct {
	ReplayWindow time.Duration
	Now          func() time.Time
}

type Store interface {
	GetDeadLetter(ctx context.Context, id string) (deadletter.Record, bool, error)
	IsEventProcessed(ctx context.Context, eventID string) (bool, error)
	StartDeadLetterReplay(ctx context.Context, id, reason string) (string, error)
	FinishDeadLetterReplay(ctx context.Context, attemptID, deadLetterID string, result ReplayResult, errorMessage string) error
	GetOutboxStatus(ctx context.Context, id string) (domain.OutboxStatus, bool, error)
	RequeueOutbox(ctx context.Context, id, reason string) error
}

type Broker interface {
	PublishMessage(ctx context.Context, topic string, message kafka.Message) error
}

type Service struct {
	store        Store
	broker       Broker
	replayWindow time.Duration
	now          func() time.Time
}

func NewService(store Store, broker Broker, config Config) *Service {
	replayWindow := config.ReplayWindow
	if replayWindow <= 0 {
		replayWindow = 14 * 24 * time.Hour
	}
	now := config.Now
	if now == nil {
		now = time.Now
	}
	return &Service{store: store, broker: broker, replayWindow: replayWindow, now: now}
}

func (s *Service) ReplayDeadLetter(ctx context.Context, id, reason string) (deadletter.Record, error) {
	if !validReason(reason) {
		return deadletter.Record{}, ErrInvalidReason
	}
	record, ok, err := s.store.GetDeadLetter(ctx, id)
	if err != nil {
		return deadletter.Record{}, fmt.Errorf("get dead letter: %w", err)
	}
	if !ok {
		return deadletter.Record{}, ErrNotFound
	}
	if record.State != deadletter.StateOpen {
		return deadletter.Record{}, ErrInvalidState
	}
	if record.FirstFailedAt.Before(s.now().UTC().Add(-s.replayWindow)) {
		return deadletter.Record{}, ErrReplayWindowExpired
	}
	if record.EventID != "" {
		processed, err := s.store.IsEventProcessed(ctx, record.EventID)
		if err != nil {
			return deadletter.Record{}, fmt.Errorf("check processed event: %w", err)
		}
		if processed {
			return deadletter.Record{}, ErrAlreadyProcessed
		}
	}
	attemptID, err := s.store.StartDeadLetterReplay(ctx, record.ID, strings.TrimSpace(reason))
	if err != nil {
		return deadletter.Record{}, fmt.Errorf("start dead letter replay: %w", err)
	}
	message := kafka.Message{
		Key: append([]byte(nil), record.Key...), Value: append([]byte(nil), record.Payload...),
		Headers: cloneHeaders(record.Headers),
	}
	if err := s.broker.PublishMessage(ctx, record.SourceTopic, message); err != nil {
		_ = s.store.FinishDeadLetterReplay(ctx, attemptID, record.ID, ReplayFailed, boundedError(err))
		return deadletter.Record{}, fmt.Errorf("publish dead letter replay: %w", err)
	}
	if err := s.store.FinishDeadLetterReplay(ctx, attemptID, record.ID, ReplayPublished, ""); err != nil {
		return deadletter.Record{}, fmt.Errorf("finish dead letter replay: %w", err)
	}
	record.State = deadletter.StateRepublished
	record.ReplayCount++
	return record, nil
}

func (s *Service) RequeueOutbox(ctx context.Context, id, reason string) (domain.OutboxStatus, error) {
	if !validReason(reason) {
		return "", ErrInvalidReason
	}
	status, ok, err := s.store.GetOutboxStatus(ctx, id)
	if err != nil {
		return "", fmt.Errorf("get outbox status: %w", err)
	}
	if !ok {
		return "", ErrNotFound
	}
	if status != domain.OutboxDeadLettered {
		return "", ErrInvalidState
	}
	if err := s.store.RequeueOutbox(ctx, id, strings.TrimSpace(reason)); err != nil {
		return "", fmt.Errorf("requeue outbox: %w", err)
	}
	return domain.OutboxPending, nil
}

func validReason(reason string) bool {
	reason = strings.TrimSpace(reason)
	if len(reason) == 0 || len(reason) > 256 {
		return false
	}
	for _, value := range reason {
		if unicode.IsControl(value) {
			return false
		}
	}
	return true
}

func boundedError(err error) string {
	message := []rune(err.Error())
	if len(message) > 512 {
		message = message[:512]
	}
	return string(message)
}

func cloneHeaders(headers []kafka.Header) []kafka.Header {
	cloned := make([]kafka.Header, len(headers))
	for index, header := range headers {
		cloned[index] = kafka.Header{Key: header.Key, Value: append([]byte(nil), header.Value...)}
	}
	return cloned
}
