package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"nexus-pro-api/internal/domain"
)

// configureWorkflowGroupStage replaces the fixture template with IAM group based approval stages.
func configureWorkflowGroupStage(t *testing.T, store workflowFixtureStore, stages []map[string]any, now time.Time) {
	t.Helper()
	template, ok, err := store.GetFormTemplate(context.Background(), "tenant-1", "ft-leave")
	if err != nil || !ok {
		t.Fatalf("workflow template lookup failed ok=%v err=%v", ok, err)
	}
	template.CurrentVersion++
	template.Schema = map[string]any{
		"workspace_design": map[string]any{
			"enabled": true,
			"stages":  stages,
		},
	}
	template.UpdatedAt = now
	if err := store.UpsertFormTemplate(context.Background(), template); err != nil {
		t.Fatal(err)
	}
}

type workflowFixtureStore interface {
	GetFormTemplate(context.Context, string, string) (domain.FormTemplate, bool, error)
	UpsertFormTemplate(context.Context, domain.FormTemplate) error
}

// TestWorkflowGroupApproverTargetingIsRetired verifies submit rejects legacy user_group_ids stages.
func TestWorkflowGroupApproverTargetingIsRetired(t *testing.T) {
	now := time.Date(2026, 7, 13, 8, 0, 0, 0, time.UTC)
	svc, applicantCtx, store := newWorkflowEngineFixture(t, now, "acct-reviewer")
	configureWorkflowGroupStage(t, store, []map[string]any{{
		"id": "stage-group", "type": "approver", "label": "安全審批組",
		"config": map[string]any{"user_group_ids": []any{"group-approvers"}, "exclude_applicant": true},
	}}, now)

	_, err := svc.Workflow().SubmitForm(applicantCtx, domain.SubmitFormInput{
		TemplateKey: "leave-request",
		Payload:     map[string]any{"desc": "group routing retired"},
	})
	var appErr *domain.AppError
	if !errors.As(err, &appErr) || appErr.Status != 400 {
		t.Fatalf("expected retired group targeting to return 400, got %T %v", err, err)
	}
	if appErr.ReasonCode != "workflow_user_group_retired" {
		t.Fatalf("expected workflow_user_group_retired reason, got %q", appErr.ReasonCode)
	}
}
