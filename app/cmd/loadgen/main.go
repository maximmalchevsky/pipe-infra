// Package main provides the loadgen CLI tool for generating campaigns and HTTP load.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/maximmalchevsky/pipe-infra/app/internal/loadgen"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "campaign":
		runCampaignCmd(os.Args[2:])
	case "http":
		runHTTPCmd(os.Args[2:])
	case "-h", "--help", "help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("Usage: loadgen <subcommand> [flags]")
	fmt.Println()
	fmt.Println("Subcommands:")
	fmt.Println("  campaign   Create bulk SMS campaigns")
	fmt.Println("             loadgen campaign -url http://api:8080 -recipients 50000 -text \"promo\" [-count 1]")
	fmt.Println()
	fmt.Println("  http       Generate HTTP traffic (open-loop) against an endpoint")
	fmt.Println("             loadgen http -url http://api:8080/api/v1/campaigns -rps 300 -duration 2m [-workers 64] [-method GET]")
}

func runCampaignCmd(args []string) {
	fs := flag.NewFlagSet("campaign", flag.ExitOnError)
	targetURL := fs.String("url", "http://localhost:8080", "Target base URL (e.g. http://localhost:8080)")
	recipients := fs.Int("recipients", 50000, "Number of recipients per campaign")
	text := fs.String("text", "Hackathon campaign promo", "SMS text (1..160 chars)")
	count := fs.Int("count", 1, "Number of campaigns to create")
	_ = fs.Parse(args)

	apiEndpoint := strings.TrimRight(*targetURL, "/")
	if !strings.HasSuffix(apiEndpoint, "/api/v1/campaigns") {
		apiEndpoint += "/api/v1/campaigns"
	}

	client := &http.Client{Timeout: 30 * time.Second}

	for i := 0; i < *count; i++ {
		payload, _ := json.Marshal(map[string]any{
			"text":       *text,
			"recipients": *recipients,
		})

		start := time.Now()
		resp, err := client.Post(apiEndpoint, "application/json", bytes.NewReader(payload))
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to send request %d: %v\n", i+1, err)
			continue
		}

		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()

		if resp.StatusCode != http.StatusCreated {
			fmt.Fprintf(os.Stderr, "campaign %d failed with status %d: %s\n", i+1, resp.StatusCode, string(body))
			continue
		}

		var created struct {
			ID    int64 `json:"id"`
			Total int   `json:"total"`
		}
		_ = json.Unmarshal(body, &created)
		fmt.Printf("Campaign #%d created successfully: id=%d, total=%d (took %v)\n", i+1, created.ID, created.Total, time.Since(start))
	}
}

func runHTTPCmd(args []string) {
	fs := flag.NewFlagSet("http", flag.ExitOnError)
	targetURL := fs.String("url", "http://localhost:8080/api/v1/campaigns", "Target URL to benchmark")
	rps := fs.Float64("rps", 300.0, "Target requests per second")
	duration := fs.Duration("duration", 2*time.Minute, "Benchmark duration (e.g. 30s, 2m)")
	workers := fs.Int("workers", 64, "Number of concurrent worker goroutines")
	method := fs.String("method", "GET", "HTTP method (GET, POST, etc.)")
	_ = fs.Parse(args)

	if *rps <= 0 {
		fmt.Fprintln(os.Stderr, "error: -rps must be positive")
		os.Exit(1)
	}
	if *workers <= 0 {
		fmt.Fprintln(os.Stderr, "error: -workers must be positive")
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	fmt.Printf("Starting open-loop HTTP loadgen against %s\n", *targetURL)
	fmt.Printf("Target RPS: %.1f, Duration: %v, Workers: %d, Method: %s\n", *rps, *duration, *workers, *method)

	summary := loadgen.NewSummary()
	jobs := make(chan struct{}, *workers*4)

	// Custom transport with pooled connections
	tr := &http.Transport{
		MaxIdleConns:        *workers * 4,
		MaxIdleConnsPerHost: *workers * 4,
		IdleConnTimeout:     90 * time.Second,
		DisableCompression:  true,
	}
	client := &http.Client{
		Transport: tr,
		Timeout:   10 * time.Second,
	}

	var wg sync.WaitGroup
	for i := 0; i < *workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range jobs {
				req, err := http.NewRequestWithContext(ctx, *method, *targetURL, nil)
				if err != nil {
					summary.Record(0, 0, err)
					continue
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

	// Open-loop ticker
	tickerInterval := time.Duration(float64(time.Second) / *rps)
	ticker := time.NewTicker(tickerInterval)
	defer ticker.Stop()

	testStart := time.Now()
	timer := time.NewTimer(*duration)
	defer timer.Stop()

producerLoop:
	for {
		select {
		case <-ctx.Done():
			fmt.Println("\nAborting early due to interrupt signal...")
			break producerLoop
		case <-timer.C:
			break producerLoop
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
	stats.PrintReport()
}
