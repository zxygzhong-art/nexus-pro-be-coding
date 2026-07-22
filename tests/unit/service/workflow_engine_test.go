package service_test

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/repository/memory"
	"nexus-pro-api/internal/service"
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

// TestParseWorkflowStagesFromTemplatePreservesLegacyIamGroupsForRejection detects retired group targeting.
func TestParseWorkflowStagesFromTemplatePreservesLegacyIamGroupsForRejection(t *testing.T) {
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
	if len(config.UserGroupIDs) != 2 {
		t.Fatalf("expected legacy user_group_ids to remain detectable, got %+v", config)
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

// TestRuntimeFormTemplateAndSubmitVersionBinding keeps rendering and submission on one published contract.
func TestRuntimeFormTemplateAndSubmitVersionBinding(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	svc, ctx, store := newWorkflowEngineFixture(t, now, "acct-admin")

	runtimeTemplate, err := svc.Workflow().GetRuntimeFormTemplate(ctx, "leave-request", "")
	if err != nil {
		t.Fatal(err)
	}
	if runtimeTemplate.TemplateVersionID == "" || len(runtimeTemplate.Fields) == 0 || len(runtimeTemplate.Stages) == 0 {
		t.Fatalf("expected a versioned runtime contract, got %+v", runtimeTemplate)
	}

	template, ok, err := store.GetFormTemplateByKey(t.Context(), "tenant-1", "leave-request")
	if err != nil || !ok {
		t.Fatalf("expected seeded template, ok=%v err=%v", ok, err)
	}
	template.CurrentVersion = 2
	template.UpdatedAt = now.Add(2 * time.Hour)
	if err := store.UpsertFormTemplate(t.Context(), template); err != nil {
		t.Fatal(err)
	}

	_, err = svc.Workflow().SubmitForm(ctx, domain.SubmitFormInput{
		TemplateKey:       "leave-request",
		TemplateVersionID: runtimeTemplate.TemplateVersionID,
		Payload:           map[string]any{"desc": "stale rendered schema"},
	})
	var appErr *domain.AppError
	if !errors.As(err, &appErr) || appErr.Status != 409 || appErr.ReasonCode != "workflow_form_template_version_changed" {
		t.Fatalf("expected stale template version conflict, got %T %v", err, err)
	}
}

// TestWorkflowSubmitKeepsDraftWhenLeaveLinkingFails verifies workflow and leave writes share one transaction.
func TestWorkflowSubmitKeepsDraftWhenLeaveLinkingFails(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	svc, ctx, store := newWorkflowEngineFixture(t, now, "acct-admin")
	draft, err := svc.Workflow().SaveFormDraft(ctx, domain.SaveFormDraftInput{
		TemplateKey: "leave-request",
		Payload: map[string]any{
			"leave_type": "not-a-real-leave",
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
	if !errors.As(err, &appErr) || appErr.Status != 400 || appErr.Message != "unknown leave type" {
		t.Fatalf("expected unsupported leave type failure, got %T %v", err, err)
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

// TestWorkflowSubmitRejectsInjectedLeaveLink verifies a new form cannot reuse another form's leave request.
func TestWorkflowSubmitRejectsInjectedLeaveLink(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	svc, ctx, store := newWorkflowEngineFixture(t, now, "acct-admin")
	victim := domain.LeaveRequest{
		ID:             "lr-victim",
		TenantID:       "tenant-1",
		EmployeeID:     "emp-victim",
		LeaveType:      "annual",
		StartAt:        now.Add(24 * time.Hour),
		EndAt:          now.Add(32 * time.Hour),
		Hours:          8,
		Status:         "pending_approval",
		FormInstanceID: "fi-victim",
		CreatedAt:      now,
	}
	if err := store.UpsertLeaveRequest(t.Context(), victim); err != nil {
		t.Fatal(err)
	}
	victimBefore, ok, err := store.GetLeaveRequest(t.Context(), "tenant-1", victim.ID)
	if err != nil || !ok {
		t.Fatalf("expected victim request snapshot, ok=%v err=%v", ok, err)
	}

	instance, err := svc.Workflow().SubmitForm(ctx, domain.SubmitFormInput{
		TemplateKey: "leave-request",
		Payload: map[string]any{
			"leave_request_id":     victim.ID,
			"linked_resource_id":   victim.ID,
			"linked_resource_type": "attendance.leave_request",
			"leave_type":           "annual",
			"start_at":             "2026-06-12T09:00:00+08:00",
			"end_at":               "2026-06-12T17:00:00+08:00",
			"reason":               "link injection regression",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	linked, ok, err := store.GetLeaveRequestByFormInstanceID(t.Context(), "tenant-1", instance.ID)
	if err != nil || !ok {
		t.Fatalf("expected a leave request linked to the submitted form, ok=%v err=%v", ok, err)
	}
	if linked.ID == victim.ID || linked.FormInstanceID != instance.ID {
		t.Fatalf("expected an independent server link, victim=%+v linked=%+v", victim, linked)
	}
	for _, key := range []string{"leave_request_id", "linked_resource_id"} {
		if instance.Payload[key] != linked.ID {
			t.Fatalf("expected payload %q to use the current form link, got %+v", key, instance.Payload)
		}
	}
	victimAfter, ok, err := store.GetLeaveRequest(t.Context(), "tenant-1", victim.ID)
	if err != nil || !ok || !reflect.DeepEqual(victimAfter, victimBefore) {
		t.Fatalf("expected victim request to remain unchanged, ok=%v err=%v before=%+v after=%+v", ok, err, victimBefore, victimAfter)
	}
}

// TestWorkflowSubmitBindsLeaveToApplicant verifies client employee metadata cannot redirect leave side effects.
func TestWorkflowSubmitBindsLeaveToApplicant(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	svc, ctx, store := newWorkflowEngineFixture(t, now, "acct-admin")
	if err := store.UpsertEmployee(t.Context(), domain.Employee{
		ID: "emp-victim", TenantID: "tenant-1", Name: "Victim", AccountID: "acct-victim",
		Status: "active", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertAccount(t.Context(), domain.Account{
		ID: "acct-victim", TenantID: "tenant-1", DisplayName: "Victim", EmployeeID: "emp-victim",
		Status: "active", CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	instance, err := svc.Workflow().SubmitForm(ctx, domain.SubmitFormInput{
		TemplateKey: "leave-request",
		Payload: map[string]any{
			"employee_id": "emp-victim",
			"leave_type":  "annual",
			"start_at":    "2026-06-12T09:00:00+08:00",
			"end_at":      "2026-06-12T17:00:00+08:00",
			"reason":      "employee injection regression",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	linked, ok, err := store.GetLeaveRequestByFormInstanceID(t.Context(), "tenant-1", instance.ID)
	if err != nil || !ok {
		t.Fatalf("expected linked leave request, ok=%v err=%v", ok, err)
	}
	if linked.EmployeeID != "emp-applicant" || instance.Payload["employee_id"] != "emp-applicant" {
		t.Fatalf("expected server-derived applicant employee, request=%+v payload=%+v", linked, instance.Payload)
	}
}

// TestWorkflowSubmitCreatesOvertimeProjection keeps standard form submission and attendance projection atomic.
func TestWorkflowSubmitCreatesOvertimeProjection(t *testing.T) {
	now := time.Date(2026, 7, 15, 8, 0, 0, 0, time.UTC)
	svc, ctx, store := newWorkflowEngineFixture(t, now, "acct-admin")
	if err := store.UpsertFormTemplate(t.Context(), domain.FormTemplate{
		ID: "ft-overtime", TenantID: "tenant-1", Key: "overtime-approval", Name: "Overtime",
		Schema: workflowEnabledTemplateSchema("acct-admin"), CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	instance, err := svc.Workflow().SubmitForm(ctx, domain.SubmitFormInput{
		TemplateKey: "overtime-approval",
		Payload: map[string]any{
			"employee_id":       "emp-victim",
			"start_at":          "2026-07-16T18:00:00+08:00",
			"end_at":            "2026-07-16T21:00:00+08:00",
			"hours":             3,
			"overtime_type":     "weekday",
			"compensation_type": "leave",
			"reason":            "release",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	request, ok, err := store.GetOvertimeRequestByFormInstanceID(t.Context(), "tenant-1", instance.ID)
	if err != nil || !ok {
		t.Fatalf("expected overtime projection for submitted form, ok=%v err=%v", ok, err)
	}
	if request.EmployeeID != "emp-applicant" || request.Status != "pending_approval" {
		t.Fatalf("expected server-owned applicant projection, got %+v", request)
	}
	if instance.Payload["overtime_request_id"] != request.ID || instance.Payload["linked_resource_id"] != request.ID {
		t.Fatalf("expected server-owned overtime link, got %+v", instance.Payload)
	}
}

// TestDirectAttendanceCreateStartsWorkflow verifies compatibility endpoints use the standard runtime.
func TestDirectAttendanceCreateStartsWorkflow(t *testing.T) {
	now := time.Date(2026, 7, 15, 8, 0, 0, 0, time.UTC)
	svc, ctx, store, fakeTemporal := newWorkflowEngineFixtureWithFake(t, now, "acct-admin")
	grantDirectAttendanceCreatePermission(t, store)
	if err := store.UpsertFormTemplate(t.Context(), domain.FormTemplate{
		ID: "ft-overtime", TenantID: "tenant-1", Key: "overtime-approval", Name: "Overtime",
		Schema: workflowEnabledTemplateSchema("acct-admin"), CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	leave, err := svc.Attendance().CreateLeaveRequest(ctx, domain.CreateLeaveRequestInput{
		LeaveType: "annual", StartAt: "2026-07-16T09:00:00+08:00", EndAt: "2026-07-16T17:00:00+08:00",
	})
	if err != nil {
		t.Fatal(err)
	}
	overtime, err := svc.Attendance().CreateOvertimeRequest(ctx, domain.CreateOvertimeRequestInput{
		StartAt: "2026-07-16T18:00:00+08:00", EndAt: "2026-07-16T21:00:00+08:00", Hours: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, formInstanceID := range []string{leave.FormInstanceID, overtime.FormInstanceID} {
		instance, found, lookupErr := store.GetFormInstance(t.Context(), "tenant-1", formInstanceID)
		if lookupErr != nil || !found || instance.Status != "in_review" {
			t.Fatalf("expected in-review form %q, found=%v err=%v instance=%+v", formInstanceID, found, lookupErr, instance)
		}
		run, found, lookupErr := store.GetWorkflowRunByFormInstance(t.Context(), "tenant-1", formInstanceID)
		if lookupErr != nil || !found || run.Status != domain.WorkflowRunStatusRunning {
			t.Fatalf("expected running workflow for %q, found=%v err=%v run=%+v", formInstanceID, found, lookupErr, run)
		}
	}
	if len(fakeTemporal.starts) != 2 {
		t.Fatalf("expected both compatibility requests to start Temporal, got %+v", fakeTemporal.starts)
	}
}

// TestReturnedOvertimeFormRefreshesLinkedProjection verifies resubmission keeps one stable overtime request.
func TestReturnedOvertimeFormRefreshesLinkedProjection(t *testing.T) {
	now := time.Date(2026, 7, 15, 8, 0, 0, 0, time.UTC)
	svc, ctx, store, _ := newWorkflowEngineFixtureWithFake(t, now, "acct-admin")
	grantDirectAttendanceCreatePermission(t, store)
	permissionSet, ok, err := store.GetPermissionSet(t.Context(), "tenant-1", "ps-workflow-applicant")
	if err != nil || !ok {
		t.Fatalf("workflow applicant permission lookup failed ok=%v err=%v", ok, err)
	}
	permissionSet.Permissions = append(permissionSet.Permissions, domain.Permission{
		Resource: "workflow.form_instance", Action: "update", Scope: "self",
	})
	if err := store.UpsertPermissionSet(t.Context(), permissionSet); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertFormTemplate(t.Context(), domain.FormTemplate{
		ID: "ft-overtime", TenantID: "tenant-1", Key: "overtime-approval", Name: "Overtime",
		Schema: workflowEnabledTemplateSchema("acct-admin"), CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	created, err := svc.Attendance().CreateOvertimeRequest(ctx, domain.CreateOvertimeRequestInput{
		StartAt: "2026-07-16T18:00:00+08:00", EndAt: "2026-07-16T21:00:00+08:00", Hours: 3,
		OvertimeType: "weekday", CompensationType: "leave", Reason: "original reason",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Workflow().ReturnForm(domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-admin"}, created.FormInstanceID, domain.ReturnFormInput{Reason: "please supplement"}); err != nil {
		t.Fatal(err)
	}
	updated, err := svc.Workflow().UpdateFormDraft(ctx, created.FormInstanceID, domain.UpdateFormDraftInput{Payload: map[string]any{
		"start_at": "2026-07-17T18:00:00+08:00", "end_at": "2026-07-17T20:00:00+08:00", "hours": 2,
		"overtime_type": "weekday", "compensation_type": "pay", "reason": "supplemented reason",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Workflow().SubmitForm(ctx, domain.SubmitFormInput{TemplateKey: created.FormInstanceID, Payload: updated.Payload}); err != nil {
		t.Fatal(err)
	}
	request, ok, err := store.GetOvertimeRequestByFormInstanceID(t.Context(), "tenant-1", created.FormInstanceID)
	if err != nil || !ok {
		t.Fatalf("resubmitted overtime lookup failed ok=%v err=%v", ok, err)
	}
	if request.ID != created.ID || request.Status != "pending_approval" || request.Hours != 2 || request.CompensationType != "pay" || request.Reason != "supplemented reason" || request.StartAt.Day() != 17 {
		t.Fatalf("expected stable refreshed overtime projection, got %+v", request)
	}
}

// TestDirectLeaveStartFailureRestoresBalanceBeforeRetry verifies only the successful retry keeps a reservation.
func TestDirectLeaveStartFailureRestoresBalanceBeforeRetry(t *testing.T) {
	now := time.Date(2026, 7, 15, 8, 0, 0, 0, time.UTC)
	svc, ctx, store, fakeTemporal := newWorkflowEngineFixtureWithFake(t, now, "acct-admin")
	grantDirectAttendanceCreatePermission(t, store)
	if err := store.UpsertLeaveBalance(t.Context(), domain.LeaveBalance{
		ID: "lb-direct-retry", TenantID: "tenant-1", EmployeeID: "emp-applicant", LeaveType: "annual",
		RemainingHours: 16, GrantedHours: 16, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	input := domain.CreateLeaveRequestInput{
		LeaveType: "annual", StartAt: "2026-07-16T09:00:00+08:00", EndAt: "2026-07-16T17:00:00+08:00",
		Reason: "temporal retry compensation",
	}

	fakeTemporal.failStart = true
	if _, err := svc.Attendance().CreateLeaveRequest(ctx, input); err == nil {
		t.Fatal("expected direct leave Temporal start failure")
	}
	instances, err := store.ListFormInstances(t.Context(), "tenant-1")
	if err != nil || len(instances) != 1 {
		t.Fatalf("expected one compensated form, count=%d err=%v", len(instances), err)
	}
	failedForm := instances[0]
	if failedForm.Status != domain.WorkflowFormStatusWorkflowStartFailed {
		t.Fatalf("expected start-failed form, got %+v", failedForm)
	}
	failedRun, ok, err := store.GetWorkflowRunByFormInstance(t.Context(), "tenant-1", failedForm.ID)
	if err != nil || !ok || failedRun.Status != domain.WorkflowRunStatusStartFailed {
		t.Fatalf("expected start-failed run, ok=%v err=%v run=%+v", ok, err, failedRun)
	}
	failedRequest, ok, err := store.GetLeaveRequestByFormInstanceID(t.Context(), "tenant-1", failedForm.ID)
	if err != nil || !ok || failedRequest.Status != "cancelled" {
		t.Fatalf("expected cancelled failed leave projection, ok=%v err=%v request=%+v", ok, err, failedRequest)
	}
	balance := effectiveLeaveBalanceForTest(t, store, "lb-direct-retry")
	if balance.RemainingHours != 16 {
		t.Fatalf("expected failed start to release its overlay reservation, balance=%+v", balance)
	}

	fakeTemporal.failStart = false
	retried, err := svc.Attendance().CreateLeaveRequest(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if retried.Status != "pending_approval" || retried.ID == failedRequest.ID {
		t.Fatalf("expected independent pending retry, failed=%+v retried=%+v", failedRequest, retried)
	}
	balance = effectiveLeaveBalanceForTest(t, store, "lb-direct-retry")
	if balance.RemainingHours != 16-retried.Hours || balance.PendingHours != retried.Hours {
		t.Fatalf("expected exactly one active overlay reservation after retry, balance=%+v retry=%+v", balance, retried)
	}
	failedRequest, ok, err = store.GetLeaveRequestByFormInstanceID(t.Context(), "tenant-1", failedForm.ID)
	if err != nil || !ok || failedRequest.Status != "cancelled" {
		t.Fatalf("expected failed projection to remain cancelled, ok=%v err=%v request=%+v", ok, err, failedRequest)
	}
}

// TestDirectOvertimeStartFailureCancelsProjectionBeforeRetry verifies failed starts never remain actionable.
func TestDirectOvertimeStartFailureCancelsProjectionBeforeRetry(t *testing.T) {
	now := time.Date(2026, 7, 15, 8, 0, 0, 0, time.UTC)
	svc, ctx, store, fakeTemporal := newWorkflowEngineFixtureWithFake(t, now, "acct-admin")
	grantDirectAttendanceCreatePermission(t, store)
	if err := store.UpsertFormTemplate(t.Context(), domain.FormTemplate{
		ID: "ft-overtime", TenantID: "tenant-1", Key: "overtime-approval", Name: "Overtime",
		Schema: workflowEnabledTemplateSchema("acct-admin"), CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	input := domain.CreateOvertimeRequestInput{
		StartAt: "2026-07-16T18:00:00+08:00", EndAt: "2026-07-16T21:00:00+08:00", Hours: 3,
		OvertimeType: "weekday", CompensationType: "leave", Reason: "temporal retry compensation",
	}

	fakeTemporal.failStart = true
	if _, err := svc.Attendance().CreateOvertimeRequest(ctx, input); err == nil {
		t.Fatal("expected direct overtime Temporal start failure")
	}
	instances, err := store.ListFormInstances(t.Context(), "tenant-1")
	if err != nil || len(instances) != 1 {
		t.Fatalf("expected one compensated form, count=%d err=%v", len(instances), err)
	}
	failedForm := instances[0]
	if failedForm.Status != domain.WorkflowFormStatusWorkflowStartFailed {
		t.Fatalf("expected start-failed form, got %+v", failedForm)
	}
	failedRun, ok, err := store.GetWorkflowRunByFormInstance(t.Context(), "tenant-1", failedForm.ID)
	if err != nil || !ok || failedRun.Status != domain.WorkflowRunStatusStartFailed {
		t.Fatalf("expected start-failed run, ok=%v err=%v run=%+v", ok, err, failedRun)
	}
	failedRequest, ok, err := store.GetOvertimeRequestByFormInstanceID(t.Context(), "tenant-1", failedForm.ID)
	if err != nil || !ok || failedRequest.Status != "cancelled" {
		t.Fatalf("expected cancelled failed overtime projection, ok=%v err=%v request=%+v", ok, err, failedRequest)
	}

	fakeTemporal.failStart = false
	retried, err := svc.Attendance().CreateOvertimeRequest(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if retried.Status != "pending_approval" || retried.ID == failedRequest.ID {
		t.Fatalf("expected independent pending retry, failed=%+v retried=%+v", failedRequest, retried)
	}
	failedRequest, ok, err = store.GetOvertimeRequestByFormInstanceID(t.Context(), "tenant-1", failedForm.ID)
	if err != nil || !ok || failedRequest.Status != "cancelled" {
		t.Fatalf("expected failed projection to remain cancelled, ok=%v err=%v request=%+v", ok, err, failedRequest)
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

// TestWorkflowDuplicateCreatesIndependentLeaveRequest verifies copied drafts retain fields without server links.
func TestWorkflowDuplicateCreatesIndependentLeaveRequest(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	svc, ctx, store := newWorkflowEngineFixture(t, now, "acct-admin")
	source, err := svc.Workflow().SubmitForm(ctx, domain.SubmitFormInput{
		TemplateKey: "leave-request",
		Payload: map[string]any{
			"leave_type":         "annual",
			"start_at":           "2026-06-11T09:00:00+08:00",
			"end_at":             "2026-06-11T17:00:00+08:00",
			"hours":              7,
			"reason":             "independent duplicate",
			"notify_account_ids": []any{"acct-admin"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	oldRequest, ok, err := store.GetLeaveRequestByFormInstanceID(t.Context(), "tenant-1", source.ID)
	if err != nil || !ok {
		t.Fatalf("source leave request lookup failed ok=%v err=%v", ok, err)
	}
	for _, key := range []string{"leave_request_id", "linked_resource_id", "linked_resource_type"} {
		if source.Payload[key] == nil {
			t.Fatalf("expected source payload to contain server link %q, got %+v", key, source.Payload)
		}
	}
	source.Payload["_review"] = map[string]any{"type": "approve", "account_id": "acct-admin"}
	source.Payload["overtime_request_id"] = "ot-stale"
	source.Payload["workflow_status"] = "approved"
	source.UpdatedAt = now.Add(90 * time.Minute)
	if err := store.UpsertFormInstance(t.Context(), source); err != nil {
		t.Fatal(err)
	}

	template, ok, err := store.GetFormTemplate(t.Context(), "tenant-1", source.TemplateID)
	if err != nil || !ok {
		t.Fatalf("template lookup failed ok=%v err=%v", ok, err)
	}
	template.CurrentVersion = 2
	template.UpdatedAt = now.Add(2 * time.Hour)
	if err := store.UpsertFormTemplate(t.Context(), template); err != nil {
		t.Fatal(err)
	}
	currentVersion, ok, err := store.GetFormTemplateVersionByNumber(t.Context(), "tenant-1", template.ID, 2)
	if err != nil || !ok {
		t.Fatalf("updated template version lookup failed ok=%v err=%v", ok, err)
	}
	if currentVersion.ID == source.TemplateVersionID {
		t.Fatalf("expected template update to create a distinct version, got %q", currentVersion.ID)
	}

	duplicate, err := svc.Workflow().DuplicateForm(ctx, source.ID)
	if err != nil {
		t.Fatal(err)
	}
	if duplicate.TemplateVersionID != source.TemplateVersionID {
		t.Fatalf("expected duplicate to preserve frozen template version, source=%q duplicate=%q", source.TemplateVersionID, duplicate.TemplateVersionID)
	}
	if duplicate.Payload["reason"] != source.Payload["reason"] || duplicate.Payload["notify_account_ids"] == nil {
		t.Fatalf("expected duplicate to retain form fields and notification metadata, got %+v", duplicate.Payload)
	}
	for _, key := range []string{"leave_request_id", "linked_resource_id", "linked_resource_type", "employee_id", "_review", "overtime_request_id", "workflow_status"} {
		if _, exists := duplicate.Payload[key]; exists {
			t.Fatalf("expected duplicate to omit server-managed payload key %q, got %+v", key, duplicate.Payload)
		}
	}

	submittedDuplicate, err := svc.Workflow().SubmitForm(ctx, domain.SubmitFormInput{TemplateKey: duplicate.ID})
	if err != nil {
		t.Fatal(err)
	}
	newRequest, ok, err := store.GetLeaveRequestByFormInstanceID(t.Context(), "tenant-1", duplicate.ID)
	if err != nil || !ok {
		t.Fatalf("duplicate leave request lookup failed ok=%v err=%v", ok, err)
	}
	if newRequest.ID == oldRequest.ID || newRequest.FormInstanceID != duplicate.ID {
		t.Fatalf("expected an independent leave request, old=%+v new=%+v", oldRequest, newRequest)
	}
	if submittedDuplicate.Payload["leave_request_id"] != newRequest.ID || submittedDuplicate.Payload["linked_resource_id"] != newRequest.ID {
		t.Fatalf("expected submitted duplicate to link the new leave request, got %+v", submittedDuplicate.Payload)
	}
	oldRequestAfter, ok, err := store.GetLeaveRequest(t.Context(), "tenant-1", oldRequest.ID)
	if err != nil || !ok {
		t.Fatalf("source leave request reload failed ok=%v err=%v", ok, err)
	}
	if !reflect.DeepEqual(oldRequestAfter, oldRequest) {
		t.Fatalf("source leave request changed during duplicate submit, before=%+v after=%+v", oldRequest, oldRequestAfter)
	}
}

// TestWorkflowDuplicatePreservesLegacyPayloadFields protects schemaless user data while removing server metadata.
func TestWorkflowDuplicatePreservesLegacyPayloadFields(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	svc, ctx, store := newWorkflowEngineFixture(t, now, "acct-admin")
	source, err := svc.Workflow().SubmitForm(ctx, domain.SubmitFormInput{
		TemplateKey: "leave-request",
		Payload: map[string]any{
			"legacy_custom_text":    "keep me",
			"legacy_nested":         map[string]any{"choice": "A"},
			"custom_request_id":     "purchase-42",
			"custom_request_status": "quoted",
			"notify_account_ids":    []any{"acct-admin"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	source.Payload["leave_request_id"] = "lr-stale"
	source.Payload["leave_request_status"] = "approved"
	source.Payload["linked_resource_id"] = "lr-stale"
	source.Payload["linked_resource_type"] = "attendance.leave_request"
	source.Payload["employee_id"] = "emp-stale"
	source.Payload["_review"] = map[string]any{"type": "approve"}
	source.Payload["workflow_status"] = "approved"
	if err := store.UpsertFormInstance(t.Context(), source); err != nil {
		t.Fatal(err)
	}

	duplicate, err := svc.Workflow().DuplicateForm(ctx, source.ID)
	if err != nil {
		t.Fatal(err)
	}
	if duplicate.Payload["legacy_custom_text"] != "keep me" || !reflect.DeepEqual(duplicate.Payload["legacy_nested"], source.Payload["legacy_nested"]) {
		t.Fatalf("expected legacy user payload to survive duplication, got %+v", duplicate.Payload)
	}
	if duplicate.Payload["custom_request_id"] != "purchase-42" || duplicate.Payload["custom_request_status"] != "quoted" {
		t.Fatalf("expected user-defined request fields to survive duplication, got %+v", duplicate.Payload)
	}
	if duplicate.Payload["notify_account_ids"] == nil {
		t.Fatalf("expected notification metadata to survive duplication, got %+v", duplicate.Payload)
	}
	for _, key := range []string{"leave_request_id", "leave_request_status", "linked_resource_id", "linked_resource_type", "employee_id", "_review", "workflow_status"} {
		if _, exists := duplicate.Payload[key]; exists {
			t.Fatalf("expected legacy duplicate to omit server-owned key %q, got %+v", key, duplicate.Payload)
		}
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

// TestWorkflowAssignedReviewerWithSelfReadCanReview closes the assigned-to-me queue and action path.
func TestWorkflowAssignedReviewerWithSelfReadCanReview(t *testing.T) {
	now := time.Date(2026, 7, 15, 8, 0, 0, 0, time.UTC)
	svc, applicantCtx, store, _ := newWorkflowEngineFixtureWithFake(t, now, "acct-reviewer")
	if err := store.UpsertPermissionSet(t.Context(), domain.PermissionSet{
		ID: "ps-workflow-assigned", TenantID: "tenant-1", Name: "Assigned reviewer baseline",
		Permissions: []domain.Permission{{Resource: "workflow.form_instance", Action: "read", Scope: "self"}},
		CreatedAt:   now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertEmployee(t.Context(), domain.Employee{
		ID: "emp-reviewer", TenantID: "tenant-1", Name: "Assigned Reviewer", AccountID: "acct-reviewer",
		Status: "active", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	reviewer, ok, err := store.GetAccount(t.Context(), "tenant-1", "acct-reviewer")
	if err != nil || !ok {
		t.Fatalf("reviewer lookup failed ok=%v err=%v", ok, err)
	}
	reviewer.EmployeeID = "emp-reviewer"
	reviewer.DirectPermissionSetIDs = []string{"ps-workflow-assigned"}
	if err := store.UpsertAccount(t.Context(), reviewer); err != nil {
		t.Fatal(err)
	}

	instance, err := svc.Workflow().SubmitForm(applicantCtx, domain.SubmitFormInput{
		TemplateKey: "leave-request",
		Payload:     map[string]any{"desc": "assigned review"},
	})
	if err != nil {
		t.Fatal(err)
	}
	reviewerCtx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-reviewer"}
	queue, err := svc.Workflow().ReviewQueue(reviewerCtx)
	if err != nil {
		t.Fatal(err)
	}
	if len(queue.PendingReview) != 1 || queue.PendingReview[0].ID != instance.ID {
		t.Fatalf("expected assigned form in pending queue, got %+v", queue.PendingReview)
	}
	if _, err := svc.Workflow().GetWorkflowFormState(reviewerCtx, instance.ID); err != nil {
		t.Fatalf("expected active assignee to read workflow state: %v", err)
	}
	approved, err := svc.Workflow().ApproveForm(reviewerCtx, instance.ID, domain.ApproveFormInput{Reason: "assigned"})
	if err != nil {
		t.Fatal(err)
	}
	if approved.Status != "approved" {
		t.Fatalf("expected assigned reviewer approval, got %+v", approved)
	}
}

// TestWorkflowSelfReaderCannotReviewUnassignedForm preserves the active-assignee enforcement boundary.
func TestWorkflowSelfReaderCannotReviewUnassignedForm(t *testing.T) {
	now := time.Date(2026, 7, 15, 8, 0, 0, 0, time.UTC)
	svc, applicantCtx, store, fakeTemporal := newWorkflowEngineFixtureWithFake(t, now, "acct-reviewer")
	if err := store.UpsertPermissionSet(t.Context(), domain.PermissionSet{
		ID: "ps-workflow-self-reader", TenantID: "tenant-1", Name: "Self reader",
		Permissions: []domain.Permission{{Resource: "workflow.form_instance", Action: "read", Scope: "self"}},
		CreatedAt:   now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertEmployee(t.Context(), domain.Employee{
		ID: "emp-bystander", TenantID: "tenant-1", Name: "Bystander", AccountID: "acct-bystander",
		Status: "active", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertAccount(t.Context(), domain.Account{
		ID: "acct-bystander", TenantID: "tenant-1", DisplayName: "Bystander", EmployeeID: "emp-bystander",
		Status: "active", DirectPermissionSetIDs: []string{"ps-workflow-self-reader"}, CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	instance, err := svc.Workflow().SubmitForm(applicantCtx, domain.SubmitFormInput{
		TemplateKey: "leave-request",
		Payload:     map[string]any{"desc": "not assigned"},
	})
	if err != nil {
		t.Fatal(err)
	}
	bystanderCtx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-bystander"}
	queue, err := svc.Workflow().ReviewQueue(bystanderCtx)
	if err != nil {
		t.Fatal(err)
	}
	if len(queue.PendingReview) != 0 {
		t.Fatalf("expected no unassigned pending forms, got %+v", queue.PendingReview)
	}
	_, err = svc.Workflow().ApproveForm(bystanderCtx, instance.ID, domain.ApproveFormInput{Reason: "not assigned"})
	var appErr *domain.AppError
	if !errors.As(err, &appErr) || appErr.Status != 403 || appErr.ReasonCode != "workflow_not_assignee" {
		t.Fatalf("expected active-assignee denial, got %T %v", err, err)
	}
	if len(fakeTemporal.signals) != 0 {
		t.Fatalf("expected denial before Temporal signal, got %+v", fakeTemporal.signals)
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

// TestSubmitFormMarksWorkflowStartFailedWhenTemporalUnavailable verifies standard leave submissions restore reservations.
func TestSubmitFormMarksWorkflowStartFailedWhenTemporalUnavailable(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	svc, applicantCtx, store, fakeTemporal := newWorkflowEngineFixtureWithFake(t, now, "acct-admin")
	if err := store.UpsertLeaveBalance(t.Context(), domain.LeaveBalance{
		ID: "lb-standard-retry", TenantID: "tenant-1", EmployeeID: "emp-applicant", LeaveType: "annual",
		RemainingHours: 16, GrantedHours: 16, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	input := domain.SubmitFormInput{
		TemplateKey: "leave-request",
		Payload: map[string]any{
			"leave_type": "annual",
			"start_at":   "2026-06-11T09:00:00+08:00",
			"end_at":     "2026-06-11T17:00:00+08:00",
			"hours":      8,
			"reason":     "temporal down",
		},
	}
	fakeTemporal.failStart = true

	_, err := svc.Workflow().SubmitForm(applicantCtx, input)
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
	failedRequest, ok, requestErr := store.GetLeaveRequestByFormInstanceID(t.Context(), "tenant-1", instances[0].ID)
	if requestErr != nil || !ok || failedRequest.Status != "cancelled" {
		t.Fatalf("expected cancelled leave projection, got ok=%v err=%v request=%+v", ok, requestErr, failedRequest)
	}
	balance := effectiveLeaveBalanceForTest(t, store, "lb-standard-retry")
	if balance.RemainingHours != 16 {
		t.Fatalf("expected failed start to release its overlay reservation, balance=%+v", balance)
	}

	fakeTemporal.failStart = false
	retried, err := svc.Workflow().SubmitForm(applicantCtx, input)
	if err != nil {
		t.Fatal(err)
	}
	retriedRequest, ok, requestErr := store.GetLeaveRequestByFormInstanceID(t.Context(), "tenant-1", retried.ID)
	if requestErr != nil || !ok || retriedRequest.Status != "pending_approval" || retriedRequest.ID == failedRequest.ID {
		t.Fatalf("expected independent pending retry, got ok=%v err=%v failed=%+v retried=%+v", ok, requestErr, failedRequest, retriedRequest)
	}
	balance = effectiveLeaveBalanceForTest(t, store, "lb-standard-retry")
	if balance.RemainingHours != 16-retriedRequest.Hours || balance.PendingHours != retriedRequest.Hours {
		t.Fatalf("expected one active overlay reservation after retry, balance=%+v", balance)
	}
	failedRequest, ok, requestErr = store.GetLeaveRequestByFormInstanceID(t.Context(), "tenant-1", instances[0].ID)
	if requestErr != nil || !ok || failedRequest.Status != "cancelled" {
		t.Fatalf("expected failed leave projection to remain cancelled, got ok=%v err=%v request=%+v", ok, requestErr, failedRequest)
	}
}

// TestSubmitOvertimeStartFailureCancelsProjectionBeforeRetry verifies standard overtime failures are not actionable.
func TestSubmitOvertimeStartFailureCancelsProjectionBeforeRetry(t *testing.T) {
	now := time.Date(2026, 7, 15, 8, 0, 0, 0, time.UTC)
	svc, applicantCtx, store, fakeTemporal := newWorkflowEngineFixtureWithFake(t, now, "acct-admin")
	if err := store.UpsertFormTemplate(t.Context(), domain.FormTemplate{
		ID: "ft-standard-overtime", TenantID: "tenant-1", Key: "overtime-approval", Name: "Overtime",
		Schema: workflowEnabledTemplateSchema("acct-admin"), CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	input := domain.SubmitFormInput{
		TemplateKey: "overtime-approval",
		Payload: map[string]any{
			"start_at":          "2026-07-16T18:00:00+08:00",
			"end_at":            "2026-07-16T21:00:00+08:00",
			"hours":             3,
			"overtime_type":     "weekday",
			"compensation_type": "leave",
			"reason":            "temporal retry compensation",
		},
	}
	fakeTemporal.failStart = true

	if _, err := svc.Workflow().SubmitForm(applicantCtx, input); err == nil {
		t.Fatal("expected standard overtime Temporal start failure")
	}
	instances, err := store.ListFormInstances(t.Context(), "tenant-1")
	if err != nil || len(instances) != 1 {
		t.Fatalf("expected one compensated form, count=%d err=%v", len(instances), err)
	}
	failedRequest, ok, requestErr := store.GetOvertimeRequestByFormInstanceID(t.Context(), "tenant-1", instances[0].ID)
	if requestErr != nil || !ok || failedRequest.Status != "cancelled" {
		t.Fatalf("expected cancelled overtime projection, got ok=%v err=%v request=%+v", ok, requestErr, failedRequest)
	}

	fakeTemporal.failStart = false
	retried, err := svc.Workflow().SubmitForm(applicantCtx, input)
	if err != nil {
		t.Fatal(err)
	}
	retriedRequest, ok, requestErr := store.GetOvertimeRequestByFormInstanceID(t.Context(), "tenant-1", retried.ID)
	if requestErr != nil || !ok || retriedRequest.Status != "pending_approval" || retriedRequest.ID == failedRequest.ID {
		t.Fatalf("expected independent pending retry, got ok=%v err=%v failed=%+v retried=%+v", ok, requestErr, failedRequest, retriedRequest)
	}
	failedRequest, ok, requestErr = store.GetOvertimeRequestByFormInstanceID(t.Context(), "tenant-1", instances[0].ID)
	if requestErr != nil || !ok || failedRequest.Status != "cancelled" {
		t.Fatalf("expected failed overtime projection to remain cancelled, got ok=%v err=%v request=%+v", ok, requestErr, failedRequest)
	}
}
