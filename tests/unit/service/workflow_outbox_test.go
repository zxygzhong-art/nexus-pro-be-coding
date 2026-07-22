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

type workflowAuditFailureStore struct {
	repository.Store
	fail *bool
}

func (s workflowAuditFailureStore) WithTenantTransaction(ctx context.Context, tenantID string, fn func(repository.Store) error) error {
	transactor := s.Store.(repository.TenantTransactor)
	return transactor.WithTenantTransaction(ctx, tenantID, func(tx repository.Store) error {
		return fn(workflowAuditFailureStore{Store: tx, fail: s.fail})
	})
}

func (s workflowAuditFailureStore) AppendAuditLog(ctx context.Context, item domain.AuditLog) error {
	if s.fail != nil && *s.fail && item.Action == "workflow.form.approve" {
		return errors.New("forced workflow audit failure")
	}
	return s.Store.AppendAuditLog(ctx, item)
}

func newWorkflowOutboxFixture(t *testing.T, now time.Time) (*service.Service, domain.RequestContext, *fakeFormApprovalWorkflowClient) {
	t.Helper()
	_, ctx, store, _ := newWorkflowEngineFixtureWithFake(t, now, "acct-reviewer")
	svc, temporal := newServiceWithFakeFormApprovalWorkflows(store, service.Options{
		Now:                        func() time.Time { return now.Add(time.Hour) },
		WorkflowStartOutboxEnabled: true,
	})
	return svc, ctx, temporal
}

func TestWorkflowStartOutboxKeepsCommittedSubmissionWhenTemporalIsDown(t *testing.T) {
	now := time.Date(2026, 7, 22, 9, 0, 0, 0, time.UTC)
	svc, applicantCtx, temporal := newWorkflowOutboxFixture(t, now)
	temporal.failStart = true

	instance, err := svc.Workflow().SubmitForm(applicantCtx, domain.SubmitFormInput{
		TemplateKey: "leave-request",
		Payload:     map[string]any{"desc": "durable start"},
	})
	if err != nil {
		t.Fatalf("submission must commit while Temporal is unavailable: %v", err)
	}
	run, ok, err := svc.Store().GetWorkflowRunByFormInstance(t.Context(), applicantCtx.TenantID, instance.ID)
	if err != nil || !ok {
		t.Fatalf("workflow run lookup failed ok=%v err=%v", ok, err)
	}
	if run.Status != domain.WorkflowRunStatusRunning || run.TemporalStartStatus != domain.WorkflowTemporalStartPending {
		t.Fatalf("expected running business state with pending delivery, got %+v", run)
	}
	events, err := svc.Store().ListOutboxEvents(t.Context(), applicantCtx.TenantID)
	if err != nil || len(events) != 1 {
		t.Fatalf("expected one workflow start event, count=%d err=%v", len(events), err)
	}
	if events[0].EventType != domain.WorkflowStartRequestedEventType || events[0].IdempotencyKey != run.ID {
		t.Fatalf("unexpected workflow start event: %+v", events[0])
	}

	reviewerCtx := domain.RequestContext{TenantID: applicantCtx.TenantID, AccountID: "acct-reviewer"}
	state, err := svc.Workflow().GetWorkflowFormState(reviewerCtx, instance.ID)
	if err != nil {
		t.Fatal(err)
	}
	if state.CanAct || state.TemporalStartStatus != domain.WorkflowTemporalStartPending {
		t.Fatalf("pending start must not be actionable: %+v", state)
	}
	_, err = svc.Workflow().ApproveForm(reviewerCtx, instance.ID, domain.ApproveFormInput{Reason: "too early"})
	var appErr *domain.AppError
	if !errors.As(err, &appErr) || appErr.Status != 409 || appErr.ReasonCode != "workflow_start_pending" {
		t.Fatalf("expected workflow_start_pending conflict, got %T %v", err, err)
	}

	if err := svc.Workflow().HandleWorkflowStartEvent(t.Context(), events[0]); err == nil {
		t.Fatal("expected the outbox handler to retain the event while Temporal is down")
	}
	run, ok, err = svc.Store().GetWorkflowRunByFormInstance(t.Context(), applicantCtx.TenantID, instance.ID)
	if err != nil || !ok || run.TemporalStartStatus != domain.WorkflowTemporalStartPending {
		t.Fatalf("failed ensure must release its fenced claim for retry, ok=%v err=%v run=%+v", ok, err, run)
	}
	committed, ok, err := svc.Store().GetFormInstance(t.Context(), applicantCtx.TenantID, instance.ID)
	if err != nil || !ok || committed.Status != domain.WorkflowFormStatusInReview {
		t.Fatalf("Temporal failure must not compensate the committed form: ok=%v err=%v form=%+v", ok, err, committed)
	}

	temporal.failStart = false
	if err := svc.Workflow().HandleWorkflowStartEvent(t.Context(), events[0]); err != nil {
		t.Fatal(err)
	}
	run, ok, err = svc.Store().GetWorkflowRunByFormInstance(t.Context(), applicantCtx.TenantID, instance.ID)
	if err != nil || !ok || run.TemporalStartStatus != domain.WorkflowTemporalStartStarted || run.TemporalStartedAt == nil {
		t.Fatalf("expected converged Temporal delivery, ok=%v err=%v run=%+v", ok, err, run)
	}
	if len(temporal.starts) != 2 {
		t.Fatalf("expected one failed and one successful Temporal ensure call, got %d", len(temporal.starts))
	}
	if err := svc.Workflow().HandleWorkflowStartEvent(t.Context(), events[0]); err != nil {
		t.Fatal(err)
	}
	if len(temporal.starts) != 2 {
		t.Fatalf("completed event replay must be a no-op, got %d starts", len(temporal.starts))
	}
}

