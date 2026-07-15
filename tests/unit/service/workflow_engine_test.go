package service_test

import (
	"errors"
	"testing"
	"time"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/repository/memory"
	"nexus-pro-be/internal/service"
)

func TestParseWorkflowStagesFromTemplateUsesExplicitAssignees(t *testing.T) {
	template := domain.FormTemplate{
		ID:       "ft-1",
		TenantID: "tenant-1",
		Key:      "general",
		Schema:   workflowEnabledTemplateSchema("acct-admin"),
	}
	stages := service.ParseWorkflowStagesFromTemplate(template)
	if len(stages) != 1 {
		t.Fatalf("expected one stage, got %d", len(stages))
	}
	if len(stages[0].Config.AccountIDs) != 1 || stages[0].Config.AccountIDs[0] != "acct-admin" {
		t.Fatalf("expected explicit assignee config, got %+v", stages[0].Config)
	}
}

// TestParseWorkflowStagesFromTemplateUsesIamGroups verifies group safety flags survive template parsing.
func TestParseWorkflowStagesFromTemplateUsesIamGroups(t *testing.T) {
	template := domain.FormTemplate{
		ID:       "ft-group",
		TenantID: "tenant-1",
		Key:      "group-approval",
		Schema: map[string]any{
			"workspace_design": map[string]any{
				"enabled": true,
				"stages": []map[string]any{{
					"id": "stage-group", "type": "approver", "label": "IAM group",
					"config": map[string]any{
						"user_group_ids":            []any{"ug-finance", "ug-security"},
						"exclude_applicant":         true,
						"require_distinct_approver": true,
					},
				}},
			},
		},
	}

	stages := service.ParseWorkflowStagesFromTemplate(template)
	if len(stages) != 1 {
		t.Fatalf("expected one stage, got %d", len(stages))
	}
	config := stages[0].Config
	if len(config.UserGroupIDs) != 2 || !config.ExcludeApplicant || !config.RequireDistinctApprover {
		t.Fatalf("expected IAM group safety config, got %+v", config)
	}
}

