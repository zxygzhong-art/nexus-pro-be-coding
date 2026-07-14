package service_test

import (
	"context"
	"testing"
	"time"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/repository/memory"
	"nexus-pro-be/internal/service"
)

func TestFormDefinitionDraftLifecycleCompilesAndPublishesOnlyAfterReview(t *testing.T) {
	now := time.Date(2026, 7, 13, 8, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	ctx := context.Background()
	if err := store.UpsertTenant(ctx, domain.Tenant{ID: "tenant-1", Name: "Tenant", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertPermissionSet(ctx, domain.PermissionSet{ID: "ps-form-builder", TenantID: "tenant-1", Name: "Form Builder", Permissions: []domain.Permission{
		{Resource: "workflow.form_definition_draft", Action: domain.ActionRead, Scope: domain.ScopeAll},
		{Resource: "workflow.form_definition_draft", Action: domain.ActionCreate, Scope: domain.ScopeAll},
		{Resource: "workflow.form_definition_draft", Action: domain.ActionUpdate, Scope: domain.ScopeAll},
		{Resource: "workflow.form_definition_draft", Action: domain.ActionSubmit, Scope: domain.ScopeAll},
		{Resource: "workflow.form_definition_draft", Action: domain.ActionApprove, Scope: domain.ScopeAll},
	}, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertAccount(ctx, domain.Account{ID: "acct-admin", TenantID: "tenant-1", Status: "active", DirectPermissionSetIDs: []string{"ps-form-builder"}, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	svc := service.New(store, service.Options{Now: func() time.Time { return now }})
	request := domain.CreateFormDefinitionDraftInput{AgentRunID: "run-1", ToolCallID: "call-1", Source: "agent", Schema: validDefinitionSchema()}
	draft, err := svc.Workflow().CreateFormDefinitionDraft(domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-admin"}, request)
	if err != nil {
		t.Fatal(err)
	}
	if !draft.ValidationResult.Valid || draft.Status != domain.FormDefinitionDraftStatusDraft {
		t.Fatalf("unexpected draft: %+v", draft)
	}
	retry, err := svc.Workflow().CreateFormDefinitionDraft(domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-admin"}, request)
	if err != nil || retry.ID != draft.ID {
		t.Fatalf("expected idempotent retry, draft=%+v err=%v", retry, err)
	}

	updatedSchema := validDefinitionSchema()
	updatedSchema.Description = "更新后的说明"
	if _, err := svc.Workflow().UpdateFormDefinitionDraft(domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-admin"}, draft.ID, domain.UpdateFormDefinitionDraftInput{Revision: draft.Revision + 1, Schema: updatedSchema}); err == nil {
		t.Fatal("expected stale revision conflict")
	}
	draft, err = svc.Workflow().SubmitFormDefinitionDraftForReview(domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-admin"}, draft.ID, draft.Revision)
	if err != nil || draft.Status != domain.FormDefinitionDraftStatusReviewPending {
		t.Fatalf("expected review_pending draft=%+v err=%v", draft, err)
	}
	draft, err = svc.Workflow().PublishFormDefinitionDraft(domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-admin"}, draft.ID, draft.Revision)
	if err != nil || draft.Status != domain.FormDefinitionDraftStatusPublished || draft.PublishedTemplateID == "" {
		t.Fatalf("expected published draft=%+v err=%v", draft, err)
	}
	template, ok, err := store.GetFormTemplate(ctx, "tenant-1", draft.PublishedTemplateID)
	if err != nil || !ok || template.Status != "published" {
		t.Fatalf("expected published runtime template=%+v ok=%v err=%v", template, ok, err)
	}
}

func validDefinitionSchema() domain.FormDefinitionSchemaV2 {
	return domain.FormDefinitionSchemaV2{SchemaVersion: 2, Name: "请假单", Fields: []domain.FormFieldDefinitionV2{{ID: "reason", Label: "事由", DataType: "string", Widget: "textarea"}}, Layout: domain.FormLayoutV2{Rows: []domain.FormLayoutRowV2{{FieldIDs: []string{"reason"}}}}, Workflow: domain.FormWorkflowV2{Stages: []domain.FormWorkflowStageV2{{ID: "manager", Type: "approver", Label: "直属主管", Config: map[string]any{"role": "manager"}}}}}
}
