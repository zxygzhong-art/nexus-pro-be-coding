package memory_test

import (
	"context"
	"testing"
	"time"

	"nexus-pro-be/internal/domain"
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
