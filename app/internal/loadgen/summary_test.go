package loadgen

import (
	"errors"
	"testing"
	"time"
)

func TestCalculateStats(t *testing.T) {
	// 100 requests with latencies 1ms, 2ms, ..., 100ms
	latencies := make([]time.Duration, 100)
	for i := 0; i < 100; i++ {
		latencies[i] = time.Duration(i+1) * time.Millisecond
	}

	statusCodes := map[int]int{
		200: 95,
		500: 5,
	}

	duration := 2 * time.Second
	stats := CalculateStats(100, statusCodes, 0, latencies, duration)

	if stats.TotalRequests != 100 {
		t.Errorf("expected total 100, got %d", stats.TotalRequests)
	}
	if stats.AchievedRPS != 50.0 {
		t.Errorf("expected RPS 50.0, got %f", stats.AchievedRPS)
	}
	if stats.StatusCodes[200] != 95 || stats.StatusCodes[500] != 5 {
		t.Errorf("status codes mismatch: %+v", stats.StatusCodes)
	}

	// 50th percentile (index 50) is 51ms
	if stats.P50 != 51*time.Millisecond {
		t.Errorf("expected p50 = 51ms, got %v", stats.P50)
	}
	// 95th percentile (index 95) is 96ms
	if stats.P95 != 96*time.Millisecond {
		t.Errorf("expected p95 = 96ms, got %v", stats.P95)
	}
	// 99th percentile (index 99) is 100ms
	if stats.P99 != 100*time.Millisecond {
		t.Errorf("expected p99 = 100ms, got %v", stats.P99)
	}
	if stats.Min != 1*time.Millisecond {
		t.Errorf("expected min = 1ms, got %v", stats.Min)
	}
	if stats.Max != 100*time.Millisecond {
		t.Errorf("expected max = 100ms, got %v", stats.Max)
	}
}

func TestSummaryRecord(t *testing.T) {
	s := NewSummary()
	s.Record(200, 10*time.Millisecond, nil)
	s.Record(200, 20*time.Millisecond, nil)
	s.Record(500, 50*time.Millisecond, nil)
	s.Record(0, 5*time.Millisecond, errors.New("connection failed"))

	stats := s.Calculate(1 * time.Second)
	if stats.TotalRequests != 4 {
		t.Errorf("expected 4 total, got %d", stats.TotalRequests)
	}
	if stats.Errors != 1 {
		t.Errorf("expected 1 error, got %d", stats.Errors)
	}
	if stats.StatusCodes[200] != 2 {
		t.Errorf("expected 2 status 200, got %d", stats.StatusCodes[200])
	}
	if stats.StatusCodes[500] != 1 {
		t.Errorf("expected 1 status 500, got %d", stats.StatusCodes[500])
	}
}
