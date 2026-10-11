package store

import (
	"context"
	"errors"
	"time"
)

// ErrNotFound is returned when a requested resource does not exist.
var ErrNotFound = errors.New("resource not found")

// Campaign represents a created campaign.
type Campaign struct {
	ID        int64     `json:"id"`
	Text      string    `json:"text"`
	Total     int       `json:"total"`
	CreatedAt time.Time `json:"created_at"`
}

// CampaignDetail represents a campaign with detailed counts.
type CampaignDetail struct {
	ID        int64     `json:"id"`
	Text      string    `json:"text"`
	Total     int       `json:"total"`
	Pending   int       `json:"pending"`
	Sent      int       `json:"sent"`
	Failed    int       `json:"failed"`
	CreatedAt time.Time `json:"created_at"`
}

// MessageClaim represents a message claimed for sending.
type MessageClaim struct {
	ID       int64
	Attempts int
}

// MessageResult represents the outcome of processing a message.
type MessageResult struct {
	ID       int64
	Status   string // "sent", "failed", "pending"
	Attempts int
}

// Store defines operations for notifier data storage.
type Store interface {
	Ping(ctx context.Context) error
	CreateCampaign(ctx context.Context, text string, recipients int) (*Campaign, error)
	GetCampaign(ctx context.Context, id int64) (*CampaignDetail, error)
	ListCampaigns(ctx context.Context, limit int) ([]CampaignDetail, error)
	GetPendingCount(ctx context.Context) (int64, error)

	WaitForTables(ctx context.Context) error
	ClaimAndProcess(
		ctx context.Context,
		batchSize int,
		processFn func(ctx context.Context, claims []MessageClaim) ([]MessageResult, error),
	) (int, error)
	Close()
}
