package worker

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"sync"
	"time"

	"github.com/maximmalchevsky/pipe-infra/app/internal/config"
	"github.com/maximmalchevsky/pipe-infra/app/internal/metrics"
	"github.com/maximmalchevsky/pipe-infra/app/internal/store"
)

// Worker runs the queue claiming and message dispatch loop.
type Worker struct {
	store   store.Store
	cfg     *config.WorkerConfig
	metrics *metrics.WorkerMetrics
	logger  *slog.Logger

	mu      sync.Mutex
	stopCh  chan struct{}
	stopped bool
}

// New creates a new Worker instance.
func New(s store.Store, cfg *config.WorkerConfig, m *metrics.WorkerMetrics, logger *slog.Logger) *Worker {
	return &Worker{
		store:   s,
		cfg:     cfg,
		metrics: m,
		logger:  logger,
		stopCh:  make(chan struct{}),
	}
}

// Stop signals the worker to finish the current batch and stop claiming.
func (w *Worker) Stop() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.stopped {
		w.stopped = true
		close(w.stopCh)
	}
}

func (w *Worker) isStopped() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.stopped
}

// Run starts the worker loop until context is canceled or Stop() is called.
func (w *Worker) Run(ctx context.Context) error {
	w.logger.Info("waiting for messages table to be available...")
	if err := w.store.WaitForTables(ctx); err != nil {
		if errors.Is(err, context.Canceled) {
			return nil
		}
		return fmt.Errorf("wait for tables: %w", err)
	}

	w.logger.Info("starting worker claim loop",
		"batch_size", w.cfg.BatchSize,
		"batch_timeout", w.cfg.BatchTimeout,
		"send_latency_ms", w.cfg.SendLatencyMS,
		"send_fail_rate", w.cfg.SendFailRate,
		"max_attempts", w.cfg.MaxAttempts,
		"poll_interval", w.cfg.PollInterval,
	)

	latency := time.Duration(w.cfg.SendLatencyMS) * time.Millisecond

	for {
		// Check for graceful stop or context cancellation BETWEEN batches
		if ctx.Err() != nil || w.isStopped() {
			w.logger.Info("worker stop requested or context canceled, exiting worker loop cleanly")
			return nil
		}

		select {
		case <-w.stopCh:
			w.logger.Info("worker stop requested, exiting worker loop cleanly")
			return nil
		case <-ctx.Done():
			w.logger.Info("context canceled, exiting worker loop cleanly")
			return nil
		default:
		}

		if ctx.Err() != nil || w.isStopped() {
			return nil
		}

		// Detach in-flight batch from shutdown cancellation so current batch finishes and commits
		batchCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), w.cfg.BatchTimeout)
		batchStart := time.Now()

		claimed, err := w.store.ClaimAndProcess(batchCtx, w.cfg.BatchSize, func(procCtx context.Context, claims []store.MessageClaim) ([]store.MessageResult, error) {
			results := make([]store.MessageResult, len(claims))
			for i, claim := range claims {
				if latency > 0 {
					select {
					case <-procCtx.Done():
						return nil, procCtx.Err()
					case <-time.After(latency):
					}
				}

				failed := shouldFail(w.cfg.SendFailRate)
				newAttempts := claim.Attempts + 1

				if failed {
					if newAttempts >= w.cfg.MaxAttempts {
						results[i] = store.MessageResult{
							ID:       claim.ID,
							Status:   "failed",
							Attempts: newAttempts,
						}
						if w.metrics != nil {
							w.metrics.MessagesProcessed.WithLabelValues("failed").Inc()
						}
					} else {
						results[i] = store.MessageResult{
							ID:       claim.ID,
							Status:   "pending",
							Attempts: newAttempts,
						}
						if w.metrics != nil {
							w.metrics.MessagesProcessed.WithLabelValues("retry").Inc()
						}
					}
				} else {
					results[i] = store.MessageResult{
						ID:       claim.ID,
						Status:   "sent",
						Attempts: newAttempts,
					}
					if w.metrics != nil {
						w.metrics.MessagesProcessed.WithLabelValues("sent").Inc()
					}
				}
			}
			return results, nil
		})
		cancel()

		if err != nil {
			w.logger.Error("error claiming or processing batch", "error", err)
			select {
			case <-w.stopCh:
				return nil
			case <-ctx.Done():
				return nil
			case <-time.After(500 * time.Millisecond):
			}
			continue
		}

		batchDuration := time.Since(batchStart)
		if claimed > 0 {
			if w.metrics != nil {
				w.metrics.BatchSize.Observe(float64(claimed))
				w.metrics.BatchDuration.Observe(batchDuration.Seconds())
			}
			w.logger.Debug("processed batch", "claimed", claimed, "duration", batchDuration)
		} else {
			// Empty batch, sleep poll interval
			select {
			case <-w.stopCh:
				return nil
			case <-ctx.Done():
				return nil
			case <-time.After(w.cfg.PollInterval):
			}
		}

		if ctx.Err() != nil || w.isStopped() {
			w.logger.Info("worker stop requested or context canceled after batch processing, exiting worker loop cleanly")
			return nil
		}
	}
}

func shouldFail(rate float64) bool {
	if rate <= 0.0 {
		return false
	}
	if rate >= 1.0 {
		return true
	}
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return false
	}
	return float64(n.Int64())/1000000.0 < rate
}
