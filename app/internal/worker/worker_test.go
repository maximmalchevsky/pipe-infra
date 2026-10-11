package worker_test

import (
	"context"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/maximmalchevsky/pipe-infra/app/internal/config"
	"github.com/maximmalchevsky/pipe-infra/app/internal/store"
	"github.com/maximmalchevsky/pipe-infra/app/internal/worker"
)

type syncStore struct {
	*store.FakeStore
	batchStarted chan struct{}
	releaseBatch chan struct{}
	claimCount   atomic.Int32
}

func (s *syncStore) ClaimAndProcess(
	ctx context.Context,
	batchSize int,
	processFn func(ctx context.Context, claims []store.MessageClaim) ([]store.MessageResult, error),
) (int, error) {
	return s.FakeStore.ClaimAndProcess(ctx, batchSize, func(procCtx context.Context, claims []store.MessageClaim) ([]store.MessageResult, error) {
		s.claimCount.Add(1)
		select {
		case s.batchStarted <- struct{}{}:
		default:
		}
		<-s.releaseBatch
		return processFn(procCtx, claims)
	})
}

func TestWorker_ProcessAndStop(t *testing.T) {
	fake := store.NewFakeStore()
	_, err := fake.CreateCampaign(context.Background(), "Worker test", 5)
	if err != nil {
		t.Fatalf("failed to create campaign: %v", err)
	}

	cfg := &config.WorkerConfig{
		BatchSize:     10,
		BatchTimeout:  5 * time.Second,
		SendLatencyMS: 0,
		SendFailRate:  0.0,
		MaxAttempts:   3,
		PollInterval:  10 * time.Millisecond,
	}

	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	w := worker.New(fake, cfg, nil, logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		_ = w.Run(ctx)
		close(done)
	}()

	// Wait until pending messages are processed
	deadline := time.Now().Add(2 * time.Second)
	for {
		count, err := fake.GetPendingCount(context.Background())
		if err != nil {
			t.Fatalf("get pending count: %v", err)
		}
		if count == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for pending messages to be processed, count=%d", count)
		}
		time.Sleep(5 * time.Millisecond)
	}

	w.Stop()

	select {
	case <-done:
		// Clean exit
	case <-time.After(1 * time.Second):
		t.Fatal("worker did not exit cleanly within timeout")
	}
}

func TestWorker_ShutdownCompletesInFlightBatch(t *testing.T) {
	fake := store.NewFakeStore()
	// Batch 1: 5 messages
	_, err := fake.CreateCampaign(context.Background(), "Batch 1", 5)
	if err != nil {
		t.Fatalf("failed to create campaign 1: %v", err)
	}
	// Batch 2: 5 messages
	_, err = fake.CreateCampaign(context.Background(), "Batch 2", 5)
	if err != nil {
		t.Fatalf("failed to create campaign 2: %v", err)
	}

	syncFake := &syncStore{
		FakeStore:    fake,
		batchStarted: make(chan struct{}),
		releaseBatch: make(chan struct{}),
	}

	cfg := &config.WorkerConfig{
		BatchSize:     5,
		BatchTimeout:  5 * time.Second,
		SendLatencyMS: 0,
		SendFailRate:  0.0,
		MaxAttempts:   3,
		PollInterval:  10 * time.Millisecond,
	}

	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	w := worker.New(syncFake, cfg, nil, logger)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		_ = w.Run(ctx)
		close(done)
	}()

	// Deterministically wait for batch 1 to be claimed and in-flight
	select {
	case <-syncFake.batchStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("batch 1 did not start processing in time")
	}

	// Cancel the parent context while batch 1 is in-flight
	cancel()

	// Release batch 1 to finish processing
	close(syncFake.releaseBatch)

	// Wait for Run to return
	select {
	case <-done:
		// Completed cleanly
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not exit within timeout")
	}

	// Verify batch 1 completed and committed (5 sent, 0 pending)
	c1, err := fake.GetCampaign(context.Background(), 1)
	if err != nil {
		t.Fatalf("get campaign 1: %v", err)
	}
	if c1.Pending != 0 || c1.Sent != 5 {
		t.Errorf("expected batch 1 to complete: pending=%d, sent=%d", c1.Pending, c1.Sent)
	}

	// Verify batch 2 was NOT claimed or processed after cancellation
	c2, err := fake.GetCampaign(context.Background(), 2)
	if err != nil {
		t.Fatalf("get campaign 2: %v", err)
	}
	if c2.Pending != 5 || c2.Sent != 0 {
		t.Errorf("expected batch 2 to remain untouched: pending=%d, sent=%d", c2.Pending, c2.Sent)
	}

	if claims := syncFake.claimCount.Load(); claims != 1 {
		t.Errorf("expected exactly 1 claim, got %d", claims)
	}
}
