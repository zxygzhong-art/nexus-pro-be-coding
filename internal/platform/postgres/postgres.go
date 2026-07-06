package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PoolOptions 定義 pool 選項的資料結構。
type PoolOptions struct {
	MaxConns        int
	MinConns        int
	MaxConnLifetime time.Duration
}

// OpenPool 處理 open pool。
func OpenPool(ctx context.Context, databaseURL string, options ...PoolOptions) (*pgxpool.Pool, error) {
	if databaseURL == "" {
		return nil, fmt.Errorf("database url is required")
	}

	opts := PoolOptions{}
	if len(options) > 0 {
		opts = options[0]
	}
	if opts.MaxConns <= 0 {
		opts.MaxConns = 10
	}
	if opts.MinConns <= 0 {
		opts.MinConns = 1
	}
	if opts.MaxConnLifetime <= 0 {
		opts.MaxConnLifetime = time.Hour
	}

	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}
	config.ConnConfig.Tracer = newQueryTracer()
	config.MaxConns = int32(opts.MaxConns)
	config.MinConns = int32(opts.MinConns)
	config.MaxConnLifetime = opts.MaxConnLifetime
	config.MaxConnIdleTime = 30 * time.Minute
	config.HealthCheckPeriod = time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("open postgres pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	return pool, nil
}