func TestWorkflowStartClaimSerializesConcurrentCancellation(t *testing.T) {
	now := time.Date(2026, 7, 22, 11, 30, 0, 0, time.UTC)
	svc, applicantCtx, temporal := newWorkflowOutboxFixture(t, now)
	instance, err := svc.Workflow().SubmitForm(applicantCtx, domain.SubmitFormInput{
		TemplateKey: "leave-request",
		Payload:     map[string]any{"desc": "cancel during fenced start"},
	})
	if err != nil {
		t.Fatal(err)
	}
	events, err := svc.Store().ListOutboxEvents(t.Context(), applicantCtx.TenantID)
	if err != nil || len(events) != 1 {
		t.Fatalf("expected one start event, count=%d err=%v", len(events), err)
	}
	startSeen := make(chan struct{}, 1)
	startGate := make(chan struct{})
	temporal.startSeen = startSeen
	temporal.startGate = startGate
	handlerResult := make(chan error, 1)
	go func() {
		handlerResult <- svc.Workflow().HandleWorkflowStartEvent(t.Context(), events[0])
	}()
	select {
	case <-startSeen:
	case <-time.After(time.Second):
		t.Fatal("Temporal start handler did not acquire its claim")
	}

	cancellerCtx := domain.RequestContext{TenantID: applicantCtx.TenantID, AccountID: "acct-reviewer", IdempotencyKey: "cancel-race-1"}
	_, err = svc.Workflow().CancelForm(cancellerCtx, instance.ID, domain.CancelFormInput{Reason: "cancel during start"})
	var appErr *domain.AppError
	if !errors.As(err, &appErr) || appErr.Status != 409 || appErr.ReasonCode != "workflow_start_raced" {
		t.Fatalf("the start claim must fence a concurrent local cancel, got %T %v", err, err)
	}
	run, ok, err := svc.Store().GetWorkflowRunByFormInstance(t.Context(), applicantCtx.TenantID, instance.ID)
	if err != nil || !ok || run.TemporalStartStatus != domain.WorkflowTemporalStartStarting || run.Status != domain.WorkflowRunStatusRunning {
		t.Fatalf("concurrent cancel must not overwrite the claimed run, ok=%v err=%v run=%+v", ok, err, run)
	}

	close(startGate)
	if err := <-handlerResult; err != nil {
		t.Fatal(err)
	}
	temporal.startGate = nil
	cancelled, err := svc.Workflow().CancelForm(cancellerCtx, instance.ID, domain.CancelFormInput{Reason: "cancel during start"})
	if err != nil {
		t.Fatal(err)
	}
	if cancelled.Status != "cancelled" || len(temporal.signals) != 1 || temporal.signals[0].Action != domain.FormApprovalWorkflowActionWithdraw {
		t.Fatalf("retry after delivery must withdraw the Temporal execution: form=%+v signals=%+v", cancelled, temporal.signals)
	}
}

