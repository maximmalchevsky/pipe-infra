// Package config manages environment configuration parsing for the API and worker services.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// APIConfig holds configuration for the notifier-api service.
type APIConfig struct {
	DatabaseURL    string
	HTTPAddr       string
	AppVersion     string
	FaultErrorRate float64
	FaultLatencyMS int
}

// WorkerConfig holds configuration for the notifier-worker service.
type WorkerConfig struct {
	DatabaseURL   string
	HTTPAddr      string
	AppVersion    string
	BatchSize     int
	BatchTimeout  time.Duration
	SendLatencyMS int
	SendFailRate  float64
	MaxAttempts   int
	PollInterval  time.Duration
}

func getEnv(key, defaultVal string) string {
	if val, ok := os.LookupEnv(key); ok && val != "" {
		return val
	}
	return defaultVal
}

// LoadAPIConfig loads API configuration from environment variables.
func LoadAPIConfig(buildVersion string) (*APIConfig, error) {
	dbURL := getEnv("DATABASE_URL", "")
	if dbURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}

	httpAddr := getEnv("HTTP_ADDR", ":8080")

	appVersion := getEnv("APP_VERSION", "")
	if appVersion == "" {
		if buildVersion != "" {
			appVersion = buildVersion
		} else {
			appVersion = "dev"
		}
	}

	faultErrorRate := 0.0
	if raw := getEnv("FAULT_ERROR_RATE", "0"); raw != "" {
		val, err := strconv.ParseFloat(raw, 64)
		if err != nil || val < 0.0 || val > 1.0 {
			return nil, fmt.Errorf("FAULT_ERROR_RATE must be a float between 0 and 1, got %q", raw)
		}
		faultErrorRate = val
	}

	faultLatencyMS := 0
	if raw := getEnv("FAULT_LATENCY_MS", "0"); raw != "" {
		val, err := strconv.Atoi(raw)
		if err != nil || val < 0 {
			return nil, fmt.Errorf("FAULT_LATENCY_MS must be a non-negative integer, got %q", raw)
		}
		faultLatencyMS = val
	}

	return &APIConfig{
		DatabaseURL:    dbURL,
		HTTPAddr:       httpAddr,
		AppVersion:     appVersion,
		FaultErrorRate: faultErrorRate,
		FaultLatencyMS: faultLatencyMS,
	}, nil
}

// LoadWorkerConfig loads worker configuration from environment variables.
func LoadWorkerConfig(buildVersion string) (*WorkerConfig, error) {
	dbURL := getEnv("DATABASE_URL", "")
	if dbURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}

	httpAddr := getEnv("HTTP_ADDR", ":9090")

	appVersion := getEnv("APP_VERSION", "")
	if appVersion == "" {
		if buildVersion != "" {
			appVersion = buildVersion
		} else {
			appVersion = "dev"
		}
	}

	batchSize := 100
	if raw := getEnv("BATCH_SIZE", "100"); raw != "" {
		val, err := strconv.Atoi(raw)
		if err != nil || val <= 0 {
			return nil, fmt.Errorf("BATCH_SIZE must be a positive integer, got %q", raw)
		}
		batchSize = val
	}

	batchTimeout := 30 * time.Second
	if raw := getEnv("BATCH_TIMEOUT", "30s"); raw != "" {
		dur, err := time.ParseDuration(raw)
		if err != nil || dur <= 0 {
			return nil, fmt.Errorf("BATCH_TIMEOUT must be a positive duration (e.g. 30s), got %q", raw)
		}
		batchTimeout = dur
	}

	sendLatencyMS := 20
	if raw := getEnv("SEND_LATENCY_MS", "20"); raw != "" {
		val, err := strconv.Atoi(raw)
		if err != nil || val < 0 {
			return nil, fmt.Errorf("SEND_LATENCY_MS must be a non-negative integer, got %q", raw)
		}
		sendLatencyMS = val
	}

	sendFailRate := 0.01
	if raw := getEnv("SEND_FAIL_RATE", "0.01"); raw != "" {
		val, err := strconv.ParseFloat(raw, 64)
		if err != nil || val < 0.0 || val > 1.0 {
			return nil, fmt.Errorf("SEND_FAIL_RATE must be a float between 0 and 1, got %q", raw)
		}
		sendFailRate = val
	}

	maxAttempts := 3
	if raw := getEnv("MAX_ATTEMPTS", "3"); raw != "" {
		val, err := strconv.Atoi(raw)
		if err != nil || val <= 0 {
			return nil, fmt.Errorf("MAX_ATTEMPTS must be a positive integer, got %q", raw)
		}
		maxAttempts = val
	}

	pollInterval := 1 * time.Second
	if raw := getEnv("POLL_INTERVAL", "1s"); raw != "" {
		dur, err := time.ParseDuration(raw)
		if err != nil || dur <= 0 {
			return nil, fmt.Errorf("POLL_INTERVAL must be a positive duration (e.g. 1s), got %q", raw)
		}
		pollInterval = dur
	}

	return &WorkerConfig{
		DatabaseURL:   dbURL,
		HTTPAddr:      httpAddr,
		AppVersion:    appVersion,
		BatchSize:     batchSize,
		BatchTimeout:  batchTimeout,
		SendLatencyMS: sendLatencyMS,
		SendFailRate:  sendFailRate,
		MaxAttempts:   maxAttempts,
		PollInterval:  pollInterval,
	}, nil
}
