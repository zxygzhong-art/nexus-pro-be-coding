package postgres_integration_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"nexus-pro-be/internal/domain"
	postgresrepo "nexus-pro-be/internal/repository/postgres"
	"nexus-pro-be/internal/service"
)

// TestPostgresWorkflowRuntimeSemantics 驗證 workflow runtime 表可持久化並依租戶隔離讀寫。
func TestPostgresWorkflowRuntimeSemantics(t *testing.T) {
	pool := openIntegrationPool(t)
	defer pool.Close()
	requireWorkflowRuntimeSchema(t, pool)
	store := postgresrepo.NewStore(pool)
	ctx := context.Background()
	now := time.Now().UTC()
	suffix := strings.ReplaceAll(strings.ToLower(t.Name()), "/", "_") + "_" + now.Format("150405000000")
	tenantA := "tenant_" + suffix + "_a"
	tenantB := "tenant_" + suffix + "_b"
	formA := "fi_" + suffix + "_a"
	formB := "fi_" + suffix + "_b"
	runA := "wfr_" + suffix + "_a"
	stageA := "wfs_" + suffix + "_a"
	accountA := "acct_" + suffix + "_a"
	accountB := "acct_" + suffix + "_b"

	for _, tenantID := range []string{tenantA, tenantB} {
		if err := store.UpsertTenant(ctx, domain.Tenant{ID: tenantID, Name: tenantID, CreatedAt: now}); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.UpsertFormTemplate(ctx, domain.FormTemplate{
		ID: "ft_" + suffix, TenantID: tenantA, Key: "general", Name: "General", CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertFormTemplate(ctx, domain.FormTemplate{
		ID: "ft_" + suffix + "_b", TenantID: tenantB, Key: "general", Name: "General", CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertAccount(ctx, domain.Account{ID: accountA, TenantID: tenantA, Status: "active", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertAccount(ctx, domain.Account{ID: accountB, TenantID: tenantB, Status: "active", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertFormInstance(ctx, domain.FormInstance{
		ID: formA, TenantID: tenantA, TemplateID: "ft_" + suffix, ApplicantAccountID: accountA,
		Status: domain.WorkflowFormStatusInReview, CurrentRunID: runA, SubmittedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertFormInstance(ctx, domain.FormInstance{
		ID: formB, TenantID: tenantB, TemplateID: "ft_" + suffix + "_b", ApplicantAccountID: accountB,
		Status: domain.WorkflowFormStatusInReview, SubmittedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	run := domain.WorkflowRun{
		ID:                     runA,
		TenantID:               tenantA,
		FormInstanceID:         formA,
		TemplateID:             "ft_" + suffix,
		Version:                1,
		Status:                 domain.WorkflowRunStatusRunning,
		CurrentStageInstanceID: stageA,
		StageDefinitionsJSON:   `[{"id":"s1","type":"approver","label":"主管審核"}]`,
		CreatedAt:              now,
		UpdatedAt:              now,
	}
	if err := store.UpsertWorkflowRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	started := now
	if err := store.UpsertWorkflowStageInstance(ctx, domain.WorkflowStageInstance{
		ID: stageA, TenantID: tenantA, RunID: runA, StageID: "s1", StageType: "approver",
		Label: "主管審核", Status: domain.WorkflowStageStatusActive, Sequence: 0, StartedAt: &started,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertWorkflowStageAssignee(ctx, domain.WorkflowStageAssignee{
		TenantID: tenantA, StageInstanceID: stageA, AccountID: accountA, Status: domain.WorkflowAssigneeStatusPending,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.InsertWorkflowAction(ctx, domain.WorkflowAction{
		ID: "wfa_" + suffix, TenantID: tenantA, RunID: runA, StageInstanceID: stageA,
		AccountID: accountA, Action: "submit", CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	gotRun, ok, err := store.GetWorkflowRunByFormInstance(ctx, tenantA, formA)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || gotRun.ID != runA || gotRun.CurrentStageInstanceID != stageA {
		t.Fatalf("expected workflow run for tenant A form, got ok=%v run=%+v", ok, gotRun)
	}
	if _, ok, err := store.GetWorkflowRunByFormInstance(ctx, tenantB, formA); err != nil || ok {
		t.Fatalf("tenant B must not read tenant A workflow run, ok=%v err=%v", ok, err)
	}

	stages, err := store.ListWorkflowStageInstancesByRun(ctx, tenantA, runA)
	if err != nil {
		t.Fatal(err)
	}
	if len(stages) != 1 || stages[0].ID != stageA {
		t.Fatalf("expected one stage instance, got %+v", stages)
	}
	assignees, err := store.ListWorkflowStageAssignees(ctx, tenantA, stageA)
	if err != nil {
		t.Fatal(err)
	}
	if len(assignees) != 1 || assignees[0].AccountID != accountA {
		t.Fatalf("expected one pending assignee, got %+v", assignees)
	}
	pending, err := store.ListPendingAssigneeStageInstanceIDs(ctx, tenantA, accountA)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0] != stageA {
		t.Fatalf("expected pending stage %s, got %+v", stageA, pending)
	}
	actions, err := store.ListWorkflowActionsByRun(ctx, tenantA, runA)
	if err != nil {
		t.Fatal(err)
	}
	if len(actions) != 1 || actions[0].Action != "submit" {
		t.Fatalf("expected submit action, got %+v", actions)
	}

	instance, ok, err := store.GetFormInstance(ctx, tenantA, formA)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || instance.CurrentRunID != runA {
		t.Fatalf("expected form instance current_run_id=%s, got %+v", runA, instance)
	}
}

// TestPostgresWorkflowSubmitApproveServiceSemantics 驗證 service 層 submit → approve 會寫入 workflow runtime。
func TestPostgresWorkflowSubmitApproveServiceSemantics(t *testing.T) {
	pool := openIntegrationPool(t)
	defer pool.Close()
	requireWorkflowRuntimeSchema(t, pool)
	store := postgresrepo.NewStore(pool)
	ctx := context.Background()
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	suffix := strings.ReplaceAll(strings.ToLower(t.Name()), "/", "_") + "_" + now.Format("150405000000")
	tenantID := "tenant_" + suffix
	applicantID := "acct_" + suffix + "_applicant"
	approverID := "acct_" + suffix + "_approver"
	templateID := "ft_" + suffix

	if err := store.UpsertTenant(ctx, domain.Tenant{ID: tenantID, Name: tenantID, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertPermissionSet(ctx, domain.PermissionSet{
		ID: "ps_" + suffix + "_applicant", TenantID: tenantID, Name: "Applicant",
		Permissions: []domain.Permission{
			{Resource: "workflow.form_instance", Action: "submit", Scope: "self"},
			{Resource: "workflow.form_instance", Action: "read", Scope: "self"},
		},
		CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertPermissionSet(ctx, domain.PermissionSet{
		ID: "ps_" + suffix + "_approver", TenantID: tenantID, Name: "Approver",
		Permissions: []domain.Permission{
			{Resource: "workflow.form_instance", Action: "read", Scope: "all"},
			{Resource: "workflow.form_instance", Action: "approve", Scope: "all"},
		},
		CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	applicantEmpID := "emp_" + suffix + "_applicant"
	if err := store.UpsertAccount(ctx, domain.Account{
		ID: applicantID, TenantID: tenantID, DisplayName: "Applicant", EmployeeID: applicantEmpID, Status: "active",
		DirectPermissionSetIDs: []string{"ps_" + suffix + "_applicant"}, CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertEmployee(ctx, domain.Employee{
		ID: applicantEmpID, TenantID: tenantID, Name: "Applicant", AccountID: applicantID,
		Status: "active", EmploymentStatus: "active", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertAccount(ctx, domain.Account{
		ID: approverID, TenantID: tenantID, DisplayName: "Approver", Status: "active",
		DirectPermissionSetIDs: []string{"ps_" + suffix + "_approver"}, CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertFormTemplate(ctx, domain.FormTemplate{
		ID: templateID, TenantID: tenantID, Key: "general", Name: "通用签呈",
		Schema: postgresWorkflowTemplateSchema(approverID), CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	svc := service.New(store, service.Options{Now: func() time.Time { return now.Add(time.Hour) }})
	applicantCtx := domain.RequestContext{TenantID: tenantID, AccountID: applicantID}
	approverCtx := domain.RequestContext{TenantID: tenantID, AccountID: approverID}

	submitted, err := svc.Workflow().SubmitForm(applicantCtx, domain.SubmitFormInput{
		TemplateKey: "general",
		Payload:     map[string]any{"desc": "integration submit"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if submitted.Status != domain.WorkflowFormStatusInReview || submitted.CurrentRunID == "" {
		t.Fatalf("expected in_review form with run, got %+v", submitted)
	}

	state, err := svc.Workflow().GetWorkflowFormState(approverCtx, submitted.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !state.CanAct || len(state.AllowedActions) == 0 || state.RunStatus != domain.WorkflowRunStatusRunning {
		t.Fatalf("expected approver can act on running workflow, got %+v", state)
	}

	approved, err := svc.Workflow().ApproveForm(approverCtx, submitted.ID, domain.ApproveFormInput{Reason: "postgres ok"})
	if err != nil {
		t.Fatal(err)
	}
	if approved.Status != "approved" {
		t.Fatalf("expected approved status, got %+v", approved)
	}

	run, ok, err := store.GetWorkflowRun(ctx, tenantID, submitted.CurrentRunID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || run.Status != domain.WorkflowRunStatusCompleted {
		t.Fatalf("expected completed workflow run, got ok=%v run=%+v", ok, run)
	}

	finalState, err := svc.Workflow().GetWorkflowFormState(applicantCtx, submitted.ID)
	if err != nil {
		t.Fatal(err)
	}
	if finalState.CanAct || finalState.RunStatus != domain.WorkflowRunStatusCompleted {
		t.Fatalf("expected completed workflow state, got %+v", finalState)
	}
}

func postgresWorkflowTemplateSchema(assigneeAccountID string) map[string]any {
	return map[string]any{
		"workspace_design": map[string]any{
			"enabled": true,
			"stages": []map[string]any{{
				"id": "stage-approver", "type": "approver", "label": "审核",
				"config": map[string]any{"account_ids": []any{assigneeAccountID}},
			}},
		},
	}
}

// requireWorkflowRuntimeSchema 驗證 workflow runtime migration 已套用。
func requireWorkflowRuntimeSchema(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	requireMigratedSchema(t, pool)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var ready bool
	err := pool.QueryRow(ctx, `
		select to_regclass('public.workflow_runs') is not null
		   and to_regclass('public.workflow_stage_instances') is not null
		   and to_regclass('public.workflow_stage_assignees') is not null
		   and to_regclass('public.workflow_actions') is not null
	`).Scan(&ready)
	if err != nil {
		t.Fatal(err)
	}
	if !ready {
		t.Skip("workflow runtime tables are not migrated; run migration 000006")
	}
}
