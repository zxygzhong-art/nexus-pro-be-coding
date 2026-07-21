package service_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/repository/memory"
	"nexus-pro-api/internal/service"
)

// TestFormDataSourcesUsesEnabledSystemLeaveTypes verifies the fixed local catalog
// is the form whitelist and tenant-disabled items are excluded.
func TestFormDataSourcesUsesEnabledSystemLeaveTypes(t *testing.T) {
	now := time.Date(2026, 7, 21, 9, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	if err := store.UpsertTenant(t.Context(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertPermissionSet(t.Context(), domain.PermissionSet{
		ID: "ps-workflow-read", TenantID: "tenant-1", Name: "Workflow Read",
		Permissions: []domain.Permission{{Resource: "workflow.form_instance", Action: "read", Scope: "self"}}, CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertAccount(t.Context(), domain.Account{
		ID: "acct-1", TenantID: "tenant-1", EmployeeID: "emp-1", Status: "active",
		DirectPermissionSetIDs: []string{"ps-workflow-read"}, CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertEmployee(t.Context(), domain.Employee{ID: "emp-1", TenantID: "tenant-1", AccountID: "acct-1", Name: "Employee", Status: "active", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertAttendancePolicy(context.Background(), domain.AttendancePolicy{
		ID: "current", TenantID: "tenant-1", LeaveTypes: []domain.AttendanceLeaveType{{Code: "policy_only", Name: "Policy Only", Active: true}}, Version: 3,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertLeaveTypeEnabled(t.Context(), "tenant-1", "business_trip", false, "acct-1", now); err != nil {
		t.Fatal(err)
	}

	catalog, err := service.New(store).Workflow().FormDataSources(domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"})
	if err != nil {
		t.Fatal(err)
	}
	var records []map[string]interface{}
	for _, source := range catalog.DataSources {
		if source.ID == "leave_types" {
			records = source.Records
		}
	}
	if len(records) != 14 {
		t.Fatalf("expected 14 enabled system leave types, got %+v", records)
	}
	byCode := map[string]map[string]interface{}{}
	for _, record := range records {
		byCode[fmt.Sprint(record["code"])] = record
	}
	if byCode["sick_full"]["name"] != "全薪病假" || byCode["sick_full"]["unit"] != "hour" {
		t.Fatalf("expected local bilingual definition projection, got %+v", byCode["sick_full"])
	}
	if _, ok := byCode["policy_only"]; ok {
		t.Fatalf("legacy policy JSON must stay out of form options: %+v", records)
	}
	if _, ok := byCode["business_trip"]; ok {
		t.Fatalf("tenant-disabled system item must stay out of form options: %+v", records)
	}
}

// TestValidateFormFieldBindingRejectsUnsupportedSources verifies persisted bindings are allowlisted.
func TestValidateFormFieldBindingRejectsUnsupportedSources(t *testing.T) {
	errors := service.ValidateFormFieldBinding("employee", "select", domain.PlatformFormBuilderFieldBinding{
		SourceID: "arbitrary_url", ValueField: "id", LabelField: "name",
	})
	if len(errors) != 1 || errors[0].Code != "invalid" {
		t.Fatalf("expected unsupported source validation error, got %+v", errors)
	}
}

// TestValidateAndResolveBoundSubmissionValue ensures object values are server-owned and collection values are allowlisted.
func TestValidateAndResolveBoundSubmissionValue(t *testing.T) {
	catalog := domain.FormDataSourceCatalogResponse{DataSources: []domain.FormDataSource{
		{ID: "current_user", Kind: "object", Records: []map[string]interface{}{{"employee_no": "E-001"}}},
		{ID: "departments", Kind: "collection", Records: []map[string]interface{}{{"id": "dept-1", "name": "產品部"}}},
	}}

	resolved, exists, message := service.ValidateAndResolveBoundSubmissionValue(catalog, domain.PlatformFormBuilderFieldBinding{
		SourceID: "current_user", ValueField: "employee_no",
	}, "tampered", true)
	if message != "" || !exists || resolved != "E-001" {
		t.Fatalf("expected server value to replace client value, got value=%v exists=%v message=%q", resolved, exists, message)
	}

	_, _, message = service.ValidateAndResolveBoundSubmissionValue(catalog, domain.PlatformFormBuilderFieldBinding{
		SourceID: "departments", ValueField: "id", LabelField: "name",
	}, "dept-unknown", true)
	if message == "" {
		t.Fatal("expected unknown collection option to be rejected")
	}
}
