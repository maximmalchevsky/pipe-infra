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
	fmt.Println("             loadgen campaign -url http://api:8080 -recipients 50000 -text \"promo\" [-count 1] [-host string] [-insecure]")
	fmt.Println()
	fmt.Println("  http       Generate HTTP traffic (open-loop) against an endpoint")
	fmt.Println("             loadgen http -url http://api:8080/api/v1/campaigns -rps 300 -duration 2m [-workers 64] [-method GET] [-host string] [-insecure]")
}

func runCampaignCmd(args []string) {
	fs := flag.NewFlagSet("campaign", flag.ExitOnError)
	targetURL := fs.String("url", "http://localhost:8080", "Target base URL (e.g. http://localhost:8080)")
	recipients := fs.Int("recipients", 50000, "Number of recipients per campaign")
	text := fs.String("text", "Hackathon campaign promo", "SMS text (1..160 chars)")
	count := fs.Int("count", 1, "Number of campaigns to create")
	host := fs.String("host", "", "HTTP Host header (req.Host) and TLS SNI server name")
	insecure := fs.Bool("insecure", false, "Skip TLS certificate verification (lab-only, in-cluster synthetic traffic with a private CA)")
	_ = fs.Parse(args)

	apiEndpoint := strings.TrimRight(*targetURL, "/")
	if !strings.HasSuffix(apiEndpoint, "/api/v1/campaigns") {
		apiEndpoint += "/api/v1/campaigns"
	}

	client := loadgen.NewClient(loadgen.ClientConfig{
		Host:     *host,
		Insecure: *insecure,
		Workers:  1,
	})

	for i := 0; i < *count; i++ {
		payload, _ := json.Marshal(map[string]any{
			"text":       *text,
			"recipients": *recipients,
		})

		start := time.Now()
		req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, apiEndpoint, bytes.NewReader(payload))
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to create request %d: %v\n", i+1, err)
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		if *host != "" {
			req.Host = *host
		}

		resp, err := client.Do(req)
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
	duration := fs.Duration("duration", 2*time.Minute, "Benchmark duration (e.g. 30s, 2m, 0 to run until SIGTERM/SIGINT)")
	workers := fs.Int("workers", 64, "Number of concurrent worker goroutines")
	method := fs.String("method", "GET", "HTTP method (GET, POST, etc.)")
	host := fs.String("host", "", "HTTP Host header (req.Host) and TLS SNI server name")
	insecure := fs.Bool("insecure", false, "Skip TLS certificate verification (lab-only, in-cluster synthetic traffic with a private CA)")
	_ = fs.Parse(args)

	if *rps <= 0 {
		fmt.Fprintln(os.Stderr, "error: -rps must be positive")
		os.Exit(1)
	}
	if *workers <= 0 {
		fmt.Fprintln(os.Stderr, "error: -workers must be positive")
		os.Exit(1)
	}
	if *duration < 0 {
		fmt.Fprintln(os.Stderr, "error: -duration must be non-negative (0 runs until SIGTERM/SIGINT)")
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	fmt.Printf("Starting open-loop HTTP loadgen against %s\n", *targetURL)
	if *duration == 0 {
		fmt.Printf("Target RPS: %.1f, Duration: unlimited (until SIGTERM/SIGINT), Workers: %d, Method: %s\n", *rps, *workers, *method)
	} else {
		fmt.Printf("Target RPS: %.1f, Duration: %v, Workers: %d, Method: %s\n", *rps, *duration, *workers, *method)
	}
	if *host != "" {
		fmt.Printf("Host override: %s (SNI and Host header)\n", *host)
	}
	if *insecure {
		fmt.Println("TLS insecure: InsecureSkipVerify=true (lab-only)")
	}

	cfg := loadgen.HTTPConfig{
		TargetURL:      *targetURL,
		RPS:            *rps,
		Duration:       *duration,
		Workers:        *workers,
		Method:         *method,
		Host:           *host,
		Insecure:       *insecure,
		ProgressWriter: os.Stdout,
	}

	stats, err := loadgen.RunHTTP(ctx, cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error running loadgen: %v\n", err)
		os.Exit(1)
	}

	if ctx.Err() != nil {
		fmt.Println("\nReceived interrupt signal, stopping...")
	}
	stats.PrintReport()
}
