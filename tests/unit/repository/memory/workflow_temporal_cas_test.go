package memory_test

import (
	"context"
	"testing"
	"time"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/repository/memory"
)

func TestWorkflowTemporalStartClaimFencesStaleWorkers(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStore()
	base := time.Date(2026, 7, 22, 10, 0, 0, 0, time.UTC)
	seedWorkflowRun(t, store, domain.WorkflowRun{
		ID: "run-1", TenantID: "tenant-1", FormInstanceID: "form-1", TemplateID: "template-1",
		Status: domain.WorkflowRunStatusRunning, TemporalStartStatus: domain.WorkflowTemporalStartPending,
		CreatedAt: base, UpdatedAt: base,
	})

	first, ok, err := store.ClaimWorkflowRunTemporalStart(ctx, "tenant-1", "run-1", base.Add(time.Minute), base)
	if err != nil || !ok {
		t.Fatalf("first claim: ok=%v err=%v", ok, err)
	}
	if first.TemporalStartStatus != domain.WorkflowTemporalStartStarting || !first.UpdatedAt.Equal(base.Add(time.Minute)) {
		t.Fatalf("unexpected first claim: %+v", first)
	}
	if _, ok, err := store.ClaimWorkflowRunTemporalStart(ctx, "tenant-1", "run-1", base.Add(2*time.Minute), base.Add(30*time.Second)); err != nil || ok {
		t.Fatalf("fresh claim must not be stolen: ok=%v err=%v", ok, err)
	}

	second, ok, err := store.ClaimWorkflowRunTemporalStart(ctx, "tenant-1", "run-1", base.Add(3*time.Minute), base.Add(time.Minute))
	if err != nil || !ok {
		t.Fatalf("stale claim recovery: ok=%v err=%v", ok, err)
	}
	if released, err := store.ReleaseWorkflowRunTemporalStart(ctx, "tenant-1", "run-1", first.UpdatedAt, base.Add(4*time.Minute)); err != nil || released {
		t.Fatalf("stale worker release must be fenced: released=%v err=%v", released, err)
	}

	startedAt := base.Add(5 * time.Minute)
	started, ok, err := store.MarkWorkflowRunTemporalStarted(ctx, "tenant-1", "run-1", second.UpdatedAt, domain.FormApprovalWorkflowExecution{
		WorkflowID: "tenant-1:form-1:run-1",
		RunID:      "temporal-run-1",
	}, startedAt)
	if err != nil || !ok {
		t.Fatalf("mark started: ok=%v err=%v", ok, err)
	}
	if started.TemporalStartStatus != domain.WorkflowTemporalStartStarted || started.TemporalStartedAt == nil ||
		!started.TemporalStartedAt.Equal(startedAt) || started.TemporalRunID != "temporal-run-1" {
		t.Fatalf("unexpected started run: %+v", started)
	}
	if _, ok, err := store.MarkWorkflowRunTemporalStarted(ctx, "tenant-1", "run-1", second.UpdatedAt, domain.FormApprovalWorkflowExecution{}, startedAt); err != nil || ok {
		t.Fatalf("completed claim must not be reusable: ok=%v err=%v", ok, err)
	}
}

func TestWorkflowTemporalStartListingAndAbandonCAS(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStore()
	base := time.Date(2026, 7, 22, 11, 0, 0, 0, time.UTC)
	for _, run := range []domain.WorkflowRun{
		{ID: "pending", TemporalStartStatus: domain.WorkflowTemporalStartPending, UpdatedAt: base},
		{ID: "stale", TemporalStartStatus: domain.WorkflowTemporalStartStarting, UpdatedAt: base.Add(time.Minute)},
		{ID: "fresh", TemporalStartStatus: domain.WorkflowTemporalStartStarting, UpdatedAt: base.Add(3 * time.Minute)},
		{ID: "started", TemporalStartStatus: domain.WorkflowTemporalStartStarted, UpdatedAt: base},
	} {
		run.TenantID = "tenant-1"
		run.FormInstanceID = "form-" + run.ID
		run.TemplateID = "template-1"
		run.Status = domain.WorkflowRunStatusRunning
		run.CreatedAt = base
		seedWorkflowRun(t, store, run)
	}

	items, err := store.ListPendingWorkflowRuns(ctx, "tenant-1", base.Add(2*time.Minute), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].ID != "pending" || items[1].ID != "stale" {
		t.Fatalf("claimable runs = %+v, want pending then stale", items)
	}

	abandonedAt := base.Add(4 * time.Minute)
	abandoned, ok, err := store.AbandonPendingWorkflowRunTemporalStart(ctx, "tenant-1", "pending", abandonedAt)
	if err != nil || !ok || abandoned.TemporalStartStatus != domain.WorkflowTemporalStartAbandoned {
		t.Fatalf("abandon pending: run=%+v ok=%v err=%v", abandoned, ok, err)
	}
	if _, ok, err := store.ClaimWorkflowRunTemporalStart(ctx, "tenant-1", "pending", abandonedAt, abandonedAt); err != nil || ok {
		t.Fatalf("abandoned pending run must not be claimable: ok=%v err=%v", ok, err)
	}

	claim, ok, err := store.ClaimWorkflowRunTemporalStart(ctx, "tenant-1", "stale", base.Add(5*time.Minute), base.Add(2*time.Minute))
	if err != nil || !ok {
		t.Fatalf("claim stale: ok=%v err=%v", ok, err)
	}
	if _, ok, err := store.AbandonClaimedWorkflowRunTemporalStart(ctx, "tenant-1", "stale", base.Add(time.Minute), abandonedAt); err != nil || ok {
		t.Fatalf("old claim must not abandon: ok=%v err=%v", ok, err)
	}
	claimedAbandoned, ok, err := store.AbandonClaimedWorkflowRunTemporalStart(ctx, "tenant-1", "stale", claim.UpdatedAt, abandonedAt)
	if err != nil || !ok || claimedAbandoned.TemporalStartStatus != domain.WorkflowTemporalStartAbandoned {
		t.Fatalf("abandon claimed: run=%+v ok=%v err=%v", claimedAbandoned, ok, err)
	}
}

func seedWorkflowRun(t *testing.T, store *memory.Store, run domain.WorkflowRun) {
	t.Helper()
	if err := store.UpsertWorkflowRun(context.Background(), run); err != nil {
		t.Fatal(err)
	}
}
