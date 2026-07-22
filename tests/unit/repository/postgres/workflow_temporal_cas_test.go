package postgres_test

import (
	"context"
	"testing"
	"time"

	"nexus-pro-api/internal/domain"
	postgresrepo "nexus-pro-api/internal/repository/postgres"
	"nexus-pro-api/internal/utils/tenantctx"
)

func TestWorkflowTemporalStartCASUsesDatabaseFencingToken(t *testing.T) {
	pool := openPostgresIntegrationPool(t)
	t.Cleanup(pool.Close)
	ctx := context.Background()
	var migrated bool
	if err := pool.QueryRow(ctx, `
		select exists (
			select 1 from information_schema.columns
			where table_schema = 'public'
			  and table_name = 'workflow_runs'
			  and column_name = 'temporal_start_status'
		)`).Scan(&migrated); err != nil {
		t.Fatal(err)
	}
	if !migrated {
		t.Skip("workflow Temporal delivery migration is not applied")
	}

	store := postgresrepo.NewStore(pool)
	suffix := time.Now().UTC().Format("20060102150405.000000000")
	tenantID := "tenant-workflow-cas-" + suffix
	accountID := "account-workflow-cas-" + suffix
	templateID := "template-workflow-cas-" + suffix
	formID := "form-workflow-cas-" + suffix
	runID := "run-workflow-cas-" + suffix
	now := time.Date(2026, 7, 22, 12, 0, 0, 123456789, time.UTC)
	tenantCtx := tenantctx.WithTenantID(ctx, tenantID)

	if err := store.UpsertTenant(tenantCtx, domain.Tenant{ID: tenantID, Name: tenantID, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		conn, err := pool.Acquire(cleanupCtx)
		if err != nil {
			t.Errorf("acquire cleanup connection: %v", err)
			return
		}
		defer conn.Release()
		_, _ = conn.Exec(cleanupCtx, "select set_config('app.tenant_id', $1, false)", tenantID)
		if _, err := conn.Exec(cleanupCtx, "delete from tenants where id = $1", tenantID); err != nil {
			t.Errorf("cleanup workflow CAS tenant: %v", err)
		}
		_, _ = conn.Exec(cleanupCtx, "select set_config('app.tenant_id', '', false)")
	})
	if err := store.UpsertAccount(tenantCtx, domain.Account{ID: accountID, TenantID: tenantID, Status: "active", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertFormTemplate(tenantCtx, domain.FormTemplate{
		ID: templateID, TenantID: tenantID, Key: "workflow-cas", Name: "Workflow CAS", Status: "published",
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertFormInstance(tenantCtx, domain.FormInstance{
		ID: formID, TenantID: tenantID, TemplateID: templateID, ApplicantAccountID: accountID,
		Status: domain.WorkflowFormStatusInReview, CurrentRunID: runID, SubmittedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertWorkflowRun(tenantCtx, domain.WorkflowRun{
		ID: runID, TenantID: tenantID, FormInstanceID: formID, TemplateID: templateID, Version: 1,
		Status: domain.WorkflowRunStatusRunning, TemporalStartStatus: domain.WorkflowTemporalStartPending,
		TemporalWorkflowID: domain.FormApprovalWorkflowIDForRun(tenantID, formID, runID), StageDefinitionsJSON: "[]", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	claimable, err := store.ListPendingWorkflowRuns(ctx, tenantID, now, 10)
	if err != nil || len(claimable) != 1 || claimable[0].ID != runID {
		t.Fatalf("pending list: runs=%+v err=%v", claimable, err)
	}
	first, ok, err := store.ClaimWorkflowRunTemporalStart(ctx, tenantID, runID, now.Add(time.Minute), now)
	if err != nil || !ok || first.TemporalStartStatus != domain.WorkflowTemporalStartStarting {
		t.Fatalf("first claim: run=%+v ok=%v err=%v", first, ok, err)
	}
	if _, ok, err := store.ClaimWorkflowRunTemporalStart(ctx, tenantID, runID, now.Add(2*time.Minute), now.Add(30*time.Second)); err != nil || ok {
		t.Fatalf("fresh claim must not be stolen: ok=%v err=%v", ok, err)
	}
	second, ok, err := store.ClaimWorkflowRunTemporalStart(ctx, tenantID, runID, now.Add(3*time.Minute), first.UpdatedAt)
	if err != nil || !ok {
		t.Fatalf("reclaim stale start: run=%+v ok=%v err=%v", second, ok, err)
	}
	if released, err := store.ReleaseWorkflowRunTemporalStart(ctx, tenantID, runID, first.UpdatedAt, now.Add(4*time.Minute)); err != nil || released {
		t.Fatalf("stale release must be fenced: released=%v err=%v", released, err)
	}

	startedAt := now.Add(5 * time.Minute)
	started, ok, err := store.MarkWorkflowRunTemporalStarted(ctx, tenantID, runID, second.UpdatedAt, domain.FormApprovalWorkflowExecution{
		WorkflowID: domain.FormApprovalWorkflowIDForRun(tenantID, formID, runID),
		RunID:      "temporal-" + runID,
	}, startedAt)
	if err != nil || !ok || started.TemporalStartStatus != domain.WorkflowTemporalStartStarted || started.TemporalStartedAt == nil {
		t.Fatalf("mark started: run=%+v ok=%v err=%v", started, ok, err)
	}

	// A real PostgreSQL barrier verifies that cancel and start claim have one
	// linearization winner; both transitions can never commit from pending.
	raceFormID := formID + "-race"
	raceRunID := runID + "-race"
	if err := store.UpsertFormInstance(tenantCtx, domain.FormInstance{
		ID: raceFormID, TenantID: tenantID, TemplateID: templateID, ApplicantAccountID: accountID,
		Status: domain.WorkflowFormStatusInReview, CurrentRunID: raceRunID, SubmittedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertWorkflowRun(tenantCtx, domain.WorkflowRun{
		ID: raceRunID, TenantID: tenantID, FormInstanceID: raceFormID, TemplateID: templateID, Version: 1,
		Status: domain.WorkflowRunStatusRunning, TemporalStartStatus: domain.WorkflowTemporalStartPending,
		TemporalWorkflowID: domain.FormApprovalWorkflowIDForRun(tenantID, raceFormID, raceRunID), StageDefinitionsJSON: "[]", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	type transitionResult struct {
		won bool
		err error
	}
	barrier := make(chan struct{})
	claimResult := make(chan transitionResult, 1)
	abandonResult := make(chan transitionResult, 1)
	go func() {
		<-barrier
		_, won, err := store.ClaimWorkflowRunTemporalStart(ctx, tenantID, raceRunID, now.Add(10*time.Minute), now)
		claimResult <- transitionResult{won: won, err: err}
	}()
	go func() {
		<-barrier
		_, won, err := store.AbandonPendingWorkflowRunTemporalStart(ctx, tenantID, raceRunID, now.Add(10*time.Minute))
		abandonResult <- transitionResult{won: won, err: err}
	}()
	close(barrier)
	claimTransition := <-claimResult
	abandonTransition := <-abandonResult
	if claimTransition.err != nil || abandonTransition.err != nil {
		t.Fatalf("concurrent transition errors: claim=%v abandon=%v", claimTransition.err, abandonTransition.err)
	}
	if claimTransition.won == abandonTransition.won {
		t.Fatalf("exactly one concurrent transition must win: claim=%v abandon=%v", claimTransition.won, abandonTransition.won)
	}
	converged, ok, err := store.GetWorkflowRun(ctx, tenantID, raceRunID)
	if err != nil || !ok {
		t.Fatalf("load concurrent result: ok=%v err=%v", ok, err)
	}
	if claimTransition.won && converged.TemporalStartStatus != domain.WorkflowTemporalStartStarting {
		t.Fatalf("claim winner must leave starting, got %+v", converged)
	}
	if abandonTransition.won && converged.TemporalStartStatus != domain.WorkflowTemporalStartAbandoned {
		t.Fatalf("cancel winner must leave abandoned, got %+v", converged)
	}
}
