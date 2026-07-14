package outbox

import (
	"context"
	"fmt"
	"time"

	"github.com/CTran10/clearance/internal/domain"
	"github.com/CTran10/clearance/internal/metrics"
)

type Config struct {
	MaxAttempts int
}

type Store interface {
	NextPending(ctx context.Context) (domain.OutboxEvent, bool, error)
	MarkPublished(ctx context.Context, eventID string) error
	MarkFailedAttempt(ctx context.Context, eventID string, maxAttempts int) error
}

type PublishFunc func(ctx context.Context, event domain.OutboxEvent) error

type Publisher struct {
	store       Store
	publish     PublishFunc
	maxAttempts int
}

func NewPublisher(store Store, publish PublishFunc, config Config) *Publisher {
	maxAttempts := config.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 3
	}
	return &Publisher{store: store, publish: publish, maxAttempts: maxAttempts}
}

func (p *Publisher) PublishNext(ctx context.Context) (bool, error) {
	event, ok, err := p.store.NextPending(ctx)
	if err != nil {
		return false, fmt.Errorf("load pending outbox event: %w", err)
	}
	if !ok {
		return false, nil
	}

	// if kafka's having a moment and publish fails, we DON'T just retry forever — that's how one poison event
	// jams the whole queue. MarkFailedAttempt bumps a counter and once it hits maxAttempts the event gets
	// "dead lettered" (parked aside) so the line keeps moving. learned the term "poison message" from this exact problem
	started := time.Now()
	if err := p.publish(ctx, event); err != nil {
		result := "failed_attempt"
		if event.Attempts+1 >= p.maxAttempts {
			result = "dead_lettered" // this attempt is the one that tips it over the edge → park it
		}
		if markErr := p.store.MarkFailedAttempt(ctx, event.ID, p.maxAttempts); markErr != nil {
			return true, fmt.Errorf("mark failed outbox event: %w", markErr)
		}
		metrics.OutboxPublish(result, time.Since(started))
		// note the %w — wrapping the error keeps the original cause attached so callers can errors.Is/As it later.
		// took me a bit to stop just doing fmt.Errorf("...%v") and losing the actual error underneath
		return true, fmt.Errorf("publish outbox event: %w", err)
	}

	if err := p.store.MarkPublished(ctx, event.ID); err != nil {
		return true, fmt.Errorf("mark published outbox event: %w", err)
	}
	metrics.OutboxPublish("published", time.Since(started))
	return true, nil
}

func (p *Publisher) PublishAvailable(ctx context.Context) error {
	for {
		found, err := p.PublishNext(ctx)
		if err != nil {
			return err
		}
		if !found {
			return nil
		}
	}
}
