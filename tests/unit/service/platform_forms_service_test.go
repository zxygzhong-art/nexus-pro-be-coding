package service_test

import (
	"context"
	"testing"
	"time"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/repository/memory"
	"nexus-pro-api/internal/service"
)

// seedPlatformFormsFixtures 建立平臺表單分頁測試共用的租戶、帳號與範本。
func seedPlatformFormsFixtures(t *testing.T, now time.Time) (*service.Service, domain.RequestContext) {
	t.Helper()
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-workflow-self",
		TenantID: "tenant-1",
		Name:     "Workflow Self Service",
		Permissions: []domain.Permission{
			{Resource: "workflow.form_instance", Action: "read", Scope: "self"},
			{Resource: "workflow.form_instance", Action: "create", Scope: "self"},
			{Resource: "workflow.form_instance", Action: "submit", Scope: "self"},
			{Resource: "workflow.form_instance", Action: "update", Scope: "self"},
			{Resource: "workflow.form_instance", Action: "delete", Scope: "self"},
			{Resource: "platform.forms", Action: "read", Scope: "self"},
		},
		CreatedAt: now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{
		ID:                     "acct-self",
		TenantID:               "tenant-1",
		DisplayName:            "Self User",
		EmployeeID:             "emp-self",
		Status:                 "active",
		DirectPermissionSetIDs: []string{"ps-workflow-self"},
		CreatedAt:              now,
	})
	_ = store.UpsertEmployee(context.Background(), domain.Employee{
		ID:        "emp-self",
		TenantID:  "tenant-1",
		Name:      "Self User",
		AccountID: "acct-self",
		Status:    "active",
		CreatedAt: now,
	})
	_ = store.UpsertFormTemplate(context.Background(), domain.FormTemplate{
		ID:        "ft-leave",
		TenantID:  "tenant-1",
		Key:       "leave-request",
		Name:      "請假申請單",
		Schema:    workflowEnabledTemplateSchema(),
		CreatedAt: now,
	})
	_ = store.UpsertFormTemplate(context.Background(), domain.FormTemplate{
		ID:        "ft-expense",
		TenantID:  "tenant-1",
		Key:       "expense-claim",
		Name:      "報銷申請單",
		Schema:    workflowEnabledTemplateSchema(),
		CreatedAt: now,
	})
	instances := []domain.FormInstance{
		{ID: "fi-leave-1", TenantID: "tenant-1", TemplateID: "ft-leave", ApplicantAccountID: "acct-self", Status: "in_review", Payload: map[string]any{"reason": "家庭因素請假"}, SubmittedAt: now.Add(-3 * time.Hour), UpdatedAt: now.Add(-3 * time.Hour)},
		{ID: "fi-leave-2", TenantID: "tenant-1", TemplateID: "ft-leave", ApplicantAccountID: "acct-self", Status: "approved", Payload: map[string]any{"reason": "病假"}, SubmittedAt: now.Add(-2 * time.Hour), UpdatedAt: now.Add(-2 * time.Hour)},
		{ID: "fi-expense-1", TenantID: "tenant-1", TemplateID: "ft-expense", ApplicantAccountID: "acct-self", Status: "in_review", Payload: map[string]any{"reason": "差旅報銷"}, SubmittedAt: now.Add(-time.Hour), UpdatedAt: now.Add(-time.Hour)},
		{ID: "fi-draft-1", TenantID: "tenant-1", TemplateID: "ft-leave", ApplicantAccountID: "acct-self", Status: "draft", Payload: map[string]any{"reason": "草稿一"}, SubmittedAt: now.Add(-5 * time.Hour), UpdatedAt: now.Add(-30 * time.Minute)},
		{ID: "fi-draft-2", TenantID: "tenant-1", TemplateID: "ft-expense", ApplicantAccountID: "acct-self", Status: "draft", Payload: map[string]any{"reason": "草稿二"}, SubmittedAt: now.Add(-4 * time.Hour), UpdatedAt: now.Add(-10 * time.Minute)},
	}
	for _, instance := range instances {
		if err := store.UpsertFormInstance(context.Background(), instance); err != nil {
			t.Fatal(err)
		}
	}
	svc, _ := newServiceWithFakeFormApprovalWorkflows(store, service.Options{Now: func() time.Time { return now }})
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-self"}
	return svc, ctx
}

