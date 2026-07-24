package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/repository"
	"nexus-pro-api/internal/service"
)

// TestPlatformFormsUsesEnabledTenantFormTemplates verifies form categories come from the tenant template store.
func TestPlatformFormsUsesEnabledTenantFormTemplates(t *testing.T) {
	store, svc, employeeCtx, _, _ := newAttendanceFixture(t)
	now := time.Date(2026, 7, 14, 9, 0, 0, 0, time.UTC)
	templates := []domain.FormTemplate{
		platformHomeTemplate("tenant-1", "ft-dynamic-leave", "dynamic-leave", "動態請假單", "人事考勤類", "🧪", "來自資料庫的請假入口", true, now),
		platformHomeTemplate("tenant-1", "ft-disabled", "disabled-form", "已停用表單", "人事考勤類", "🚫", "不應出現在首頁", false, now.Add(time.Minute)),
		platformHomeTemplate("tenant-1", "ft-dynamic-hr", "dynamic-hr", "動態人資單", "人資相關", "🧑‍💼", "來自資料庫的人資入口", true, now.Add(2*time.Minute)),
		platformHomeTemplate("tenant-1", "ft-dynamic-finance", "dynamic-finance", "動態財務單", "財會相關", "💹", "財會分類入口", true, now.Add(3*time.Minute)),
		platformHomeTemplate("tenant-2", "ft-foreign", "foreign-form", "其他租戶表單", "人事考勤類", "🔒", "不應跨租戶出現", true, now),
	}
	for _, template := range templates {
		if err := store.UpsertFormTemplate(context.Background(), template); err != nil {
			t.Fatal(err)
		}
	}

	forms, err := svc.Platform().Forms(employeeCtx, domain.PlatformFormsQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(forms.Categories) != 3 {
		t.Fatalf("expected three dynamic form categories, got %+v", forms.Categories)
	}
	if forms.Categories[0].Title != "人事考勤類" || len(forms.Categories[0].Items) != 1 {
		t.Fatalf("expected one enabled attendance form, got %+v", forms.Categories[0])
	}
	attendanceForm := forms.Categories[0].Items[0]
	if attendanceForm.ID != "dynamic-leave" || attendanceForm.Title != "動態請假單" || attendanceForm.Emoji != "🧪" || attendanceForm.Desc != "來自資料庫的請假入口" {
		t.Fatalf("expected database-backed attendance form metadata, got %+v", attendanceForm)
	}
	if forms.Categories[1].Title != "人資相關" || len(forms.Categories[1].Items) != 1 || forms.Categories[1].Items[0].ID != "dynamic-hr" {
		t.Fatalf("expected database-backed HR column, got %+v", forms.Categories[1])
	}
	if forms.Categories[2].Title != "財會相關" || len(forms.Categories[2].Items) != 1 || forms.Categories[2].Items[0].ID != "dynamic-finance" {
		t.Fatalf("expected database-backed finance column, got %+v", forms.Categories[2])
	}
}

// TestPlatformFormsDoesNotRestoreStaticForms verifies that disabling every stored template leaves categories empty.
func TestPlatformFormsDoesNotRestoreStaticForms(t *testing.T) {
	store, svc, employeeCtx, _, _ := newAttendanceFixture(t)
	now := time.Date(2026, 7, 14, 9, 0, 0, 0, time.UTC)
	if err := store.UpsertFormTemplate(context.Background(), platformHomeTemplate(
		"tenant-1", "ft-disabled", "disabled-form", "已停用表單", "人事考勤類", "🚫", "不應出現在首頁", false, now,
	)); err != nil {
		t.Fatal(err)
	}

	forms, err := svc.Platform().Forms(employeeCtx, domain.PlatformFormsQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if forms.Categories == nil {
		t.Fatal("expected empty form categories to serialize as an array, got nil")
	}
	if len(forms.Categories) != 0 {
		t.Fatalf("expected no form categories when every template is disabled, got %+v", forms.Categories)
	}
}

// TestPlatformTasksOmitsClockSummaryWithoutAttendanceRead verifies that task data remains available when the optional clock widget is denied.
func TestPlatformTasksOmitsClockSummaryWithoutAttendanceRead(t *testing.T) {
	store, svc, employeeCtx, _, _ := newAttendanceFixture(t)
	permissionSet, ok, err := store.GetPermissionSet(context.Background(), "tenant-1", "ps-attendance-self")
	if err != nil || !ok {
		t.Fatalf("load fixture permission set: ok=%v err=%v", ok, err)
	}
	permissionSet.Permissions = []domain.Permission{
		{Resource: "me", Action: domain.ActionRead, Scope: domain.ScopeSelf},
	}
	if err := store.UpsertPermissionSet(context.Background(), permissionSet); err != nil {
		t.Fatal(err)
	}

	tasks, err := svc.Platform().Tasks(employeeCtx, domain.PlatformTasksQuery{})
	if err != nil {
		t.Fatalf("expected accessible task subset, got %v", err)
	}
	if tasks.ClockSummary != nil {
		t.Fatalf("expected unauthorized clock summary to be omitted, got %+v", tasks.ClockSummary)
	}
	if tasks.Records == nil || tasks.Todos == nil || tasks.AIMessages == nil || tasks.QuickPrompts == nil {
		t.Fatalf("expected accessible task collections to remain present, got %+v", tasks)
	}
}

// TestPlatformTasksKeepsAuthorizedClockErrors verifies that only explicit denial is downgraded for the task aggregate.
func TestPlatformTasksKeepsAuthorizedClockErrors(t *testing.T) {
	store, _, employeeCtx, _, _ := newAttendanceFixture(t)
	expected := errors.New("clock projection unavailable")
	svc := service.New(&platformHomeClockFailureStore{Store: store, err: expected})

	_, err := svc.Platform().Tasks(employeeCtx, domain.PlatformTasksQuery{})
	if !errors.Is(err, expected) {
		t.Fatalf("expected clock projection error to propagate, got %v", err)
	}
}

type platformHomeClockFailureStore struct {
	repository.Store
	err error
}

func (s *platformHomeClockFailureStore) ListAttendanceClockRecords(
	context.Context,
	string,
	domain.AttendanceClockRecordQuery,
) ([]domain.AttendanceClockRecord, error) {
	return nil, s.err
}

// platformHomeTemplate builds persisted template metadata used by platform form category tests.
func platformHomeTemplate(tenantID, id, key, name, category, icon, desc string, enabled bool, createdAt time.Time) domain.FormTemplate {
	return domain.FormTemplate{
		ID:          id,
		TenantID:    tenantID,
		Key:         key,
		Name:        name,
		Description: desc,
		Schema: map[string]any{
			"type": "object",
			"workspace_design": map[string]any{
				"enabled":  enabled,
				"category": category,
				"icon":     icon,
				"desc":     desc,
			},
		},
		Status:    "published",
		CreatedAt: createdAt,
		UpdatedAt: createdAt,
	}
}
