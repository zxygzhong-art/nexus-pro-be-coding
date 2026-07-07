package service_test

import (
	"context"
	"testing"
	"time"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/service"
)

// TestHRPositionCRUDSoftDisablesPosition 驗證崗位 CRUD 與刪除策略。
func TestHRPositionCRUDSoftDisablesPosition(t *testing.T) {
	store, svc, ctx := newEmployeeFeatureFixture(t, hrPositionContractPermissions())
	ctx.ApprovalConfirmed = true

	created, err := svc.HR().CreatePosition(ctx, domain.CreatePositionInput{
		Code:        "eng",
		Name:        "Engineer",
		OrgUnitID:   "ou-1",
		Level:       "L3",
		Description: "Engineering role",
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.Status != string(domain.PositionStatusActive) || created.Code != "eng" {
		t.Fatalf("unexpected created position: %+v", created)
	}

	level := "L4"
	updated, err := svc.HR().UpdatePosition(ctx, created.ID, domain.UpdatePositionInput{Level: &level})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Level != "L4" {
		t.Fatalf("expected updated level L4, got %+v", updated)
	}

	page, err := svc.HR().ListPositionPage(ctx, domain.PageRequest{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || len(page.Items) != 1 || page.Items[0].ID != created.ID {
		t.Fatalf("unexpected position page: %+v", page)
	}

	disabled, err := svc.HR().DeletePosition(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if disabled.Status != string(domain.PositionStatusDisabled) {
		t.Fatalf("expected soft-disabled position, got %+v", disabled)
	}
	logs, err := store.ListAuditLogs(context.Background(), "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := findAuditLog(logs, "hr.position.delete"); !ok {
		t.Fatalf("expected hr.position.delete audit log, got %+v", logs)
	}
}

// TestBackfillEmployeePositionsFromStringsDeduplicatesByName 驗證既有 position 字串可回填為崗位實體。
func TestBackfillEmployeePositionsFromStringsDeduplicatesByName(t *testing.T) {
	store, svc, ctx := newEmployeeFeatureFixture(t, hrPositionContractPermissions())
	now := time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)
	for _, employee := range []domain.Employee{
		{ID: "emp-eng-1", TenantID: "tenant-1", Name: "Engineer One", Position: "Engineer", Status: "active", CreatedAt: now, UpdatedAt: now},
		{ID: "emp-eng-2", TenantID: "tenant-1", Name: "Engineer Two", Position: "Engineer", Status: "active", CreatedAt: now, UpdatedAt: now},
		{ID: "emp-design", TenantID: "tenant-1", Name: "Designer", Position: "Designer", Status: "active", CreatedAt: now, UpdatedAt: now},
		{ID: "emp-empty", TenantID: "tenant-1", Name: "Empty", Status: "active", CreatedAt: now, UpdatedAt: now},
	} {
		if err := store.UpsertEmployee(context.Background(), employee); err != nil {
			t.Fatal(err)
		}
	}

	updated, err := svc.HR().BackfillEmployeePositionsFromStrings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if updated != 3 {
		t.Fatalf("expected 3 employees to be backfilled, got %d", updated)
	}
	positions, err := store.ListPositions(context.Background(), "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(positions) != 2 {
		t.Fatalf("expected 2 deduplicated positions, got %+v", positions)
	}
	eng1, _, _ := store.GetEmployee(context.Background(), "tenant-1", "emp-eng-1")
	eng2, _, _ := store.GetEmployee(context.Background(), "tenant-1", "emp-eng-2")
	if eng1.PositionID == "" || eng1.PositionID != eng2.PositionID {
		t.Fatalf("expected employees with same position string to share position_id, got %q and %q", eng1.PositionID, eng2.PositionID)
	}
	empty, _, _ := store.GetEmployee(context.Background(), "tenant-1", "emp-empty")
	if empty.PositionID != "" {
		t.Fatalf("expected empty position to remain untouched, got %+v", empty)
	}
}

// TestEmploymentContractStatusTransitionsAndAudit 驗證合約狀態流轉與審計。
func TestEmploymentContractStatusTransitionsAndAudit(t *testing.T) {
	store, svc, ctx := newEmployeeFeatureFixture(t, hrPositionContractPermissions())
	ctx.ApprovalConfirmed = true
	now := time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)
	if err := store.UpsertEmployee(context.Background(), domain.Employee{
		ID:        "emp-contract",
		TenantID:  "tenant-1",
		Name:      "Contract Person",
		Status:    "active",
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	created, err := svc.HR().CreateEmploymentContract(ctx, "emp-contract", domain.CreateEmploymentContractInput{
		ContractType:        string(domain.EmploymentContractTypeFulltime),
		ContractNo:          "C-001",
		StartDate:           "2026-07-01",
		AttachmentObjectKey: "contracts/C-001.pdf",
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.Status != string(domain.EmploymentContractStatusDraft) || created.Version != 1 {
		t.Fatalf("unexpected created contract: %+v", created)
	}

	active := string(domain.EmploymentContractStatusActive)
	updated, err := svc.HR().UpdateEmploymentContract(ctx, created.ID, domain.UpdateEmploymentContractInput{Status: &active})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != active || updated.Version != 2 {
		t.Fatalf("expected active contract version 2, got %+v", updated)
	}

	draft := string(domain.EmploymentContractStatusDraft)
	if _, err := svc.HR().UpdateEmploymentContract(ctx, created.ID, domain.UpdateEmploymentContractInput{Status: &draft}); err == nil {
		t.Fatal("expected active -> draft transition to be rejected")
	} else if appErr, ok := domain.AsAppError(err); !ok || appErr.PublicCode != domain.ErrorCodeEmploymentContractInvalidTransition {
		t.Fatalf("expected invalid transition error code, got %v", err)
	}

	terminated, err := svc.HR().DeleteEmploymentContract(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if terminated.Status != string(domain.EmploymentContractStatusTerminated) || terminated.Version != 3 {
		t.Fatalf("expected terminated contract version 3, got %+v", terminated)
	}
	logs, err := store.ListAuditLogs(context.Background(), "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"hr.contract.create", "hr.contract.update", "hr.contract.delete"} {
		if _, ok := findAuditLog(logs, action); !ok {
			t.Fatalf("expected %s audit log, got %+v", action, logs)
		}
	}
}

// TestEmployeePositionStringCompatibilityDoubleWritesPositionEntity 驗證舊 position 字串請求仍可工作並雙寫 position_id。
func TestEmployeePositionStringCompatibilityDoubleWritesPositionEntity(t *testing.T) {
	store, svc, ctx := newEmployeeFeatureFixture(t, hrPositionContractPermissions(), service.Options{Now: func() time.Time {
		return time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)
	}})

	input := validEmployeeInput("E9001", "Compat Employee", "compat.employee@example.com")
	input.Position = "Legacy Engineer"
	input.EmploymentInfo["position"] = "Legacy Engineer"
	created, err := svc.HR().CreateEmployee(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if created.PositionID == "" || created.Position != "Legacy Engineer" {
		t.Fatalf("expected create to double-write position entity, got %+v", created)
	}
	if got := created.EmploymentInfo["position_id"]; got != created.PositionID {
		t.Fatalf("expected employment_info.position_id=%q, got %#v", created.PositionID, got)
	}
	position, ok, err := store.GetPosition(context.Background(), "tenant-1", created.PositionID)
	if err != nil || !ok {
		t.Fatalf("expected created position entity, position=%+v ok=%v err=%v", position, ok, err)
	}
	if position.Name != "Legacy Engineer" {
		t.Fatalf("expected position name synced from string, got %+v", position)
	}

	nextPosition := "Staff Engineer"
	updated, err := svc.HR().UpdateEmployee(ctx, created.ID, domain.UpdateEmployeeInput{Position: &nextPosition})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Position != nextPosition || updated.PositionID == "" || updated.PositionID == created.PositionID {
		t.Fatalf("expected update to double-write a new position entity, got %+v", updated)
	}
	if got := updated.EmploymentInfo["position_id"]; got != updated.PositionID {
		t.Fatalf("expected updated employment_info.position_id=%q, got %#v", updated.PositionID, got)
	}

	reused, err := svc.HR().UpdateEmployee(ctx, created.ID, domain.UpdateEmployeeInput{Position: &input.Position})
	if err != nil {
		t.Fatal(err)
	}
	if reused.PositionID != created.PositionID {
		t.Fatalf("expected update to reuse existing position %q, got %+v", created.PositionID, reused)
	}
}

func hrPositionContractPermissions() []domain.Permission {
	return []domain.Permission{
		{Resource: "hr.employee", Action: "read", Scope: "all"},
		{Resource: "hr.employee", Action: "create", Scope: "all"},
		{Resource: "hr.employee", Action: "update", Scope: "all"},
		{Resource: "hr.position", Action: "read", Scope: "all"},
		{Resource: "hr.position", Action: "create", Scope: "all"},
		{Resource: "hr.position", Action: "update", Scope: "all"},
		{Resource: "hr.position", Action: "delete", Scope: "all"},
		{Resource: "hr.employment_contract", Action: "read", Scope: "all"},
		{Resource: "hr.employment_contract", Action: "create", Scope: "all"},
		{Resource: "hr.employment_contract", Action: "update", Scope: "all"},
		{Resource: "hr.employment_contract", Action: "delete", Scope: "all"},
	}
}