// TestPlatformFormsPaginationEnvelope 驗證 applications 與 drafts 回傳分頁信封且列表不含 payload。
func TestPlatformFormsPaginationEnvelope(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	svc, ctx := seedPlatformFormsFixtures(t, now)

	forms, err := svc.Platform().Forms(ctx, domain.PlatformFormsQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if forms.Applications.Total != 3 || forms.Applications.Page != 1 || forms.Applications.PageSize != domain.DefaultPageSize {
		t.Fatalf("unexpected applications envelope: %+v", forms.Applications)
	}
	if len(forms.Applications.Items) != 3 || forms.Drafts.Total != 2 || len(forms.Drafts.Items) != 2 {
		t.Fatalf("unexpected items: applications=%+v drafts=%+v", forms.Applications, forms.Drafts)
	}
	for _, item := range forms.Applications.Items {
		if item.Payload != nil {
			t.Fatalf("list item must not carry payload, got %+v", item)
		}
	}
	for _, item := range forms.Drafts.Items {
		if item.Payload != nil {
			t.Fatalf("draft list item must not carry payload, got %+v", item)
		}
	}
	if len(forms.Categories) == 0 || len(forms.AIMessages) == 0 || len(forms.QuickPrompts) == 0 {
		t.Fatal("expected metadata sections to stay populated")
	}

	paged, err := svc.Platform().Forms(ctx, domain.PlatformFormsQuery{Page: 2, PageSize: 2})
	if err != nil {
		t.Fatal(err)
	}
	if paged.Applications.Total != 3 || paged.Applications.Page != 2 || paged.Applications.PageSize != 2 || len(paged.Applications.Items) != 1 {
		t.Fatalf("unexpected applications page 2: %+v", paged.Applications)
	}
	if paged.Applications.Items[0].ID != "fi-leave-1" {
		t.Fatalf("expected oldest application on page 2, got %+v", paged.Applications.Items[0])
	}
	if paged.Drafts.Total != 2 || len(paged.Drafts.Items) != 0 {
		t.Fatalf("unexpected drafts page 2: %+v", paged.Drafts)
	}
}

// TestPlatformFormsFilters 驗證 status/template/search 過濾行為。
func TestPlatformFormsFilters(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	svc, ctx := seedPlatformFormsFixtures(t, now)

	byStatus, err := svc.Platform().Forms(ctx, domain.PlatformFormsQuery{Status: "approved"})
	if err != nil {
		t.Fatal(err)
	}
	if byStatus.Applications.Total != 1 || byStatus.Applications.Items[0].ID != "fi-leave-2" {
		t.Fatalf("expected only approved application, got %+v", byStatus.Applications)
	}
	if byStatus.Drafts.Total != 2 {
		t.Fatalf("status filter must not affect drafts, got %+v", byStatus.Drafts)
	}

	byTemplate, err := svc.Platform().Forms(ctx, domain.PlatformFormsQuery{Template: "expense-claim"})
	if err != nil {
		t.Fatal(err)
	}
	if byTemplate.Applications.Total != 1 || byTemplate.Applications.Items[0].ID != "fi-expense-1" {
		t.Fatalf("expected only expense application, got %+v", byTemplate.Applications)
	}
	if byTemplate.Drafts.Total != 1 || byTemplate.Drafts.Items[0].ID != "fi-draft-2" {
		t.Fatalf("expected only expense draft, got %+v", byTemplate.Drafts)
	}

	byTitle, err := svc.Platform().Forms(ctx, domain.PlatformFormsQuery{Search: "請假"})
	if err != nil {
		t.Fatal(err)
	}
	if byTitle.Applications.Total != 2 || byTitle.Drafts.Total != 1 {
		t.Fatalf("expected title search hits, got applications=%+v drafts=%+v", byTitle.Applications, byTitle.Drafts)
	}

	byPayload, err := svc.Platform().Forms(ctx, domain.PlatformFormsQuery{Search: "差旅"})
	if err != nil {
		t.Fatal(err)
	}
	if byPayload.Applications.Total != 1 || byPayload.Applications.Items[0].ID != "fi-expense-1" {
		t.Fatalf("expected payload search hit, got %+v", byPayload.Applications)
	}

	byApplicant, err := svc.Platform().Forms(ctx, domain.PlatformFormsQuery{Search: "self user"})
	if err != nil {
		t.Fatal(err)
	}
	if byApplicant.Applications.Total != 3 || byApplicant.Drafts.Total != 2 {
		t.Fatalf("expected applicant name search to match all, got applications=%+v drafts=%+v", byApplicant.Applications, byApplicant.Drafts)
	}
}

// TestPlatformFormsDetailKeepsFullPayload 驗證列表瘦身後詳情仍回傳完整 payload。
func TestPlatformFormsDetailKeepsFullPayload(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	svc, ctx := seedPlatformFormsFixtures(t, now)

	detail, err := svc.Workflow().GetFormInstanceDetail(ctx, "fi-leave-1")
	if err != nil {
		t.Fatal(err)
	}
	if detail.Payload["reason"] != "家庭因素請假" {
		t.Fatalf("expected detail to carry full payload, got %+v", detail.Payload)
	}
	if detail.TemplateKey != "leave-request" || detail.TemplateName != "請假申請單" {
		t.Fatalf("expected template metadata, got %+v", detail)
	}
}
