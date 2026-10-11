// Package store defines the data access layer and fake implementation for testing.
package store

import (
	"context"
	"errors"
	"sync"
	"time"
)

// FakeStore is an in-memory implementation of Store for tests.
type FakeStore struct {
	mu          sync.RWMutex
	campaigns   map[int64]*CampaignDetail
	nextID      int64
	pingErr     error
	tablesReady bool
}

// NewFakeStore creates an initialized FakeStore.
func NewFakeStore() *FakeStore {
	return &FakeStore{
		campaigns:   make(map[int64]*CampaignDetail),
		nextID:      1,
		tablesReady: true,
	}
}

// SetPingErr configures an error to return from Ping.
func (f *FakeStore) SetPingErr(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pingErr = err
}

// Ping checks if the fake store is healthy.
func (f *FakeStore) Ping(_ context.Context) error {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.pingErr
}

// Close closes the fake store resources.
func (f *FakeStore) Close() {}

// CreateCampaign adds a campaign and its pending messages to the fake store.
func (f *FakeStore) CreateCampaign(_ context.Context, text string, recipients int) (*Campaign, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	id := f.nextID
	f.nextID++

	now := time.Now()
	f.campaigns[id] = &CampaignDetail{
		ID:        id,
		Text:      text,
		Total:     recipients,
		Pending:   recipients,
		Sent:      0,
		Failed:    0,
		CreatedAt: now,
	}

	return &Campaign{
		ID:        id,
		Text:      text,
		Total:     recipients,
		CreatedAt: now,
	}, nil
}

// GetCampaign retrieves campaign details by ID from the fake store.
func (f *FakeStore) GetCampaign(_ context.Context, id int64) (*CampaignDetail, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	c, ok := f.campaigns[id]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *c
	return &cp, nil
}

// ListCampaigns returns the most recent campaigns up to limit from the fake store.
func (f *FakeStore) ListCampaigns(_ context.Context, limit int) ([]CampaignDetail, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	if limit <= 0 {
		limit = 20
	}

	result := make([]CampaignDetail, 0, len(f.campaigns))
	for id := f.nextID - 1; id >= 1 && len(result) < limit; id-- {
		if c, ok := f.campaigns[id]; ok {
			result = append(result, *c)
		}
	}
	return result, nil
}

// GetPendingCount returns the total number of pending messages across all campaigns in the fake store.
func (f *FakeStore) GetPendingCount(_ context.Context) (int64, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	var total int64
	for _, c := range f.campaigns {
		total += int64(c.Pending)
	}
	return total, nil
}

// WaitForTables checks if tables are ready in the fake store.
func (f *FakeStore) WaitForTables(_ context.Context) error {
	f.mu.RLock()
	defer f.mu.RUnlock()
	if !f.tablesReady {
		return errors.New("tables not ready")
	}
	return nil
}

// ClaimAndProcess claims a batch of messages and executes processFn.
func (f *FakeStore) ClaimAndProcess(
	ctx context.Context,
	batchSize int,
	processFn func(ctx context.Context, claims []MessageClaim) ([]MessageResult, error),
) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	var claims []MessageClaim
	for id := int64(1); id < f.nextID; id++ {
		c, ok := f.campaigns[id]
		if !ok || c.Pending <= 0 {
			continue
		}
		toClaim := batchSize - len(claims)
		if toClaim > c.Pending {
			toClaim = c.Pending
		}
		for i := 0; i < toClaim; i++ {
			claims = append(claims, MessageClaim{ID: int64(len(claims) + 1), Attempts: 0})
		}
		if len(claims) >= batchSize {
			break
		}
	}

	if len(claims) == 0 {
		return 0, nil
	}

	results, err := processFn(ctx, claims)
	if err != nil {
		return 0, err
	}

	for _, res := range results {
		for id := int64(1); id < f.nextID; id++ {
			c, ok := f.campaigns[id]
			if !ok || c.Pending <= 0 {
				continue
			}
			c.Pending--
			switch res.Status {
			case "sent":
				c.Sent++
			case "failed":
				c.Failed++
			default:
				c.Pending++
			}
			break
		}
	}

	return len(results), nil
}
