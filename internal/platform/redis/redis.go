package redis

import (
	"context"
	"fmt"

	redisotel "github.com/redis/go-redis/extra/redisotel/v9"
	goredis "github.com/redis/go-redis/v9"
)

type Options struct {
	Addr     string
	Password string
	DB       int
}

func OpenClient(ctx context.Context, options Options) (*goredis.Client, error) {
	if options.Addr == "" {
		return nil, fmt.Errorf("redis addr is required")
	}
	client := goredis.NewClient(&goredis.Options{
		Addr:     options.Addr,
		Password: options.Password,
		DB:       options.DB,
	})
	if err := redisotel.InstrumentTracing(client, redisotel.WithDBStatement(false)); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("instrument redis tracing: %w", err)
	}
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("ping redis: %w", err)
	}
	return client, nil
}