func TestTemporalWorkflowAuditAndCommandReceiptCommitAtomically(t *testing.T) {
	now := time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC)
	_, applicantCtx, baseStore, _ := newWorkflowEngineFixtureWithFake(t, now, "acct-reviewer")
	failAudit := false
	store := workflowAuditFailureStore{Store: baseStore, fail: &failAudit}
	svc, _ := newServiceWithFakeFormApprovalWorkflows(store, service.Options{
		Now:                        func() time.Time { return now.Add(time.Hour) },
		WorkflowStartOutboxEnabled: true,
	})
	instance, err := svc.Workflow().SubmitForm(applicantCtx, domain.SubmitFormInput{
		TemplateKey: "leave-request",
		Payload:     map[string]any{"desc": "atomic audit"},
	})
	if err != nil {
		t.Fatal(err)
	}
	events, err := svc.Store().ListOutboxEvents(t.Context(), applicantCtx.TenantID)
	if err != nil || len(events) != 1 {
		t.Fatalf("expected one start event, count=%d err=%v", len(events), err)
	}
	if err := svc.Workflow().HandleWorkflowStartEvent(t.Context(), events[0]); err != nil {
		t.Fatal(err)
	}
	run, ok, err := svc.Store().GetWorkflowRunByFormInstance(t.Context(), applicantCtx.TenantID, instance.ID)
	if err != nil || !ok {
		t.Fatalf("workflow run lookup failed ok=%v err=%v", ok, err)
	}
	reviewerCtx := domain.RequestContext{TenantID: applicantCtx.TenantID, AccountID: "acct-reviewer", RequestID: "req-atomic-audit"}
	signal := domain.FormApprovalWorkflowSignal{
		TenantID:           applicantCtx.TenantID,
		FormInstanceID:     instance.ID,
		RunID:              run.ID,
		WorkflowID:         run.TemporalWorkflowID,
		AccountID:          reviewerCtx.AccountID,
		Action:             domain.FormApprovalWorkflowActionApprove,
		Reason:             "atomic audit",
		IdempotencyKey:     "approval-atomic-audit",
		CommandFingerprint: "ignored-client-fingerprint",
	}
	failAudit = true
	if _, err := svc.Workflow().ApplyTemporalFormApprovalSignal(reviewerCtx, signal); err == nil {
		t.Fatal("expected the forced audit failure to abort the workflow transaction")
	}
	if _, found, err := svc.Store().GetWorkflowActionByIdempotencyKey(t.Context(), applicantCtx.TenantID, run.ID, signal.IdempotencyKey); err != nil || found {
		t.Fatalf("audit failure must roll back the command receipt, found=%v err=%v", found, err)
	}
	current, ok, err := svc.Store().GetFormInstance(t.Context(), applicantCtx.TenantID, instance.ID)
	if err != nil || !ok || current.Status != domain.WorkflowFormStatusInReview {
		t.Fatalf("audit failure must roll back the business projection, ok=%v err=%v form=%+v", ok, err, current)
	}

	failAudit = false
	if _, err := svc.Workflow().ApplyTemporalFormApprovalSignal(reviewerCtx, signal); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Workflow().ApplyTemporalFormApprovalSignal(reviewerCtx, signal); err != nil {
		t.Fatal(err)
	}
	audits, err := svc.Store().ListAuditLogs(t.Context(), applicantCtx.TenantID)
	if err != nil {
		t.Fatal(err)
	}
	approvalAudits := 0
	for _, audit := range audits {
		if audit.Action == "workflow.form.approve" {
			approvalAudits++
		}
	}
	if approvalAudits != 1 {
		t.Fatalf("committed receipt must correspond to exactly one audit record, got %d", approvalAudits)
	}
}

