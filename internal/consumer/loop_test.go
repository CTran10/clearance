package consumer

import (
	"context"
	"errors"
	"testing"

	"github.com/segmentio/kafka-go"
)

func TestRunLoopRetriesDeadLettersAndCommits(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	reader := &fakeReader{message: kafka.Message{Key: []byte("txn_123")}}
	deadLetterer := &fakeDeadLetterer{}
	attempts := 0

	RunLoop(ctx, reader, deadLetterer, Config{Name: "test", MaxAttempts: 2}, func(context.Context, kafka.Message) error {
		attempts++
		if attempts == 2 {
			cancel()
		}
		return errors.New("process failed")
	})

	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
	}
	if deadLetterer.moves != 1 {
		t.Fatalf("dead letter moves = %d, want 1", deadLetterer.moves)
	}
	if deadLetterer.cause == nil || deadLetterer.cause.Error() != "process failed" {
		t.Fatalf("dead letter cause = %v, want final handler error", deadLetterer.cause)
	}
	if reader.commits != 1 {
		t.Fatalf("commits = %d, want 1", reader.commits)
	}
}

type fakeReader struct {
	message kafka.Message
	fetched bool
	commits int
}

func (r *fakeReader) FetchMessage(ctx context.Context) (kafka.Message, error) {
	if !r.fetched {
		r.fetched = true
		return r.message, nil
	}
	<-ctx.Done()
	return kafka.Message{}, ctx.Err()
}

func (r *fakeReader) CommitMessages(context.Context, ...kafka.Message) error {
	r.commits++
	return nil
}

type fakeDeadLetterer struct {
	moves int
	cause error
}

func (d *fakeDeadLetterer) Move(_ context.Context, _ kafka.Message, cause error) error {
	d.moves++
	d.cause = cause
	return nil
}
