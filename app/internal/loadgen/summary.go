// Package loadgen provides statistics collection and latency calculation for load generation.
package loadgen

import (
	"fmt"
	"sort"
	"sync"
	"time"
)

// Summary accumulates request statistics.
type Summary struct {
	mu          sync.Mutex
	statusCodes map[int]int
	errorCount  int
	latencies   []time.Duration
	total       int
}

// Stats represents calculated loadgen results.
type Stats struct {
	TotalRequests int
	StatusCodes   map[int]int
	Errors        int
	AchievedRPS   float64
	P50           time.Duration
	P95           time.Duration
	P99           time.Duration
	Min           time.Duration
	Max           time.Duration
	Mean          time.Duration
}

// NewSummary returns an initialized Summary.
func NewSummary() *Summary {
	return &Summary{
		statusCodes: make(map[int]int),
		latencies:   make([]time.Duration, 0, 1024),
	}
}

// Record records a single request outcome.
func (s *Summary) Record(statusCode int, latency time.Duration, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.total++
	if err != nil {
		s.errorCount++
	} else {
		s.statusCodes[statusCode]++
	}
	s.latencies = append(s.latencies, latency)
}

// Calculate computes summary statistics for the given test duration.
func (s *Summary) Calculate(duration time.Duration) Stats {
	s.mu.Lock()
	defer s.mu.Unlock()
	return CalculateStats(s.total, s.statusCodes, s.errorCount, s.latencies, duration)
}

// CalculateStats performs percentile and throughput calculations.
func CalculateStats(
	total int,
	statusCodes map[int]int,
	errCount int,
	latencies []time.Duration,
	duration time.Duration,
) Stats {
	scCopy := make(map[int]int, len(statusCodes))
	for k, v := range statusCodes {
		scCopy[k] = v
	}

	achievedRPS := 0.0
	if duration > 0 {
		achievedRPS = float64(total) / duration.Seconds()
	}

	if len(latencies) == 0 {
		return Stats{
			TotalRequests: total,
			StatusCodes:   scCopy,
			Errors:        errCount,
			AchievedRPS:   achievedRPS,
		}
	}

	sorted := make([]time.Duration, len(latencies))
	copy(sorted, latencies)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	var sum time.Duration
	for _, l := range sorted {
		sum += l
	}

	return Stats{
		TotalRequests: total,
		StatusCodes:   scCopy,
		Errors:        errCount,
		AchievedRPS:   achievedRPS,
		P50:           Percentile(sorted, 0.50),
		P95:           Percentile(sorted, 0.95),
		P99:           Percentile(sorted, 0.99),
		Min:           sorted[0],
		Max:           sorted[len(sorted)-1],
		Mean:          sum / time.Duration(len(sorted)),
	}
}

// Percentile calculates the nearest-rank percentile on sorted duration slice.
func Percentile(sorted []time.Duration, p float64) time.Duration {
	n := len(sorted)
	if n == 0 {
		return 0
	}
	if p <= 0 {
		return sorted[0]
	}
	if p >= 1.0 {
		return sorted[n-1]
	}

	idx := int(float64(n) * p)
	if idx >= n {
		idx = n - 1
	}
	return sorted[idx]
}

// PrintReport prints formatted test results.
func (s Stats) PrintReport() {
	fmt.Println("=== Loadgen Summary ===")
	fmt.Printf("Total Requests:  %d\n", s.TotalRequests)
	fmt.Printf("Achieved RPS:    %.2f\n", s.AchievedRPS)
	fmt.Println("Status Codes:")
	for code, count := range s.StatusCodes {
		fmt.Printf("  %d: %d\n", code, count)
	}
	if s.Errors > 0 {
		fmt.Printf("Network/Other Errors: %d\n", s.Errors)
	}
	fmt.Printf("Latencies:\n")
	fmt.Printf("  p50:  %v\n", s.P50)
	fmt.Printf("  p95:  %v\n", s.P95)
	fmt.Printf("  p99:  %v\n", s.P99)
	fmt.Printf("  min:  %v\n", s.Min)
	fmt.Printf("  max:  %v\n", s.Max)
	fmt.Printf("  mean: %v\n", s.Mean)
}
