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

// addWorkflowReviewer creates an active reviewer account with the fixture approval permission set.
func addWorkflowReviewer(t *testing.T, store interface {
	UpsertAccount(context.Context, domain.Account) error
}, accountID string, status string, now time.Time) {
	t.Helper()
	if err := store.UpsertAccount(context.Background(), domain.Account{
		ID:                     accountID,
		TenantID:               "tenant-1",
		DisplayName:            accountID,
		Status:                 status,
		DirectPermissionSetIDs: []string{"ps-workflow-reviewer"},
		CreatedAt:              now,
	}); err != nil {
		t.Fatal(err)
	}
}

// addWorkflowGroupMember adds a membership with explicit validity bounds for routing tests.
func addWorkflowGroupMember(t *testing.T, store interface {
	UpsertGroupMembership(context.Context, domain.GroupMembership) error
}, groupID, accountID string, validFrom time.Time, validUntil *time.Time) {
	t.Helper()
	if err := store.UpsertGroupMembership(context.Background(), domain.GroupMembership{
		ID:          groupID + ":" + accountID,
		TenantID:    "tenant-1",
		UserGroupID: groupID,
		AccountID:   accountID,
		ValidFrom:   validFrom,
		ValidUntil:  validUntil,
		Source:      "manual",
		CreatedAt:   validFrom,
	}); err != nil {
		t.Fatal(err)
	}
}

// TestWorkflowGroupApproverResolutionFiltersUnsafeMembers verifies the routing snapshot is fail-closed.
func TestWorkflowGroupApproverResolutionFiltersUnsafeMembers(t *testing.T) {
	now := time.Date(2026, 7, 13, 8, 0, 0, 0, time.UTC)
	svc, applicantCtx, store := newWorkflowEngineFixture(t, now, "acct-reviewer")
	addWorkflowReviewer(t, store, "acct-disabled", "disabled", now)
	addWorkflowReviewer(t, store, "acct-expired", "active", now)
	addWorkflowReviewer(t, store, "acct-future", "active", now)
	expiredAt := now.Add(30 * time.Minute)
	addWorkflowGroupMember(t, store, "group-approvers", "acct-reviewer", now, nil)
	addWorkflowGroupMember(t, store, "group-approvers", "acct-applicant", now, nil)
	addWorkflowGroupMember(t, store, "group-approvers", "acct-disabled", now, nil)
	addWorkflowGroupMember(t, store, "group-approvers", "acct-expired", now, &expiredAt)
	addWorkflowGroupMember(t, store, "group-approvers", "acct-future", now.Add(2*time.Hour), nil)
	configureWorkflowGroupStage(t, store, []map[string]any{{
		"id": "stage-group", "type": "approver", "label": "安全審批組",
		"config": map[string]any{"user_group_ids": []any{"group-approvers"}, "exclude_applicant": true},
	}}, now)

	instance, err := svc.Workflow().SubmitForm(applicantCtx, domain.SubmitFormInput{
		TemplateKey: "leave-request",
		Payload:     map[string]any{"desc": "group routing"},
	})
	if err != nil {
		t.Fatal(err)
	}
	run, ok, err := store.GetWorkflowRunByFormInstance(t.Context(), "tenant-1", instance.ID)
	if err != nil || !ok {
		t.Fatalf("workflow run missing ok=%v err=%v", ok, err)
	}
	assignees, err := store.ListWorkflowStageAssignees(t.Context(), "tenant-1", run.CurrentStageInstanceID)
	if err != nil {
		t.Fatal(err)
	}
	if len(assignees) != 1 || assignees[0].AccountID != "acct-reviewer" {
		t.Fatalf("expected only the active non-applicant group member, got %+v", assignees)
	}
}

