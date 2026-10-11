package api

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/maximmalchevsky/pipe-infra/app/internal/fault"
	"github.com/maximmalchevsky/pipe-infra/app/internal/metrics"
	"github.com/maximmalchevsky/pipe-infra/app/internal/store"
)

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

// NewRouter constructs the complete HTTP handler including middleware and routes.
func NewRouter(
	s store.Store,
	m *metrics.APIMetrics,
	version string,
	faultLatencyMS int,
	faultErrorRate float64,
) http.Handler {
	server := NewServer(s, m, version)
	mux := http.NewServeMux()

	// API routes
	mux.HandleFunc("POST /api/v1/campaigns", server.HandleCreateCampaign)
	mux.HandleFunc("GET /api/v1/campaigns/{id}", server.HandleGetCampaign)
	mux.HandleFunc("GET /api/v1/campaigns", server.HandleListCampaigns)

	// Health and management routes
	mux.HandleFunc("GET /healthz", server.HandleHealthz)
	mux.HandleFunc("GET /readyz", server.HandleReadyz)
	mux.HandleFunc("GET /version", server.HandleVersion)
	mux.Handle("GET /metrics", promhttp.Handler())

	// Middleware chain:
	// 1. Fault middleware (only affects /api/ routes)
	// 2. Metrics middleware (records http_requests_total and duration histogram using route patterns)
	faultMW := fault.Middleware(faultLatencyMS, faultErrorRate)
	metricsHandler := faultMW(mux)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

		metricsHandler.ServeHTTP(rec, r)

		if m != nil {
			duration := time.Since(start)
			route := r.Pattern
			if route == "" {
				route = "unknown"
			} else {
				// Pattern is like "GET /api/v1/campaigns/{id}", extract route pattern
				parts := strings.SplitN(route, " ", 2)
				if len(parts) == 2 {
					route = parts[1]
				}
			}
			m.RequestsTotal.WithLabelValues(r.Method, route, strconv.Itoa(rec.status), version).Inc()
			m.RequestDuration.WithLabelValues(route, version).Observe(duration.Seconds())
		}
	})
}
