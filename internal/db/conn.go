// Package db provides PostgreSQL connection pool management and schema migration
// for Postbrain.
package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/simplyblock/postbrain/internal/config"
)

// NewPool creates and returns a pgx connection pool configured from cfg.
// AfterConnect sets search_path = public so queries always resolve to the
// correct schema (ag_catalog is omitted here; AGE support is optional and
// handled at the application layer when the extension is present).
func NewPool(ctx context.Context, cfg *config.DatabaseConfig) (*pgxpool.Pool, error) {
	if cfg.URL == "" {
		return nil, fmt.Errorf("db: database URL is empty")
	}

	poolConfig, err := pgxpool.ParseConfig(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("db: parse config: %w", err)
	}

	poolConfig.MaxConns = int32(cfg.MaxOpen) //nolint:gosec // value is bounded by config validation
	poolConfig.MinConns = int32(cfg.MaxIdle) //nolint:gosec // value is bounded by config validation
	poolConfig.ConnConfig.ConnectTimeout = cfg.ConnectTimeout

	poolConfig.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		_, err := conn.Exec(ctx, "SET search_path = public")
		if err != nil {
			return fmt.Errorf("db: set search_path: %w", err)
		}
		return nil
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("db: create pool: %w", err)
	}

	return pool, nil
}
