package service_test

import (
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
