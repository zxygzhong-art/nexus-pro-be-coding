package service

import (
	"testing"
	"time"

	"nexus-pro-be/internal/domain"
)

// TestValidatePublishedFormFieldIdentityLocksIDAndType 驗證已發布欄位不可刪除或改型別。
func TestValidatePublishedFormFieldIdentityLocksIDAndType(t *testing.T) {
	previous := []domain.PlatformFormBuilderField{{ID: "amount", Type: "number", Label: "金額"}}

	if err := validatePublishedFormFieldIdentity(previous, nil); err == nil {
		t.Fatal("expected removing a published field to fail")
	}
	if err := validatePublishedFormFieldIdentity(previous, []domain.PlatformFormBuilderField{{ID: "amount", Type: "text"}}); err == nil {
		t.Fatal("expected changing a published field type to fail")
	}
	if err := validatePublishedFormFieldIdentity(previous, []domain.PlatformFormBuilderField{
		{ID: "amount", Type: "number", Label: "申請金額"},
		{ID: "reason", Type: "text", Label: "原因"},
	}); err != nil {
		t.Fatalf("expected label edits and additive fields to remain valid: %v", err)
	}
}

// TestProjectFormInstanceFieldValueUsesTypedColumns 驗證報表投影保留數值與時間型別。
func TestProjectFormInstanceFieldValueUsesTypedColumns(t *testing.T) {
	now := time.Date(2026, 7, 13, 6, 0, 0, 0, time.UTC)
	instance := domain.FormInstance{
		ID: "fi-1", TenantID: "tenant-1", TemplateID: "ft-1", TemplateVersionID: "ftv-1", UpdatedAt: now,
	}

	number, ok := projectFormInstanceFieldValue(instance, domain.PlatformFormBuilderField{ID: "amount", Type: "number"}, "1250.50")
	if !ok || number.ValueType != "number" || number.ValueNumber != "1250.50" {
		t.Fatalf("unexpected number projection: %+v ok=%v", number, ok)
	}
	timestamp, ok := projectFormInstanceFieldValue(instance, domain.PlatformFormBuilderField{ID: "occurred_at", Type: "datetime"}, "2026-07-13T14:00:00+08:00")
	if !ok || timestamp.ValueType != "timestamp" || timestamp.ValueTimestamp == "" {
		t.Fatalf("unexpected timestamp projection: %+v ok=%v", timestamp, ok)
	}
}

// TestValidateFormFieldAnalyticsRequiresCompatibleAggregation 驗證非數值欄位不能啟用 sum/avg。
func TestValidateFormFieldAnalyticsRequiresCompatibleAggregation(t *testing.T) {
	errors := validateFormFieldAnalyticsAndSecurity("department", domain.PlatformFormBuilderField{
		ID: "department", Type: "text",
		Analytics: &domain.PlatformFormBuilderFieldAnalytics{Reportable: true, Role: "dimension", Aggregations: []string{"sum"}},
	})
	if len(errors) != 1 || errors[0].Code != "invalid" {
		t.Fatalf("expected incompatible aggregation error, got %+v", errors)
	}
}
