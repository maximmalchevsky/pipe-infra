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

	"github.com/maximmalchevsky/pipe-infra/app/internal/api"
	"github.com/maximmalchevsky/pipe-infra/app/internal/config"
	"github.com/maximmalchevsky/pipe-infra/app/internal/metrics"
	"github.com/maximmalchevsky/pipe-infra/app/internal/migrate"
	"github.com/maximmalchevsky/pipe-infra/app/internal/store"
)

var version = "dev"

func main() {
	cfg, err := config.LoadAPIConfig(version)
	if err != nil {
		slog.Error("failed to load configuration", "error", err)
		os.Exit(1)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil)).With(
		"service", "notifier-api",
		"version", cfg.AppVersion,
	)
	slog.SetDefault(logger)

	logger.Info("starting notifier-api",
		"http_addr", cfg.HTTPAddr,
		"fault_error_rate", cfg.FaultErrorRate,
		"fault_latency_ms", cfg.FaultLatencyMS,
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

	// Run migrations under advisory lock
	if err := migrate.Run(ctx, pgStore.Pool(), logger); err != nil {
		logger.Error("failed to run database migrations", "error", err)
		os.Exit(1)
	}

	// Metrics
	apiMetrics := metrics.NewAPIMetrics(cfg.AppVersion, nil)
	apiMetrics.StartQueuePendingCollector(ctx, pgStore.GetPendingCount, logger)

	// Router and HTTP server
	router := api.NewRouter(pgStore, apiMetrics, cfg.AppVersion, cfg.FaultLatencyMS, cfg.FaultErrorRate)
	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		logger.Info("HTTP server listening", "addr", cfg.HTTPAddr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	select {
	case <-ctx.Done():
		logger.Info("shutdown signal received, initiating graceful shutdown")
	case err := <-serverErr:
		logger.Error("HTTP server error", "error", err)
		os.Exit(1)
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("error during server shutdown", "error", err)
	}

	logger.Info("notifier-api stopped cleanly")
}
