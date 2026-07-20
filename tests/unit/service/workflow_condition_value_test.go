package service_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/repository/memory"
	"nexus-pro-api/internal/service"
)

func conditionDesignFixture(t *testing.T) (*service.Service, domain.RequestContext) {
	t.Helper()
	now := time.Date(2026, 7, 16, 9, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID: "ps-designer", TenantID: "tenant-1", Name: "Form Designer", CreatedAt: now,
		Permissions: []domain.Permission{
			{Resource: "workflow.form_template", Action: "create", Scope: "all"},
			{Resource: "workflow.form_template", Action: "read", Scope: "all"},
		},
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{ID: "acct-1", TenantID: "tenant-1", Status: "active", DirectPermissionSetIDs: []string{"ps-designer"}, CreatedAt: now})
	return service.New(store, service.Options{Now: func() time.Time { return now }}), domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"}
}

func conditionDesignInput(value string) domain.SaveWorkspaceFormDesignInput {
	return domain.SaveWorkspaceFormDesignInput{
		Name: "金額審批",
		Fields: []domain.PlatformFormBuilderField{
			{ID: "amount", Type: "number", Label: "金額"},
		},
		Stages: []domain.PlatformFormBuilderStage{
			{ID: "stage-condition", Type: "condition", Label: "金額條件", Config: map[string]any{
				"field": "amount", "operator": ">", "value": value,
			}},
			{ID: "stage-approver", Type: "approver", Label: "主管", Config: map[string]any{
				"role": "manager",
			}},
		},
	}
}

// TestFormDesignConditionRejectsNonNumericValue 驗證數值型條件 value 必須可解析為數字。
func TestFormDesignConditionRejectsNonNumericValue(t *testing.T) {
	svc, ctx := conditionDesignFixture(t)

	_, err := svc.Workspace().CreateWorkspaceFormDesign(ctx, conditionDesignInput("not-a-number"))
	appErr, ok := domain.AsAppError(err)
	if !ok || appErr.Code != "validation_failed" {
		t.Fatalf("expected validation_failed for non-numeric condition value, got %v", err)
	}
	found := false
	for _, fieldErr := range appErr.FieldErrors {
		if strings.HasSuffix(fieldErr.Field, ".config.value") && fieldErr.Code == "invalid" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected field error on config.value, got %+v", appErr.FieldErrors)
	}
}

// TestFormDesignConditionAcceptsNumericAndLevelValues 驗證數值與 level 條件可通過設計校驗。
func TestFormDesignConditionAcceptsNumericAndLevelValues(t *testing.T) {
	svc, ctx := conditionDesignFixture(t)

	if _, err := svc.Workspace().CreateWorkspaceFormDesign(ctx, conditionDesignInput("100")); err != nil {
		t.Fatalf("expected numeric condition value to pass design validation, got %v", err)
	}

	levelInput := conditionDesignInput("manager")
	levelInput.ID = "level-approval"
	levelInput.Name = "職級審批"
	levelInput.Stages[0].Config["field"] = "level"
	levelInput.Stages[0].Config["levels"] = []any{3, 4}
	if _, err := svc.Workspace().CreateWorkspaceFormDesign(ctx, levelInput); err != nil {
		t.Fatalf("expected level condition to skip numeric value validation, got %v", err)
	}
}

func conditionWorkflowTemplateSchema(value string) map[string]any {
	return map[string]any{
		"workspace_design": map[string]any{
			"enabled": true,
			"stages": []map[string]any{
				{
					"id": "stage-condition", "type": "condition", "label": "金額條件",
					"config": map[string]any{
						"field": "amount", "operator": ">", "value": value,
						"true_next_stage_id": "stage-approver-high", "false_next_stage_id": "stage-approver-low",
					},
				},
				{
					"id": "stage-approver-high", "type": "approver", "label": "高階主管",
					"config": map[string]any{"account_ids": []any{"acct-reviewer"}},
				},
				{
					"id": "stage-approver-low", "type": "approver", "label": "直屬主管",
					"config": map[string]any{"account_ids": []any{"acct-reviewer"}},
				},
			},
		},
	}
}

func submitConditionWorkflow(t *testing.T, svc *service.Service, store *memory.Store, ctx domain.RequestContext) []domain.WorkflowStageInstance {
	t.Helper()
	instance, err := svc.Workflow().SubmitForm(ctx, domain.SubmitFormInput{
		TemplateKey: "amount-approval",
		Payload:     map[string]any{"amount": 500},
	})
	if err != nil {
		t.Fatal(err)
	}
	run, found, err := store.GetWorkflowRunByFormInstance(context.Background(), "tenant-1", instance.ID)
	if err != nil || !found {
		t.Fatalf("expected workflow run, found=%v err=%v", found, err)
	}
	stages, err := store.ListWorkflowStageInstancesByRun(context.Background(), "tenant-1", run.ID)
	if err != nil {
		t.Fatal(err)
	}
	return stages
}

// TestWorkflowConditionUnparseableValueRoutesFalse 驗證運行期條件值無法解析時按條件不成立路由。
func TestWorkflowConditionUnparseableValueRoutesFalse(t *testing.T) {
	now := time.Date(2026, 7, 16, 8, 0, 0, 0, time.UTC)
	svc, ctx, store := newWorkflowEngineFixture(t, now, "acct-reviewer")
	if err := store.UpsertFormTemplate(context.Background(), domain.FormTemplate{
		ID: "ft-amount", TenantID: "tenant-1", Key: "amount-approval", Name: "Amount",
		Schema: conditionWorkflowTemplateSchema("not-a-number"), CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	stages := submitConditionWorkflow(t, svc, store, ctx)
	var condition *domain.WorkflowStageInstance
	for i := range stages {
		if stages[i].StageID == "stage-condition" {
			condition = &stages[i]
		}
	}
	if condition == nil {
		t.Fatalf("expected condition stage instance, got %+v", stages)
	}
	if matched, _ := condition.Result["matched"].(bool); matched {
		t.Fatalf("expected unparseable condition value to evaluate as not matched, got %+v", condition.Result)
	}
	if target, _ := condition.Result["target_stage_id"].(string); target != "stage-approver-low" {
		t.Fatalf("expected routing to false branch stage-approver-low, got %+v", condition.Result)
	}
}

// TestWorkflowConditionNumericValueStillRoutes 驗證可解析條件值的數值比較不受影響。
func TestWorkflowConditionNumericValueStillRoutes(t *testing.T) {
	now := time.Date(2026, 7, 16, 8, 0, 0, 0, time.UTC)
	svc, ctx, store := newWorkflowEngineFixture(t, now, "acct-reviewer")
	if err := store.UpsertFormTemplate(context.Background(), domain.FormTemplate{
		ID: "ft-amount", TenantID: "tenant-1", Key: "amount-approval", Name: "Amount",
		Schema: conditionWorkflowTemplateSchema("100"), CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	stages := submitConditionWorkflow(t, svc, store, ctx)
	var condition *domain.WorkflowStageInstance
	for i := range stages {
		if stages[i].StageID == "stage-condition" {
			condition = &stages[i]
		}
	}
	if condition == nil {
		t.Fatalf("expected condition stage instance, got %+v", stages)
	}
	if matched, _ := condition.Result["matched"].(bool); !matched {
		t.Fatalf("expected 500 > 100 to match, got %+v", condition.Result)
	}
	if target, _ := condition.Result["target_stage_id"].(string); target != "stage-approver-high" {
		t.Fatalf("expected routing to true branch stage-approver-high, got %+v", condition.Result)
	}
}
