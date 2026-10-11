package store

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresStore implements Store using PostgreSQL and pgxpool.
type PostgresStore struct {
	pool   *pgxpool.Pool
	logger *slog.Logger
}

// NewPostgresStore creates a new PostgresStore connected to the given database URL.
func NewPostgresStore(ctx context.Context, dbURL string, logger *slog.Logger) (*PostgresStore, error) {
	cfg, err := pgxpool.ParseConfig(dbURL)
	if err != nil {
		return nil, fmt.Errorf("parse database URL: %w", err)
	}

	cfg.MaxConns = 25
	cfg.MinConns = 2
	cfg.MaxConnIdleTime = 5 * time.Minute
	cfg.MaxConnLifetime = 30 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create connection pool: %w", err)
	}

	return &PostgresStore{
		pool:   pool,
		logger: logger,
	}, nil
}

// Pool returns the underlying pgxpool.Pool.
func (s *PostgresStore) Pool() *pgxpool.Pool {
	return s.pool
}

// Ping checks database connectivity.
func (s *PostgresStore) Ping(ctx context.Context) error {
	return s.pool.Ping(ctx)
}

// Close closes the pool.
func (s *PostgresStore) Close() {
	s.pool.Close()
}

// CreateCampaign creates a campaign and bulk generates messages in one SQL statement.
func (s *PostgresStore) CreateCampaign(ctx context.Context, text string, recipients int) (*Campaign, error) {
	query := `
		WITH new_campaign AS (
			INSERT INTO campaigns (text, total)
			VALUES ($1, $2)
			RETURNING id, text, total, created_at
		),
		ins_messages AS (
			INSERT INTO messages (campaign_id, recipient, status, attempts)
			SELECT
				new_campaign.id,
				'+7900' || lpad(n::text, 7, '0'),
				'pending',
				0
			FROM new_campaign, generate_series(1, $2) AS n
		)
		SELECT id, text, total, created_at FROM new_campaign;
	`

	var c Campaign
	err := s.pool.QueryRow(ctx, query, text, recipients).Scan(&c.ID, &c.Text, &c.Total, &c.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert campaign and messages: %w", err)
	}
	return &c, nil
}

// GetCampaign returns details and counts for a single campaign.
func (s *PostgresStore) GetCampaign(ctx context.Context, id int64) (*CampaignDetail, error) {
	query := `
		SELECT c.id, c.text, c.total, c.created_at,
		       COALESCE(COUNT(m.id) FILTER (WHERE m.status = 'pending'), 0) AS pending,
		       COALESCE(COUNT(m.id) FILTER (WHERE m.status = 'sent'), 0) AS sent,
		       COALESCE(COUNT(m.id) FILTER (WHERE m.status = 'failed'), 0) AS failed
		FROM campaigns c
		LEFT JOIN messages m ON m.campaign_id = c.id
		WHERE c.id = $1
		GROUP BY c.id, c.text, c.total, c.created_at;
	`

	var cd CampaignDetail
	err := s.pool.QueryRow(ctx, query, id).Scan(
		&cd.ID, &cd.Text, &cd.Total, &cd.CreatedAt,
		&cd.Pending, &cd.Sent, &cd.Failed,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("query campaign: %w", err)
	}
	return &cd, nil
}

