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

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/maximmalchevsky/pipe-infra/app/internal/config"
	"github.com/maximmalchevsky/pipe-infra/app/internal/metrics"
	"github.com/maximmalchevsky/pipe-infra/app/internal/store"
	"github.com/maximmalchevsky/pipe-infra/app/internal/worker"
)

var version = "dev"

func main() {
	cfg, err := config.LoadWorkerConfig(version)
	if err != nil {
		slog.Error("failed to load worker configuration", "error", err)
		os.Exit(1)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil)).With(
		"service", "notifier-worker",
		"version", cfg.AppVersion,
	)
	slog.SetDefault(logger)

	logger.Info("starting notifier-worker",
		"http_addr", cfg.HTTPAddr,
		"batch_size", cfg.BatchSize,
		"batch_timeout", cfg.BatchTimeout,
		"send_latency_ms", cfg.SendLatencyMS,
		"send_fail_rate", cfg.SendFailRate,
		"max_attempts", cfg.MaxAttempts,
		"poll_interval", cfg.PollInterval,
	)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Initialize Postgres store
	pgStore, err := store.NewPostgresStore(ctx, cfg.DatabaseURL, logger)
	if err != nil {
		logger.Error("failed to initialize postgres store", "error", err)
		os.Exit(1)
	}
	defer pgStore.Close()

	// Metrics
	workerMetrics := metrics.NewWorkerMetrics(cfg.AppVersion, nil)

	// Worker process
	w := worker.New(pgStore, cfg, workerMetrics, logger)

	workerDone := make(chan struct{})
	go func() {
		if err := w.Run(ctx); err != nil {
			logger.Error("worker loop exited with error", "error", err)
		}
		close(workerDone)
	}()

	// Health and metrics HTTP server
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	mux.Handle("GET /metrics", promhttp.Handler())

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		logger.Info("health and metrics server listening", "addr", cfg.HTTPAddr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("HTTP server error", "error", err)
		}
	}()

	// Wait for shutdown signal
	<-ctx.Done()
	logger.Info("SIGTERM received, finishing current batch and stopping worker (KEDA scale-in friendly)...")

	// Stop claiming new batches
	w.Stop()

	// Wait for current batch to complete
	waitTimeout := cfg.BatchTimeout + 5*time.Second
	select {
	case <-workerDone:
		logger.Info("current batch finished")
	case <-time.After(waitTimeout):
		logger.Warn("timeout waiting for worker to finish current batch", "timeout", waitTimeout)
	}

	// Shutdown HTTP server
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = server.Shutdown(shutdownCtx)

	logger.Info("notifier-worker exited cleanly")
}
