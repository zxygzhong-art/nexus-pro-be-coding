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

// TestWithTenantTransactionReleasesConnectionOnSuccessErrorAndPanic 驗證租戶 transaction releases connection on success 錯誤 and panic。
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

// TestListTenantsInjectsSystemTaskScope 驗證租戶 injects system 任務範圍。
func TestListTenantsInjectsSystemTaskScope(t *testing.T) {
	pool := openPostgresIntegrationPool(t)
	defer pool.Close()
	ctx := context.Background()

	var bypass bool
	if err := pool.QueryRow(ctx, "select rolsuper or rolbypassrls from pg_roles where rolname = current_user").Scan(&bypass); err != nil {
		t.Fatal(err)
	}
	if bypass {
		t.Skip("current user bypasses RLS; system_task scope is exercised by non-BYPASSRLS integration runs")
	}
	var policyExists bool
	if err := pool.QueryRow(ctx, "select exists (select 1 from pg_policies where tablename = 'tenants' and policyname = 'system_read_tenants')").Scan(&policyExists); err != nil {
		t.Fatal(err)
	}
	if !policyExists {
		t.Skip("tenants system_read_tenants policy is not migrated; run current migrations")
	}

	tenantID := "tenant-system-task-" + time.Now().UTC().Format("20060102150405.000000000")
	// 先透過 tenant isolation policy 寫入，再清除 session 設定。
	// 如此可見性只能來自 system_task policy。
	if _, err := pool.Exec(ctx, "select set_config('app.tenant_id', $1, false)", tenantID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "insert into tenants (id, name, created_at) values ($1, $1, now())", tenantID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_, _ = pool.Exec(cleanupCtx, "select set_config('app.tenant_id', $1, false)", tenantID)
		_, _ = pool.Exec(cleanupCtx, "delete from tenants where id = $1", tenantID)
		_, _ = pool.Exec(cleanupCtx, "select set_config('app.tenant_id', '', false)")
	})
	if _, err := pool.Exec(ctx, "select set_config('app.tenant_id', '', false)"); err != nil {
		t.Fatal(err)
	}

	tenants, err := postgresrepo.NewStore(pool).ListTenants(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, tenant := range tenants {
		if tenant.ID == tenantID {
			return
		}
	}
	t.Fatalf("expected ListTenants to see %s via app.system_task under RLS, got %d tenants", tenantID, len(tenants))
}

// openPostgresIntegrationPool 驗證 open Postgres integration pool。
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

// requirePostgresConnectionAvailable 驗證 require Postgres connection available。
func requirePostgresConnectionAvailable(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := pool.Exec(ctx, "select 1"); err != nil {
		t.Fatalf("expected postgres connection to be available after transaction, got %v", err)
	}
}
