package loadgen

import (
	"bytes"
	"context"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestRunHTTP_DurationZero_ContextCancelStopsCleanly(t *testing.T) {
	var requestCount atomic.Int64
	server, dialCtx := startHTTPTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requestCount.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())

	cfg := HTTPConfig{
		TargetURL:        server.URL,
		RPS:              100,
		Duration:         0, // Duration 0: run until canceled
		Workers:          4,
		Method:           "GET",
		ProgressInterval: 20 * time.Millisecond,
		ProgressWriter:   &bytes.Buffer{},
		DialContext:      dialCtx,
	}

	errCh := make(chan error, 1)
	statsCh := make(chan Stats, 1)

	go func() {
		stats, err := RunHTTP(ctx, cfg)
		if err != nil {
			errCh <- err
			return
		}
		statsCh <- stats
	}()

	// Let it run briefly to execute requests and at least one progress tick
	time.Sleep(60 * time.Millisecond)

	// Cancel context; must stop cleanly
	cancel()

	select {
	case err := <-errCh:
		t.Fatalf("RunHTTP returned unexpected error: %v", err)
	case stats := <-statsCh:
		if stats.TotalRequests == 0 {
			t.Errorf("expected requests to be recorded, got 0")
		}
		if stats.StatusCodes[http.StatusOK] == 0 {
			t.Errorf("expected 200 status codes, got %v", stats.StatusCodes)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("RunHTTP did not stop cleanly within 2 seconds after context cancellation")
	}
}

func TestRunHTTP_FiniteDuration(t *testing.T) {
	server, dialCtx := startHTTPTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	var buf bytes.Buffer
	cfg := HTTPConfig{
		TargetURL:        server.URL,
		RPS:              50,
		Duration:         100 * time.Millisecond,
		Workers:          2,
		Method:           "GET",
		ProgressInterval: 30 * time.Millisecond,
		ProgressWriter:   &buf,
		DialContext:      dialCtx,
	}

	stats, err := RunHTTP(context.Background(), cfg)
	if err != nil {
		t.Fatalf("RunHTTP failed: %v", err)
	}
	if stats.TotalRequests == 0 {
		t.Errorf("expected > 0 total requests, got %d", stats.TotalRequests)
	}
	if buf.Len() == 0 {
		t.Errorf("expected progress output in buffer, got empty")
	}
	if !strings.Contains(buf.String(), "Progress:") {
		t.Errorf("expected buffer to contain 'Progress:', got: %s", buf.String())
	}
}
