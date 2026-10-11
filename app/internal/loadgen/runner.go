package loadgen

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"
)

// HTTPConfig configures an open-loop HTTP load generation workload.
type HTTPConfig struct {
	TargetURL        string
	RPS              float64
	Duration         time.Duration
	Workers          int
	Method           string
	Host             string
	Insecure         bool
	Client           *http.Client
	ProgressInterval time.Duration
	ProgressWriter   io.Writer
	DialContext      func(ctx context.Context, network, addr string) (net.Conn, error)
}

// RunHTTP executes an open-loop HTTP load generation workload.
// When Duration is <= 0, it runs indefinitely until ctx is canceled.
// In long runs, it prints a one-line progress summary periodically to ProgressWriter.
func RunHTTP(ctx context.Context, cfg HTTPConfig) (Stats, error) {
	if cfg.RPS <= 0 {
		return Stats{}, fmt.Errorf("rps must be positive")
	}
	if cfg.Workers <= 0 {
		return Stats{}, fmt.Errorf("workers must be positive")
	}
	if cfg.Method == "" {
		cfg.Method = "GET"
	}
	if cfg.TargetURL == "" {
		return Stats{}, fmt.Errorf("target URL is required")
	}

	client := cfg.Client
	if client == nil {
		client = NewClient(ClientConfig{
			Host:        cfg.Host,
			Insecure:    cfg.Insecure,
			Workers:     cfg.Workers,
			DialContext: cfg.DialContext,
		})
	}

	progressInterval := cfg.ProgressInterval
	if progressInterval <= 0 {
		progressInterval = 30 * time.Second
	}

	summary := NewSummary()
	jobs := make(chan struct{}, cfg.Workers*4)

	var wg sync.WaitGroup
	for i := 0; i < cfg.Workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range jobs {
				if ctx.Err() != nil {
					continue
				}

				req, err := http.NewRequestWithContext(ctx, cfg.Method, cfg.TargetURL, nil)
				if err != nil {
					summary.Record(0, 0, err)
					continue
				}
				if cfg.Host != "" {
					req.Host = cfg.Host
				}

				start := time.Now()
				resp, err := client.Do(req)
				latency := time.Since(start)

				if err != nil {
					summary.Record(0, latency, err)
					continue
				}

				_, _ = io.Copy(io.Discard, resp.Body)
				_ = resp.Body.Close()
				summary.Record(resp.StatusCode, latency, nil)
			}
		}()
	}

	tickerInterval := time.Duration(float64(time.Second) / cfg.RPS)
	if tickerInterval <= 0 {
		tickerInterval = time.Nanosecond
	}
	ticker := time.NewTicker(tickerInterval)
	defer ticker.Stop()

	progressTicker := time.NewTicker(progressInterval)
	defer progressTicker.Stop()

	testStart := time.Now()

	var timerChan <-chan time.Time
	if cfg.Duration > 0 {
		timer := time.NewTimer(cfg.Duration)
		defer timer.Stop()
		timerChan = timer.C
	}

producerLoop:
	for {
		select {
		case <-ctx.Done():
			break producerLoop
		case <-timerChan:
			break producerLoop
		case <-progressTicker.C:
			if cfg.ProgressWriter != nil {
				elapsed := time.Since(testStart)
				snap := summary.Calculate(elapsed)
				_, _ = fmt.Fprintln(cfg.ProgressWriter, snap.ProgressLine())
			}
		case <-ticker.C:
			select {
			case jobs <- struct{}{}:
			default:
				// Worker pool saturated; drop token to maintain open-loop rate
			}
		}
	}

	close(jobs)
	wg.Wait()
	actualDuration := time.Since(testStart)

	stats := summary.Calculate(actualDuration)
	return stats, nil
}
