package health

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/CTran10/clearance/internal/metrics"
)

func Start(ctx context.Context, addr string, metricsEnabled bool) {
	server := &http.Server{
		Addr:              addr,
		Handler:           handler(metricsEnabled),
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
	go func() {
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Warn("health server stopped unexpectedly", "err", err)
		}
	}()
}

func handler(metricsEnabled bool) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		response.WriteHeader(http.StatusOK)
		_, _ = response.Write([]byte(`{"status":"ok"}`))
	})
	if metricsEnabled {
		mux.Handle("/metrics", metrics.Handler())
	}
	return mux
}
