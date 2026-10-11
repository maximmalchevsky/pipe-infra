package config

import (
	"os"
	"testing"
	"time"
)

func TestLoadAPIConfig(t *testing.T) {
	os.Clearenv()

	// Missing DATABASE_URL
	_, err := LoadAPIConfig("1.0.0")
	if err == nil {
		t.Fatal("expected error when DATABASE_URL is missing")
	}

	// Valid defaults
	_ = os.Setenv("DATABASE_URL", "postgres://localhost/test")
	cfg, err := LoadAPIConfig("1.0.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.HTTPAddr != ":8080" {
		t.Errorf("expected :8080, got %s", cfg.HTTPAddr)
	}
	if cfg.AppVersion != "1.0.0" {
		t.Errorf("expected 1.0.0, got %s", cfg.AppVersion)
	}
	if cfg.FaultErrorRate != 0.0 {
		t.Errorf("expected 0.0, got %f", cfg.FaultErrorRate)
	}
	if cfg.FaultLatencyMS != 0 {
		t.Errorf("expected 0, got %d", cfg.FaultLatencyMS)
	}

	// Overrides
	_ = os.Setenv("HTTP_ADDR", ":8888")
	_ = os.Setenv("APP_VERSION", "2.0.0")
	_ = os.Setenv("FAULT_ERROR_RATE", "0.25")
	_ = os.Setenv("FAULT_LATENCY_MS", "50")
	cfg, err = LoadAPIConfig("1.0.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.HTTPAddr != ":8888" || cfg.AppVersion != "2.0.0" || cfg.FaultErrorRate != 0.25 || cfg.FaultLatencyMS != 50 {
		t.Errorf("override mismatch: %+v", cfg)
	}

	// Invalid error rate
	_ = os.Setenv("FAULT_ERROR_RATE", "1.5")
	if _, err := LoadAPIConfig("1.0.0"); err == nil {
		t.Error("expected error for FAULT_ERROR_RATE > 1")
	}
}

func TestLoadWorkerConfig(t *testing.T) {
	os.Clearenv()

	_ = os.Setenv("DATABASE_URL", "postgres://localhost/test")
	cfg, err := LoadWorkerConfig("dev")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.HTTPAddr != ":9090" {
		t.Errorf("expected :9090, got %s", cfg.HTTPAddr)
	}
	if cfg.BatchSize != 100 {
		t.Errorf("expected 100, got %d", cfg.BatchSize)
	}
	if cfg.BatchTimeout != 30*time.Second {
		t.Errorf("expected 30s, got %v", cfg.BatchTimeout)
	}
	if cfg.SendLatencyMS != 20 {
		t.Errorf("expected 20, got %d", cfg.SendLatencyMS)
	}
	if cfg.SendFailRate != 0.01 {
		t.Errorf("expected 0.01, got %f", cfg.SendFailRate)
	}
	if cfg.MaxAttempts != 3 {
		t.Errorf("expected 3, got %d", cfg.MaxAttempts)
	}
	if cfg.PollInterval != 1*time.Second {
		t.Errorf("expected 1s, got %v", cfg.PollInterval)
	}

	_ = os.Setenv("POLL_INTERVAL", "500ms")
	_ = os.Setenv("BATCH_TIMEOUT", "15s")
	cfg, err = LoadWorkerConfig("dev")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.PollInterval != 500*time.Millisecond {
		t.Errorf("expected 500ms, got %v", cfg.PollInterval)
	}
	if cfg.BatchTimeout != 15*time.Second {
		t.Errorf("expected 15s, got %v", cfg.BatchTimeout)
	}
}
