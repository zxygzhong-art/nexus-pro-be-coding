package service_test

import (
	"context"
	"testing"
	"time"

	"nexus-pro-api/internal/domain"
)

// TestEmployeeDetailReturnsUnmaskedValuesWhileListRemainsMasked 驗證列表脫敏但詳情回傳授權後原值。
func TestEmployeeDetailReturnsUnmaskedValuesWhileListRemainsMasked(t *testing.T) {
	store, svc, ctx := newEmployeeFeatureFixture(t, []domain.Permission{{Resource: "hr.employee", Action: "read", Scope: "all"}})
	now := time.Date(2026, 7, 13, 10, 0, 0, 0, time.UTC)
	_ = store.UpsertFieldPolicy(context.Background(), domain.FieldPolicy{
		ID:              "fp-mask-company-email",
		TenantID:        "tenant-1",
		ApplicationCode: "hr",
		ResourceType:    "employee",
		FieldName:       "company_email",
		Effect:          "mask",
		CreatedAt:       now,
	})
	_ = store.UpsertEmployee(context.Background(), domain.Employee{
		ID:           "emp-1",
		TenantID:     "tenant-1",
		EmployeeNo:   "E001",
		Name:         "Employee One",
		CompanyEmail: "employee.one@example.com",
		Status:       "active",
		BasicInfo:    map[string]any{"nationality_type": "local", "national_id": "A123456789"},
		CreatedAt:    now,
		UpdatedAt:    now,
	})

	page, err := svc.HR().QueryEmployees(ctx, domain.EmployeeQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].CompanyEmail == "employee.one@example.com" || page.Items[0].BasicInfo["national_id"] != "***" {
		t.Fatalf("expected masked employee list, got %+v", page.Items)
	}

	detail, err := svc.HR().GetEmployee(ctx, "emp-1")
	if err != nil {
		t.Fatal(err)
	}
	if detail.CompanyEmail != "employee.one@example.com" || detail.BasicInfo["national_id"] != "A123456789" {
		t.Fatalf("expected unmasked employee detail, got %+v", detail)
	}
	logs, err := store.ListAuditLogs(context.Background(), "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	log, ok := findAuditLog(logs, "hr.employee.sensitive_field.read")
	fields, fieldsOK := log.Details["fields"].([]string)
	if !ok || !fieldsOK || !stringSliceContains(fields, "national_id") {
		t.Fatalf("expected sensitive detail read audit, got %+v", logs)
	}
}
