// Package metrics defines and registers Prometheus metrics for API and worker services.
package metrics

import (
	"context"
	"log/slog"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// APIMetrics holds Prometheus metrics for notifier-api.
type APIMetrics struct {
	RequestsTotal    *prometheus.CounterVec
	RequestDuration  *prometheus.HistogramVec
	CampaignsCreated prometheus.Counter
	QueuePending     prometheus.Gauge
	BuildInfo        *prometheus.GaugeVec
	version          string
}

// NewAPIMetrics registers and returns APIMetrics.
func NewAPIMetrics(version string, reg prometheus.Registerer) *APIMetrics {
	if reg == nil {
		reg = prometheus.DefaultRegisterer
	}
	factory := promauto.With(reg)

	m := &APIMetrics{
		RequestsTotal: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: "http_requests_total",
				Help: "Total number of HTTP requests processed.",
			},
			[]string{"method", "route", "code", "version"},
		),
		RequestDuration: factory.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "http_request_duration_seconds",
				Help:    "Duration of HTTP requests in seconds.",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"route", "version"},
		),
		CampaignsCreated: factory.NewCounter(
			prometheus.CounterOpts{
				Name: "notifier_campaigns_created_total",
				Help: "Total number of SMS campaigns created.",
			},
		),
		QueuePending: factory.NewGauge(
			prometheus.GaugeOpts{
				Name: "notifier_queue_pending",
				Help: "Current number of pending messages waiting to be sent.",
			},
		),
		BuildInfo: factory.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "notifier_build_info",
				Help: "Build and version info.",
			},
			[]string{"version"},
		),
		version: version,
	}

	m.BuildInfo.WithLabelValues(version).Set(1)
	return m
}

// StartQueuePendingCollector periodically updates notifier_queue_pending gauge every 5 seconds.
func (m *APIMetrics) StartQueuePendingCollector(
	ctx context.Context,
	getPendingCount func(ctx context.Context) (int64, error),
	logger *slog.Logger,
) {
	ticker := time.NewTicker(5 * time.Second)
	go func() {
		defer ticker.Stop()
		// Initial fetch
		if count, err := getPendingCount(ctx); err == nil {
			m.QueuePending.Set(float64(count))
		}
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				count, err := getPendingCount(ctx)
				if err != nil {
					logger.Warn("failed to fetch pending queue count", "error", err)
				} else {
					m.QueuePending.Set(float64(count))
				}
			}
		}
	}()
}

// WorkerMetrics holds Prometheus metrics for notifier-worker.
type WorkerMetrics struct {
	MessagesProcessed *prometheus.CounterVec
	BatchDuration     prometheus.Histogram
	BatchSize         prometheus.Histogram
	BuildInfo         *prometheus.GaugeVec
	version           string
}

// NewWorkerMetrics registers and returns WorkerMetrics.
func NewWorkerMetrics(version string, reg prometheus.Registerer) *WorkerMetrics {
	if reg == nil {
		reg = prometheus.DefaultRegisterer
	}
	factory := promauto.With(reg)

	m := &WorkerMetrics{
		MessagesProcessed: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: "notifier_messages_processed_total",
				Help: "Total number of messages processed by worker by result.",
			},
			[]string{"result"},
		),
		BatchDuration: factory.NewHistogram(
			prometheus.HistogramOpts{
				Name:    "notifier_batch_duration_seconds",
				Help:    "Duration of batch processing in seconds.",
				Buckets: prometheus.DefBuckets,
			},
		),
		BatchSize: factory.NewHistogram(
			prometheus.HistogramOpts{
				Name:    "notifier_batch_size",
				Help:    "Number of messages claimed per batch.",
				Buckets: []float64{1, 5, 10, 25, 50, 100, 200, 500},
			},
		),
		BuildInfo: factory.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "notifier_build_info",
				Help: "Build and version info.",
			},
			[]string{"version"},
		),
		version: version,
	}

	m.BuildInfo.WithLabelValues(version).Set(1)
	return m
}
