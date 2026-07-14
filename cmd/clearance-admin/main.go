package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/CTran10/clearance/internal/appenv"
	"github.com/CTran10/clearance/internal/deadletter"
	"github.com/CTran10/clearance/internal/kafkabus"
	"github.com/CTran10/clearance/internal/maintenance"
	"github.com/CTran10/clearance/internal/operations"
	"github.com/CTran10/clearance/internal/postgres"
)

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "clearance-admin:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	if len(args) < 2 {
		return usageError()
	}
	store, err := postgres.Open(ctx, appenv.Must("DATABASE_URL"))
	if err != nil {
		return err
	}
	defer store.Close()
	broker := kafkabus.NewPublisher(appenv.CSV("KAFKA_BROKERS", []string{"redpanda:9092"}))
	defer func() { _ = broker.Close() }()
	replayWindow := appenv.DurationSeconds("REPLAY_WINDOW_SECONDS", 14*24*time.Hour)
	operationsService := operations.NewService(store, broker, operations.Config{ReplayWindow: replayWindow})
	maintenanceService, err := maintenance.NewProcessedEventsService(store, maintenance.Config{
		Retention:    appenv.DurationSeconds("PROCESSED_EVENT_RETENTION_SECONDS", 30*24*time.Hour),
		ReplayWindow: replayWindow,
		BatchSize:    appenv.Int("PROCESSED_EVENT_PRUNE_BATCH_SIZE", 1_000),
	})
	if err != nil {
		return err
	}

	switch args[0] {
	case "dlq":
		return runDLQ(ctx, store, operationsService, args[1:])
	case "outbox":
		return runOutbox(ctx, store, operationsService, args[1:])
	case "processed-events":
		return runProcessed(ctx, maintenanceService, args[1:])
	default:
		return usageError()
	}
}

func runDLQ(ctx context.Context, store *postgres.Store, service *operations.Service, args []string) error {
	switch args[0] {
	case "list":
		state := deadletter.State("")
		if len(args) > 1 {
			state = deadletter.State(strings.ToUpper(args[1]))
		}
		items, err := store.ListDeadLetters(ctx, state, 50)
		return writeResult(items, err)
	case "show":
		if len(args) != 2 {
			return usageError()
		}
		item, ok, err := store.GetDeadLetter(ctx, args[1])
		if err != nil {
			return err
		}
		if !ok {
			return operations.ErrNotFound
		}
		return writeJSON(item)
	case "replay":
		if len(args) < 3 {
			return usageError()
		}
		item, err := service.ReplayDeadLetter(ctx, args[1], strings.Join(args[2:], " "))
		return writeResult(item, err)
	default:
		return usageError()
	}
}

func runOutbox(ctx context.Context, store *postgres.Store, service *operations.Service, args []string) error {
	switch args[0] {
	case "list-dead":
		items, err := store.ListDeadOutbox(ctx, 50)
		return writeResult(items, err)
	case "show":
		if len(args) != 2 {
			return usageError()
		}
		item, ok, err := store.GetOutboxEvent(ctx, args[1])
		if err != nil {
			return err
		}
		if !ok {
			return operations.ErrNotFound
		}
		return writeJSON(item)
	case "requeue":
		if len(args) < 3 {
			return usageError()
		}
		status, err := service.RequeueOutbox(ctx, args[1], strings.Join(args[2:], " "))
		return writeResult(map[string]string{"id": args[1], "status": string(status)}, err)
	default:
		return usageError()
	}
}

func runProcessed(ctx context.Context, service *maintenance.ProcessedEventsService, args []string) error {
	switch args[0] {
	case "stats":
		result, err := service.Stats(ctx)
		return writeResult(result, err)
	case "preview":
		result, err := service.Preview(ctx)
		return writeResult(result, err)
	case "prune":
		if len(args) < 2 {
			return usageError()
		}
		if strings.TrimSpace(os.Getenv("CLEARANCE_ADMIN_CONFIRM")) != "yes" {
			return errors.New("set CLEARANCE_ADMIN_CONFIRM=yes to execute a prune")
		}
		result, err := service.Prune(ctx, strings.Join(args[1:], " "))
		return writeResult(result, err)
	default:
		return usageError()
	}
}

func writeResult(value any, err error) error {
	if err != nil {
		return err
	}
	return writeJSON(value)
}

func writeJSON(value any) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func usageError() error {
	return errors.New("usage: clearance-admin dlq (list [state]|show ID|replay ID REASON) | outbox (list-dead|show ID|requeue ID REASON) | processed-events (stats|preview|prune REASON)")
}