// TestWorkflowGroupApproverMembershipIsRevalidated verifies stale snapshots cannot approve.
func TestWorkflowGroupApproverMembershipIsRevalidated(t *testing.T) {
	now := time.Date(2026, 7, 13, 8, 0, 0, 0, time.UTC)
	svc, applicantCtx, store := newWorkflowEngineFixture(t, now, "acct-reviewer")
	addWorkflowGroupMember(t, store, "group-approvers", "acct-reviewer", now, nil)
	configureWorkflowGroupStage(t, store, []map[string]any{{
		"id": "stage-group", "type": "approver", "label": "安全審批組",
		"config": map[string]any{"user_group_ids": []any{"group-approvers"}, "exclude_applicant": true},
	}}, now)

	instance, err := svc.Workflow().SubmitForm(applicantCtx, domain.SubmitFormInput{TemplateKey: "leave-request", Payload: map[string]any{"desc": "membership revoked"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, err := store.DeleteGroupMembership(t.Context(), "tenant-1", "group-approvers", "acct-reviewer"); err != nil || !ok {
		t.Fatalf("membership removal failed ok=%v err=%v", ok, err)
	}
	_, err = svc.Workflow().ActOnWorkflowStage(domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-reviewer"}, instance.ID, "approve", "stale membership")
	var appErr *domain.AppError
	if !errors.As(err, &appErr) || appErr.Status != 403 {
		t.Fatalf("expected revoked group member to be forbidden, got %T %v", err, err)
	}
}

// TestWorkflowGroupApproverCanRequireDistinctStages verifies segregation of duties across stages.
func TestWorkflowGroupApproverCanRequireDistinctStages(t *testing.T) {
	now := time.Date(2026, 7, 13, 8, 0, 0, 0, time.UTC)
	svc, applicantCtx, store := newWorkflowEngineFixture(t, now, "acct-reviewer")
	addWorkflowReviewer(t, store, "acct-reviewer-2", "active", now)
	addWorkflowGroupMember(t, store, "group-approvers", "acct-reviewer", now, nil)
	addWorkflowGroupMember(t, store, "group-approvers", "acct-reviewer-2", now, nil)
	configureWorkflowGroupStage(t, store, []map[string]any{
		{"id": "stage-one", "type": "approver", "label": "一級審批", "config": map[string]any{"user_group_ids": []any{"group-approvers"}, "exclude_applicant": true}},
		{"id": "stage-two", "type": "approver", "label": "二級審批", "config": map[string]any{"user_group_ids": []any{"group-approvers"}, "exclude_applicant": true, "require_distinct_approver": true}},
	}, now)

	instance, err := svc.Workflow().SubmitForm(applicantCtx, domain.SubmitFormInput{TemplateKey: "leave-request", Payload: map[string]any{"desc": "two stages"}})
	if err != nil {
		t.Fatal(err)
	}
	reviewerCtx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-reviewer"}
	if _, err := svc.Workflow().ActOnWorkflowStage(reviewerCtx, instance.ID, "approve", "first"); err != nil {
		t.Fatal(err)
	}
	run, ok, err := store.GetWorkflowRunByFormInstance(t.Context(), "tenant-1", instance.ID)
	if err != nil || !ok {
		t.Fatalf("workflow run missing after first approval ok=%v err=%v", ok, err)
	}
	assignees, err := store.ListWorkflowStageAssignees(t.Context(), "tenant-1", run.CurrentStageInstanceID)
	if err != nil {
		t.Fatal(err)
	}
	if len(assignees) != 1 || assignees[0].AccountID != "acct-reviewer-2" {
		t.Fatalf("expected prior reviewer to be omitted from the second-stage snapshot, got %+v", assignees)
	}
	_, err = svc.Workflow().ActOnWorkflowStage(reviewerCtx, instance.ID, "approve", "second")
	var appErr *domain.AppError
	if !errors.As(err, &appErr) || appErr.Status != 403 {
		t.Fatalf("expected repeated approver to be forbidden, got %T %v", err, err)
	}
	approved, err := svc.Workflow().ActOnWorkflowStage(domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-reviewer-2"}, instance.ID, "approve", "independent review")
	if err != nil {
		t.Fatal(err)
	}
	if approved.Status != "approved" {
		t.Fatalf("expected second reviewer to complete workflow, got %+v", approved)
	}
}
