package migrate

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/maximmalchevsky/pipe-infra/app/migrations"
)

// advisoryLockID is an arbitrary 64-bit integer used for advisory locking.
const advisoryLockID = 84291041928371

// Run applies all pending embedded SQL migrations under an advisory lock.
func Run(ctx context.Context, pool *pgxpool.Pool, logger *slog.Logger) error {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire db connection for migration: %w", err)
	}
	defer conn.Release()

	logger.Info("acquiring pg_advisory_lock for migrations...")
	var dummy int
	if err := conn.QueryRow(ctx, "SELECT 1 FROM pg_advisory_lock($1)", advisoryLockID).Scan(&dummy); err != nil {
		return fmt.Errorf("acquire pg_advisory_lock: %w", err)
	}
	defer func() {
		if _, err := conn.Exec(ctx, "SELECT pg_advisory_unlock($1)", advisoryLockID); err != nil {
			logger.Error("failed to release advisory lock", "error", err)
		} else {
			logger.Info("released pg_advisory_lock")
		}
	}()

	// Ensure schema_migrations exists
	_, err = conn.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
	`)
	if err != nil {
		return fmt.Errorf("create schema_migrations table: %w", err)
	}

	// Check if migration 1 is applied
	var count int
	err = conn.QueryRow(ctx, "SELECT COUNT(*) FROM schema_migrations WHERE version = $1", 1).Scan(&count)
	if err != nil {
		return fmt.Errorf("check schema_migrations: %w", err)
	}
	if count > 0 {
		logger.Info("database schema already up to date", "version", 1)
		return nil
	}

	logger.Info("applying migration 001_init.sql...")
	content, err := migrations.FS.ReadFile("001_init.sql")
	if err != nil {
		return fmt.Errorf("read embedded 001_init.sql: %w", err)
	}

	tx, err := conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin migration tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, string(content)); err != nil {
		return fmt.Errorf("execute 001_init.sql: %w", err)
	}

	if _, err := tx.Exec(ctx, "INSERT INTO schema_migrations (version) VALUES ($1)", 1); err != nil {
		return fmt.Errorf("insert into schema_migrations: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit migration tx: %w", err)
	}

	logger.Info("migration 001_init.sql applied successfully", "version", 1)
	return nil
}
