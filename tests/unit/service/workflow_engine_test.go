package service_test

import (
	"errors"
	"testing"
	"time"

	"nexus-pro-be/internal/domain"
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
