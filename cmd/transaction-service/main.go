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
	"github.com/CTran10/clearance/internal/funding"
	"github.com/CTran10/clearance/internal/httpapi"
	"github.com/CTran10/clearance/internal/metrics"
	"github.com/CTran10/clearance/internal/postgres"
	"github.com/CTran10/clearance/internal/redislimiter"
	"github.com/CTran10/clearance/internal/transaction"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	store, err := postgres.Open(ctx, appenv.Must("DATABASE_URL"))
	if err != nil {
		slog.Error("postgres startup failed", "err", err)
		os.Exit(1)
	}
	defer store.Close()
	metricsEnabled := appenv.Bool("METRICS_ENABLED", false)
	metrics.Configure("transaction-service")
	if metricsEnabled {
		metrics.StartSampler(ctx, appenv.DurationSeconds("METRICS_SAMPLE_SECONDS", 15*time.Second), store)
	}

	limiter := redislimiter.Open(
		appenv.String("REDIS_ADDR", "redis:6379"),
		appenv.Int("RATE_LIMIT_MAX_REQUESTS", 60),
		appenv.DurationSeconds("RATE_LIMIT_WINDOW_SECONDS", time.Minute),
	)
	defer func() {
		_ = limiter.Close()
	}()

	transactionService := transaction.NewService(store)
	handler := httpapi.NewRouter(
		transactionService,
		limiter,
		httpapi.Config{
			AuthValue:         appenv.Must("TRANSACTION_API_AUTH_VALUE"),
			FundingAuthValue:  appenv.Must("FUNDING_API_AUTH_VALUE"),
			OperatorAuthValue: appenv.Must("OPERATOR_API_AUTH_VALUE"),
			AllowedOrigins:    appenv.CSV("CORS_ORIGINS", nil),
			TrustForwardedFor: appenv.Bool("TRUST_X_FORWARDED_FOR", false),
			MetricsEnabled:    metricsEnabled,
		},
		httpapi.WithQueryService(transaction.NewQueryService(store)),
		httpapi.WithFundingService(funding.NewService(store, funding.Config{
			MaxAmountCents: int64(appenv.Int("FUNDING_MAX_AMOUNT_CENTS", 100_000_000)),
		})),
	)
	server := &http.Server{
		Addr:              ":" + appenv.String("PORT", "8080"),
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	slog.Info("transaction service listening", "addr", server.Addr)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("transaction service stopped unexpectedly", "err", err)
		os.Exit(1)
	}
}
