package postgres_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"nexus-pro-be/internal/repository"
	postgresrepo "nexus-pro-be/internal/repository/postgres"
)

func TestWithTenantTransactionReleasesConnectionOnSuccessErrorAndPanic(t *testing.T) {
	pool := openPostgresIntegrationPool(t)
	defer pool.Close()
	store := postgresrepo.NewStore(pool)

	if err := store.WithTenantTransaction(context.Background(), "tenant-1", func(repository.Store) error {
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	requirePostgresConnectionAvailable(t, pool)

	sentinel := errors.New("sentinel transaction failure")
	if err := store.WithTenantTransaction(context.Background(), "tenant-1", func(repository.Store) error {
		return sentinel
	}); !errors.Is(err, sentinel) {
		t.Fatalf("expected sentinel error, got %v", err)
	}
	requirePostgresConnectionAvailable(t, pool)

	var recovered any
	func() {
		defer func() {
			recovered = recover()
		}()
		_ = store.WithTenantTransaction(context.Background(), "tenant-1", func(repository.Store) error {
			panic("panic transaction failure")
		})
	}()
	if recovered != "panic transaction failure" {
		t.Fatalf("expected original panic to be re-thrown, got %v", recovered)
	}
	requirePostgresConnectionAvailable(t, pool)
}

func openPostgresIntegrationPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL is not set; skipping postgres integration test")
	}
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	config.MaxConns = 1
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Fatal(err)
	}
	return pool
}

func requirePostgresConnectionAvailable(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := pool.Exec(ctx, "select 1"); err != nil {
		t.Fatalf("expected postgres connection to be available after transaction, got %v", err)
	}
}
