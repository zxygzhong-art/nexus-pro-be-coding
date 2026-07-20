package postgres_integration_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/repository"
	postgresrepo "nexus-pro-api/internal/repository/postgres"
)

// TestLeaveTypeMappingAdvisoryLockSerializesOverlapValidation proves that two
// PostgreSQL sessions cannot validate the same normalized mapping key at once.
func TestLeaveTypeMappingAdvisoryLockSerializesOverlapValidation(t *testing.T) {
	firstPool := openIntegrationPool(t)
	defer firstPool.Close()
	secondPool := openIntegrationPool(t)
	defer secondPool.Close()
	requireLeaveMappingSchema(t, firstPool)

	firstStore := postgresrepo.NewStore(firstPool)
	secondStore := postgresrepo.NewStore(secondPool)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	now := time.Now().UTC()
	suffix := strings.ReplaceAll(strings.ToLower(t.Name()), "/", "_") + "_" + now.Format("150405000000")
	tenantID := "tenant_" + suffix
	if err := firstStore.UpsertTenant(ctx, domain.Tenant{ID: tenantID, Name: tenantID, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}

	firstMapping := domain.LeaveTypeExternalMapping{
		ID: "ltm_" + suffix + "_first", TenantID: tenantID, Source: "ehrms", ExternalCode: "Annual Leave",
		LeaveTypeID: "lt_annual", LeaveTypeCode: "annual", EffectiveFrom: "2026-01-01",
		CreatedAt: now, UpdatedAt: now,
	}
	overlapDetected := errors.New("overlapping mapping observed after advisory lock")
	firstLocked := make(chan struct{})
	releaseFirst := make(chan struct{})
	secondAttempting := make(chan struct{})
	secondAcquired := make(chan struct{})
	firstDone := make(chan error, 1)
	secondDone := make(chan error, 1)

	go func() {
		firstDone <- firstStore.WithTenantTransaction(ctx, tenantID, func(tx repository.Store) error {
			if err := tx.LockLeaveTypeExternalMappingKey(ctx, tenantID, " EHRMS ", " Annual Leave "); err != nil {
				return err
			}
			close(firstLocked)
			select {
			case <-releaseFirst:
			case <-ctx.Done():
				return ctx.Err()
			}
			return tx.UpsertLeaveTypeExternalMapping(ctx, firstMapping)
		})
	}()

	select {
	case <-firstLocked:
	case err := <-firstDone:
		t.Fatalf("first transaction ended before acquiring the lock: %v", err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}

	go func() {
		secondDone <- secondStore.WithTenantTransaction(ctx, tenantID, func(tx repository.Store) error {
			close(secondAttempting)
			if err := tx.LockLeaveTypeExternalMappingKey(ctx, tenantID, "ehrms", "annual leave"); err != nil {
				return err
			}
			close(secondAcquired)
			mappings, err := tx.ListLeaveTypeExternalMappings(ctx, tenantID)
			if err != nil {
				return err
			}
			if len(mappings) != 1 || mappings[0].ID != firstMapping.ID {
				return fmt.Errorf("expected the committed first mapping after lock wait, got %+v", mappings)
			}
			return overlapDetected
		})
	}()

	select {
	case <-secondAttempting:
	case err := <-secondDone:
		close(releaseFirst)
		firstErr := <-firstDone
		t.Fatalf("second transaction ended before attempting the lock: second=%v first=%v", err, firstErr)
	case <-ctx.Done():
		close(releaseFirst)
		firstErr := <-firstDone
		secondErr := <-secondDone
		t.Fatalf("timed out before the second lock attempt: context=%v first=%v second=%v", ctx.Err(), firstErr, secondErr)
	}
	acquiredBeforeCommit := false
	select {
	case <-secondAcquired:
		acquiredBeforeCommit = true
	case <-time.After(200 * time.Millisecond):
	}
	close(releaseFirst)

	firstErr := <-firstDone
	secondErr := <-secondDone
	if acquiredBeforeCommit {
		t.Fatalf("second connection acquired the normalized mapping lock before commit: first=%v second=%v", firstErr, secondErr)
	}
	if firstErr != nil {
		t.Fatalf("first mapping transaction failed: %v", firstErr)
	}
	if !errors.Is(secondErr, overlapDetected) {
		t.Fatalf("second transaction did not revalidate after waiting: %v", secondErr)
	}
	mappings, err := firstStore.ListLeaveTypeExternalMappings(ctx, tenantID)
	if err != nil {
		t.Fatal(err)
	}
	if len(mappings) != 1 || mappings[0].ID != firstMapping.ID {
		t.Fatalf("expected exactly one persisted mapping, got %+v", mappings)
	}
}

// requireLeaveMappingSchema skips older local databases that have not applied
// the leave mapping tables while keeping fresh CI databases strict.
func requireLeaveMappingSchema(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	requireMigratedSchema(t, pool)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var ready bool
	err := pool.QueryRow(ctx, `
		select to_regclass('public.leave_types') is not null
		  and to_regclass('public.leave_type_external_mappings') is not null
	`).Scan(&ready)
	if err != nil {
		t.Fatal(err)
	}
	if !ready {
		t.Skip("leave mapping schema is not migrated; skipping advisory-lock integration test")
	}
}