// ListCampaigns returns the most recent campaigns with message counts.
func (s *PostgresStore) ListCampaigns(ctx context.Context, limit int) ([]CampaignDetail, error) {
	if limit <= 0 {
		limit = 20
	}

	query := `
		WITH recent_campaigns AS (
			SELECT id, text, total, created_at
			FROM campaigns
			ORDER BY id DESC
			LIMIT $1
		)
		SELECT rc.id, rc.text, rc.total, rc.created_at,
		       COALESCE(COUNT(m.id) FILTER (WHERE m.status = 'pending'), 0) AS pending,
		       COALESCE(COUNT(m.id) FILTER (WHERE m.status = 'sent'), 0) AS sent,
		       COALESCE(COUNT(m.id) FILTER (WHERE m.status = 'failed'), 0) AS failed
		FROM recent_campaigns rc
		LEFT JOIN messages m ON m.campaign_id = rc.id
		GROUP BY rc.id, rc.text, rc.total, rc.created_at
		ORDER BY rc.id DESC;
	`

	rows, err := s.pool.Query(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("list campaigns: %w", err)
	}
	defer rows.Close()

	list := make([]CampaignDetail, 0)
	for rows.Next() {
		var cd CampaignDetail
		if err := rows.Scan(
			&cd.ID, &cd.Text, &cd.Total, &cd.CreatedAt,
			&cd.Pending, &cd.Sent, &cd.Failed,
		); err != nil {
			return nil, fmt.Errorf("scan campaign detail: %w", err)
		}
		list = append(list, cd)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate campaigns: %w", err)
	}
	return list, nil
}

// GetPendingCount counts total pending messages using the partial index.
func (s *PostgresStore) GetPendingCount(ctx context.Context) (int64, error) {
	var count int64
	err := s.pool.QueryRow(ctx, "SELECT COUNT(*) FROM messages WHERE status = 'pending'").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count pending messages: %w", err)
	}
	return count, nil
}

// WaitForTables polls until the messages table exists in the database.
func (s *PostgresStore) WaitForTables(ctx context.Context) error {
	backoff := 500 * time.Millisecond
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		var exists bool
		err := s.pool.QueryRow(ctx, "SELECT to_regclass('public.messages') IS NOT NULL").Scan(&exists)
		if err == nil && exists {
			s.logger.Info("database schema verified, ready for work")
			return nil
		}

		s.logger.Info("waiting for schema to be initialized by notifier-api...", "retry_in", backoff)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
			if backoff < 5*time.Second {
				backoff *= 2
			}
		}
	}
}

// ClaimAndProcess locks a batch of pending messages, executes processFn, and updates their status in a transaction.
func (s *PostgresStore) ClaimAndProcess(
	ctx context.Context,
	batchSize int,
	processFn func(ctx context.Context, claims []MessageClaim) ([]MessageResult, error),
) (int, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	rows, err := tx.Query(
		ctx,
		"SELECT id, attempts FROM messages WHERE status = 'pending' ORDER BY id LIMIT $1 FOR UPDATE SKIP LOCKED",
		batchSize,
	)
	if err != nil {
		return 0, fmt.Errorf("query pending messages: %w", err)
	}

	var claims []MessageClaim
	for rows.Next() {
		var c MessageClaim
		if err := rows.Scan(&c.ID, &c.Attempts); err != nil {
			rows.Close()
			return 0, fmt.Errorf("scan claim: %w", err)
		}
		claims = append(claims, c)
	}
	rows.Close()

	if len(claims) == 0 {
		return 0, nil
	}

	results, err := processFn(ctx, claims)
	if err != nil {
		return 0, fmt.Errorf("process messages: %w", err)
	}

	batch := &pgx.Batch{}
	for _, res := range results {
		switch res.Status {
		case "sent":
			batch.Queue(
				"UPDATE messages SET status = 'sent', sent_at = NOW(), attempts = $1, updated_at = NOW() WHERE id = $2",
				res.Attempts, res.ID,
			)
		case "failed":
			batch.Queue(
				"UPDATE messages SET status = 'failed', attempts = $1, updated_at = NOW() WHERE id = $2",
				res.Attempts, res.ID,
			)
		default:
			batch.Queue(
				"UPDATE messages SET status = 'pending', attempts = $1, updated_at = NOW() WHERE id = $2",
				res.Attempts, res.ID,
			)
		}
	}

	br := tx.SendBatch(ctx, batch)
	for range results {
		if _, err := br.Exec(); err != nil {
			_ = br.Close()
			return 0, fmt.Errorf("exec batch update: %w", err)
		}
	}
	if err := br.Close(); err != nil {
		return 0, fmt.Errorf("close batch: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit tx: %w", err)
	}

	return len(results), nil
}
