//go:build integration

package store_test

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/maximmalchevsky/pipe-infra/app/internal/migrate"
	"github.com/maximmalchevsky/pipe-infra/app/internal/store"
)

func TestPostgresStore_Integration(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL is not set, skipping integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	s, err := store.NewPostgresStore(ctx, dbURL, logger)
	if err != nil {
		t.Fatalf("failed to connect to postgres: %v", err)
	}
	defer s.Close()

	if err := s.Ping(ctx); err != nil {
		t.Fatalf("failed to ping postgres: %v", err)
	}

	if err := migrate.Run(ctx, s.Pool(), logger); err != nil {
		t.Fatalf("failed to run migrations: %v", err)
	}

	campaign, err := s.CreateCampaign(ctx, "Integration test campaign", 10)
	if err != nil {
		t.Fatalf("failed to create campaign: %v", err)
	}
	if campaign.Total != 10 {
		t.Fatalf("expected total 10, got %d", campaign.Total)
	}

	detail, err := s.GetCampaign(ctx, campaign.ID)
	if err != nil {
		t.Fatalf("failed to get campaign detail: %v", err)
	}
	if detail.Pending != 10 {
		t.Fatalf("expected 10 pending, got %d", detail.Pending)
	}

	list, err := s.ListCampaigns(ctx, 5)
	if err != nil {
		t.Fatalf("failed to list campaigns: %v", err)
	}
	if len(list) == 0 {
		t.Fatal("expected at least one campaign in list")
	}
}
