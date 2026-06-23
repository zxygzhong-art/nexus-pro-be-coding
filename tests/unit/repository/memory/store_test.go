package memory_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/repository"
	"nexus-pro-be/internal/repository/memory"
)

func TestNextEmployeeNoIncrementsAcrossCalls(t *testing.T) {
	store := memory.NewStore()
	ctx := context.Background()
	if err := store.UpsertEmployee(ctx, domain.Employee{
		ID:         "emp-1",
		TenantID:   "tenant-1",
		EmployeeNo: "IKL002",
		CreatedAt:  time.Now(),
	}); err != nil {
		t.Fatal(err)
	}

	first, err := store.NextEmployeeNo(ctx, "tenant-1", "IKL")
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.NextEmployeeNo(ctx, "tenant-1", "IKL")
	if err != nil {
		t.Fatal(err)
	}

	if first != "IKL003" || second != "IKL004" {
		t.Fatalf("NextEmployeeNo() = %q then %q, want IKL003 then IKL004", first, second)
	}
}

func TestListEmployeePageByQueryMatchesMemoryFiltering(t *testing.T) {
	store := memory.NewStore()
	ctx := context.Background()
	now := time.Now()
	employees := []domain.Employee{
		{ID: "emp-1", TenantID: "tenant-1", EmployeeNo: "IKL001", Name: "One", Status: "active", CreatedAt: now},
		{ID: "emp-2", TenantID: "tenant-1", EmployeeNo: "IKL002", Name: "Two", Status: "active", CreatedAt: now.Add(time.Minute)},
		{ID: "emp-3", TenantID: "tenant-1", EmployeeNo: "IKL003", Name: "Deleted", Status: "deleted", CreatedAt: now.Add(2 * time.Minute)},
	}
	for _, employee := range employees {
		if err := store.UpsertEmployee(ctx, employee); err != nil {
			t.Fatal(err)
		}
	}

	items, total, err := store.ListEmployeePageByQuery(ctx, "tenant-1", domain.EmployeeQuery{
		Page:     1,
		PageSize: 1,
		Sort:     "created_at_desc",
	})
	if err != nil {
		t.Fatal(err)
	}

	if total != 2 {
		t.Fatalf("total = %d, want 2 active employees", total)
	}
	if len(items) != 1 || items[0].ID != "emp-2" {
		t.Fatalf("items = %#v, want newest active employee", items)
	}
}

func TestWithTenantTransactionCommitsAndRollsBack(t *testing.T) {
	store := memory.NewStore()
	ctx := context.Background()
	now := time.Now()

	err := store.WithTenantTransaction(ctx, "tenant-1", func(tx repository.Store) error {
		return tx.UpsertTenant(ctx, domain.Tenant{ID: "tenant-rollback", Name: "Rollback", CreatedAt: now})
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, err := store.GetTenant(ctx, "tenant-rollback"); err != nil || !ok {
		t.Fatalf("expected committed tenant, ok=%v err=%v", ok, err)
	}

	err = store.WithTenantTransaction(ctx, "tenant-1", func(tx repository.Store) error {
		if err := tx.UpsertTenant(ctx, domain.Tenant{ID: "tenant-error", Name: "Error", CreatedAt: now}); err != nil {
			return err
		}
		return errors.New("force rollback")
	})
	if err == nil {
		t.Fatal("expected transaction error")
	}
	if _, ok, err := store.GetTenant(ctx, "tenant-error"); err != nil || ok {
		t.Fatalf("expected error transaction to roll back, ok=%v err=%v", ok, err)
	}
}

func TestWithTenantTransactionRollsBackPanic(t *testing.T) {
	store := memory.NewStore()
	ctx := context.Background()
	now := time.Now()

	defer func() {
		if recovered := recover(); recovered == nil {
			t.Fatal("expected panic from transaction body")
		}
		if _, ok, err := store.GetTenant(ctx, "tenant-panic"); err != nil || ok {
			t.Fatalf("expected panic transaction to roll back, ok=%v err=%v", ok, err)
		}
	}()

	_ = store.WithTenantTransaction(ctx, "tenant-1", func(tx repository.Store) error {
		if err := tx.UpsertTenant(ctx, domain.Tenant{ID: "tenant-panic", Name: "Panic", CreatedAt: now}); err != nil {
			return err
		}
		panic("force rollback")
	})
}