func TestWorkflowSubmitCreatesInReviewRun(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	svc, ctx, store := newWorkflowEngineFixture(t, now, "acct-admin")

	instance, err := svc.Workflow().SubmitForm(ctx, domain.SubmitFormInput{
		TemplateKey: "leave-request",
		Payload:     map[string]any{"desc": "test submit"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if instance.Status != "in_review" || instance.CurrentRunID == "" {
		t.Fatalf("expected in_review with active run, got %+v", instance)
	}
	run, ok, err := store.GetWorkflowRunByFormInstance(t.Context(), "tenant-1", instance.ID)
	if err != nil || !ok || run.Status != domain.WorkflowRunStatusRunning {
		t.Fatalf("expected running workflow run, got ok=%v err=%v run=%+v", ok, err, run)
	}
}

// TestWorkflowSubmitKeepsDraftWhenLeaveLinkingFails verifies workflow and leave writes share one transaction.
func TestWorkflowSubmitKeepsDraftWhenLeaveLinkingFails(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	svc, ctx, store := newWorkflowEngineFixture(t, now, "acct-admin")
	draft, err := svc.Workflow().SaveFormDraft(ctx, domain.SaveFormDraftInput{
		TemplateKey: "leave-request",
		Payload: map[string]any{
			"leave_type": "annual",
			"start_at":   "2026-06-11T09:00:00+08:00",
			"end_at":     "2026-06-11T18:00:00+08:00",
			"reason":     "transaction regression",
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = svc.Workflow().SubmitForm(ctx, domain.SubmitFormInput{TemplateKey: draft.ID, Payload: draft.Payload})
	var appErr *domain.AppError
	if !errors.As(err, &appErr) || appErr.Status != 400 || appErr.Message != "leave balance is required for this leave type" {
		t.Fatalf("expected missing leave balance failure, got %T %v", err, err)
	}
	persisted, ok, err := store.GetFormInstance(t.Context(), "tenant-1", draft.ID)
	if err != nil || !ok {
		t.Fatalf("expected original draft to remain, ok=%v err=%v", ok, err)
	}
	if persisted.Status != "draft" || persisted.CurrentRunID != "" {
		t.Fatalf("expected unchanged draft after rollback, got %+v", persisted)
	}
	if _, ok, err := store.GetWorkflowRunByFormInstance(t.Context(), "tenant-1", draft.ID); err != nil || ok {
		t.Fatalf("workflow run must roll back with leave failure, ok=%v err=%v", ok, err)
	}
	if _, ok, err := store.GetLeaveRequestByFormInstanceID(t.Context(), "tenant-1", draft.ID); err != nil || ok {
		t.Fatalf("leave request must not survive failed submit, ok=%v err=%v", ok, err)
	}
}

// TestWorkflowDuplicateSubmitReturnsConflict exposes a stable already-submitted contract to clients.
func TestWorkflowDuplicateSubmitReturnsConflict(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	svc, ctx, _ := newWorkflowEngineFixture(t, now, "acct-admin")
	draft, err := svc.Workflow().SaveFormDraft(ctx, domain.SaveFormDraftInput{
		TemplateKey: "leave-request",
		Payload:     map[string]any{"desc": "duplicate submit"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Workflow().SubmitForm(ctx, domain.SubmitFormInput{TemplateKey: draft.ID}); err != nil {
		t.Fatal(err)
	}

	_, err = svc.Workflow().SubmitForm(ctx, domain.SubmitFormInput{TemplateKey: draft.ID})
	var appErr *domain.AppError
	if !errors.As(err, &appErr) || appErr.Status != 409 || appErr.ReasonCode != "workflow_form_already_submitted" {
		t.Fatalf("expected already-submitted conflict, got %T %v", err, err)
	}
}

// TestWorkflowStateHonorsSelfScope prevents one employee from reading another employee's approval trail.
func TestWorkflowStateHonorsSelfScope(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	svc, applicantCtx, store := newWorkflowEngineFixture(t, now, "acct-admin")

	instance, err := svc.Workflow().SubmitForm(applicantCtx, domain.SubmitFormInput{
		TemplateKey: "leave-request",
		Payload:     map[string]any{"desc": "private approval trail"},
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = store.UpsertAccount(t.Context(), domain.Account{
		ID:                     "acct-other-employee",
		TenantID:               "tenant-1",
		DisplayName:            "Other Employee",
		EmployeeID:             "emp-other-employee",
		Status:                 "active",
		DirectPermissionSetIDs: []string{"ps-workflow-applicant"},
		CreatedAt:              now,
	})
	_ = store.UpsertEmployee(t.Context(), domain.Employee{
		ID:        "emp-other-employee",
		TenantID:  "tenant-1",
		Name:      "Other Employee",
		AccountID: "acct-other-employee",
		Status:    "active",
		CreatedAt: now,
	})

	_, err = svc.Workflow().GetWorkflowFormState(
		domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-other-employee"},
		instance.ID,
	)
	var appErr *domain.AppError
	if !errors.As(err, &appErr) || appErr.Status != 404 {
		t.Fatalf("expected scoped reader to receive not found, got %T %v", err, err)
	}
	if _, err := svc.Workflow().GetWorkflowFormState(applicantCtx, instance.ID); err != nil {
		t.Fatalf("expected applicant to read own workflow state: %v", err)
	}
}

func TestWorkflowTemporalSignalFailsWhenWorkflowMissing(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	svc, applicantCtx, _, fakeTemporal := newWorkflowEngineFixtureWithFake(t, now, "acct-admin")

	instance, err := svc.Workflow().SubmitForm(applicantCtx, domain.SubmitFormInput{
		TemplateKey: "leave-request",
		Payload:     map[string]any{"desc": "missing workflow"},
	})
	if err != nil {
		t.Fatal(err)
	}
	fakeTemporal.forgetWorkflow("tenant-1", instance.ID)

	_, err = svc.Workflow().ApproveForm(domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-admin"}, instance.ID, domain.ApproveFormInput{Reason: "missing"})
	if err == nil {
		t.Fatal("expected missing workflow error")
	}
	var appErr *domain.AppError
	if !errors.As(err, &appErr) || appErr.Status != 404 || appErr.Code != "workflow_not_found" {
		t.Fatalf("expected workflow_not_found 404, got %T %v", err, err)
	}
	if len(fakeTemporal.signals) != 1 {
		t.Fatalf("expected one Temporal signal attempt, got %d", len(fakeTemporal.signals))
	}
}

func TestReturnFormUsesTemporalSignal(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	svc, applicantCtx, store, fakeTemporal := newWorkflowEngineFixtureWithFake(t, now, "acct-admin")

	instance, err := svc.Workflow().SubmitForm(applicantCtx, domain.SubmitFormInput{
		TemplateKey: "leave-request",
		Payload:     map[string]any{"desc": "return"},
	})
	if err != nil {
		t.Fatal(err)
	}
	returned, err := svc.Workflow().ReturnForm(domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-admin"}, instance.ID, domain.ReturnFormInput{Reason: "please update"})
	if err != nil {
		t.Fatal(err)
	}
	if returned.Status != domain.WorkflowFormStatusReturned {
		t.Fatalf("expected returned status, got %+v", returned)
	}
	if len(fakeTemporal.signals) != 1 || fakeTemporal.signals[0].Action != domain.FormApprovalWorkflowActionReturn {
		t.Fatalf("expected one return signal, got %+v", fakeTemporal.signals)
	}
	run, ok, err := store.GetWorkflowRunByFormInstance(t.Context(), "tenant-1", instance.ID)
	if err != nil || !ok || run.Status != domain.WorkflowRunStatusReturned {
		t.Fatalf("expected returned workflow run, got ok=%v err=%v run=%+v", ok, err, run)
	}
}

func TestCustomFormDesignSubmitApproveRoundTrip(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	svc, applicantCtx, store, _ := newWorkflowEngineFixtureWithFake(t, now, "acct-admin")
	_ = store.UpsertPermissionSet(t.Context(), domain.PermissionSet{
		ID:       "ps-workspace-forms",
		TenantID: "tenant-1",
		Name:     "Workspace Forms",
		Permissions: []domain.Permission{
			{Resource: "workflow.form_template", Action: "read", Scope: "all"},
			{Resource: "workflow.form_template", Action: "create", Scope: "all"},
			{Resource: "workflow.form_template", Action: "update", Scope: "all"},
			{Resource: "workflow.form_instance", Action: "update", Scope: "all"},
		},
		CreatedAt: now,
	})
	admin := domain.Account{
		ID:                     "acct-forms-admin",
		TenantID:               "tenant-1",
		DisplayName:            "Forms Admin",
		Status:                 "active",
		DirectPermissionSetIDs: []string{"ps-workspace-forms"},
		CreatedAt:              now,
	}
	_ = store.UpsertAccount(t.Context(), admin)
	adminCtx := domain.RequestContext{TenantID: "tenant-1", AccountID: admin.ID}

	_, err := svc.Workspace().CreateWorkspaceFormDesign(adminCtx, domain.SaveWorkspaceFormDesignInput{
		ID:       "custom-ot",
		Name:     "Custom OT",
		Category: "出勤相關",
		Enabled:  boolPtr(true),
		Fields: []domain.PlatformFormBuilderField{
			{ID: "field-hours", Type: "number", Label: "時數", Placeholder: "0", Required: true},
			{ID: "field-reason", Type: "textarea", Label: "原因", Placeholder: "請描述", Required: true},
		},
		Stages: []domain.PlatformFormBuilderStage{
			{ID: "stage-manager", Type: "approver", Label: "Manager", Detail: "Manager approves", Config: map[string]any{"account_ids": []any{"acct-admin"}}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = svc.Workflow().SubmitForm(applicantCtx, domain.SubmitFormInput{
		TemplateKey: "custom-ot",
		Payload:     map[string]any{"field-hours": 8},
	})
	if err == nil {
		t.Fatal("expected required field validation failure")
	}
	var appErr *domain.AppError
	if !errors.As(err, &appErr) || appErr.Status != 400 {
		t.Fatalf("expected 400 validation error, got %T %v", err, err)
	}

	instance, err := svc.Workflow().SubmitForm(applicantCtx, domain.SubmitFormInput{
		TemplateKey: "custom-ot",
		Payload: map[string]any{
			"field-hours":  8,
			"field-reason": "project deadline",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if instance.Status != domain.WorkflowFormStatusInReview {
		t.Fatalf("expected in_review, got %+v", instance)
	}

	approved, err := svc.Workflow().ApproveForm(domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-admin"}, instance.ID, domain.ApproveFormInput{Reason: "ok"})
	if err != nil {
		t.Fatal(err)
	}
	if approved.Status != "approved" {
		t.Fatalf("expected approved, got %+v", approved)
	}
}

func TestCreateWorkspaceFormDesignRejectsMissingStageConfig(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.UpsertTenant(t.Context(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertPermissionSet(t.Context(), domain.PermissionSet{
		ID:       "ps-workspace-forms",
		TenantID: "tenant-1",
		Name:     "Workspace Forms",
		Permissions: []domain.Permission{
			{Resource: "workflow.form_template", Action: "create", Scope: "all"},
			{Resource: "workflow.form_template", Action: "read", Scope: "all"},
		},
		CreatedAt: now,
	})
	_ = store.UpsertAccount(t.Context(), domain.Account{
		ID:                     "acct-forms",
		TenantID:               "tenant-1",
		Status:                 "active",
		DirectPermissionSetIDs: []string{"ps-workspace-forms"},
		CreatedAt:              now,
	})
	svc := service.New(store, service.Options{Now: func() time.Time { return now }})
	_, err := svc.Workspace().CreateWorkspaceFormDesign(domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-forms"}, domain.SaveWorkspaceFormDesignInput{
		Name: "Broken Flow",
		Stages: []domain.PlatformFormBuilderStage{
			{ID: "stage-1", Type: "approver", Label: "Manager", Detail: "no config"},
		},
	})
	if err == nil {
		t.Fatal("expected missing stage config validation error")
	}
}

func TestSubmitFormMarksWorkflowStartFailedWhenTemporalUnavailable(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	svc, applicantCtx, store, fakeTemporal := newWorkflowEngineFixtureWithFake(t, now, "acct-admin")
	fakeTemporal.failStart = true

	_, err := svc.Workflow().SubmitForm(applicantCtx, domain.SubmitFormInput{
		TemplateKey: "leave-request",
		Payload:     map[string]any{"desc": "temporal down"},
	})
	if err == nil {
		t.Fatal("expected temporal start failure")
	}

	instances, listErr := store.ListFormInstances(t.Context(), "tenant-1")
	if listErr != nil {
		t.Fatal(listErr)
	}
	if len(instances) != 1 {
		t.Fatalf("expected one form instance, got %d", len(instances))
	}
	if instances[0].Status != domain.WorkflowFormStatusWorkflowStartFailed {
		t.Fatalf("expected workflow_start_failed, got %+v", instances[0])
	}
	run, ok, runErr := store.GetWorkflowRunByFormInstance(t.Context(), "tenant-1", instances[0].ID)
	if runErr != nil || !ok || run.Status != domain.WorkflowRunStatusStartFailed {
		t.Fatalf("expected start_failed run, got ok=%v err=%v run=%+v", ok, runErr, run)
	}
}