func TestWorkflowCommandIdempotencyReturnsTheRecordedResult(t *testing.T) {
	now := time.Date(2026, 7, 22, 10, 0, 0, 0, time.UTC)
	svc, applicantCtx, temporal := newWorkflowOutboxFixture(t, now)
	instance, err := svc.Workflow().SubmitForm(applicantCtx, domain.SubmitFormInput{
		TemplateKey: "leave-request",
		Payload:     map[string]any{"desc": "idempotent approval"},
	})
	if err != nil {
		t.Fatal(err)
	}
	events, err := svc.Store().ListOutboxEvents(t.Context(), applicantCtx.TenantID)
	if err != nil || len(events) != 1 {
		t.Fatalf("expected one start event, count=%d err=%v", len(events), err)
	}
	if err := svc.Workflow().HandleWorkflowStartEvent(t.Context(), events[0]); err != nil {
		t.Fatal(err)
	}

	reviewerCtx := domain.RequestContext{
		TenantID:       applicantCtx.TenantID,
		AccountID:      "acct-reviewer",
		RequestID:      "req-approval-1",
		IdempotencyKey: "approval-command-1",
	}
	first, err := svc.Workflow().ApproveForm(reviewerCtx, instance.ID, domain.ApproveFormInput{Reason: "looks good"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.Workflow().ApproveForm(reviewerCtx, instance.ID, domain.ApproveFormInput{Reason: "looks good"})
	if err != nil {
		t.Fatal(err)
	}
	if first.Status != second.Status || len(temporal.signals) != 1 {
		t.Fatalf("idempotent replay must return current state without a second signal: first=%+v second=%+v signals=%d", first, second, len(temporal.signals))
	}
	_, err = svc.Workflow().ApproveForm(reviewerCtx, instance.ID, domain.ApproveFormInput{Reason: "changed payload"})
	var appErr *domain.AppError
	if !errors.As(err, &appErr) || appErr.Status != 409 || appErr.ReasonCode != "idempotency_key_reused" {
		t.Fatalf("expected idempotency_key_reused, got %T %v", err, err)
	}
}

func TestCancelPendingWorkflowStartNeverCreatesAnOrphanExecution(t *testing.T) {
	now := time.Date(2026, 7, 22, 11, 0, 0, 0, time.UTC)
	svc, applicantCtx, temporal := newWorkflowOutboxFixture(t, now)
	instance, err := svc.Workflow().SubmitForm(applicantCtx, domain.SubmitFormInput{
		TemplateKey: "leave-request",
		Payload:     map[string]any{"desc": "cancel before delivery"},
	})
	if err != nil {
		t.Fatal(err)
	}
	events, err := svc.Store().ListOutboxEvents(t.Context(), applicantCtx.TenantID)
	if err != nil || len(events) != 1 {
		t.Fatalf("expected one start event, count=%d err=%v", len(events), err)
	}
	reviewerCtx := domain.RequestContext{TenantID: applicantCtx.TenantID, AccountID: "acct-reviewer"}
	cancelled, err := svc.Workflow().CancelForm(reviewerCtx, instance.ID, domain.CancelFormInput{Reason: "no longer needed"})
	if err != nil {
		t.Fatal(err)
	}
	if cancelled.Status != "cancelled" {
		t.Fatalf("expected locally cancelled form, got %+v", cancelled)
	}
	run, ok, err := svc.Store().GetWorkflowRunByFormInstance(t.Context(), applicantCtx.TenantID, instance.ID)
	if err != nil || !ok || run.Status != domain.WorkflowRunStatusCancelled || run.TemporalStartStatus != domain.WorkflowTemporalStartAbandoned {
		t.Fatalf("expected abandoned pending start, ok=%v err=%v run=%+v", ok, err, run)
	}
	if err := svc.Workflow().HandleWorkflowStartEvent(t.Context(), events[0]); err != nil {
		t.Fatal(err)
	}
	if len(temporal.starts) != 0 {
		t.Fatalf("cancelled pending event must not start Temporal, got %d calls", len(temporal.starts))
	}
}
