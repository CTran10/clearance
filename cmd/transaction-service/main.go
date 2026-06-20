package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/CTran10/clearance/internal/appenv"
	"github.com/CTran10/clearance/internal/httpapi"
	"github.com/CTran10/clearance/internal/postgres"
	"github.com/CTran10/clearance/internal/redislimiter"
	"github.com/CTran10/clearance/internal/transaction"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	store, err := postgres.Open(ctx, appenv.Must("DATABASE_URL"))
	if err != nil {
		slog.Error("postgres startup failed")
		os.Exit(1)
	}
	defer store.Close()

	limiter := redislimiter.Open(
		appenv.String("REDIS_ADDR", "redis:6379"),
		appenv.Int("RATE_LIMIT_MAX_REQUESTS", 60),
		appenv.DurationSeconds("RATE_LIMIT_WINDOW_SECONDS", time.Minute),
	)
	defer func() {
		_ = limiter.Close()
	}()

	handler := httpapi.NewRouter(
		transaction.NewService(store),
		limiter,
		httpapi.Config{
			AuthValue:      appenv.Must("TRANSACTION_API_AUTH_VALUE"),
			AllowedOrigins: appenv.CSV("CORS_ORIGINS", nil),
		},
	)
	server := &http.Server{
		Addr:              ":" + appenv.String("PORT", "8080"),
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	slog.Info("transaction service listening", "addr", server.Addr)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("transaction service stopped unexpectedly")
		os.Exit(1)
	}
}
