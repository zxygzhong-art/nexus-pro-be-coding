package workflows_test

import (
	"context"
	"testing"
	"time"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/workflows"

	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/testsuite"
)

func TestFormApprovalWorkflowApproveSignalCompletes(t *testing.T) {
	env, counters := newFormApprovalWorkflowEnv(t, formApprovalWorkflowScript{
		afterSignal: domain.FormApprovalProjection{TenantID: "tenant-1", FormInstanceID: "fi-1", FormStatus: "approved", RunStatus: domain.WorkflowRunStatusCompleted},
	})
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(domain.FormApprovalWorkflowSignalName, domain.FormApprovalWorkflowSignal{
			TenantID:       "tenant-1",
			FormInstanceID: "fi-1",
			AccountID:      "acct-admin",
			Action:         domain.FormApprovalWorkflowActionApprove,
		})
	}, time.Millisecond)

	env.ExecuteWorkflow(workflows.FormApprovalWorkflow, formApprovalStart())

	if err := env.GetWorkflowError(); err != nil {
		t.Fatal(err)
	}
	if !env.IsWorkflowCompleted() {
		t.Fatal("expected workflow to complete")
	}
	if counters.apply != 1 {
		t.Fatalf("expected projection activity to be called once, got %d", counters.apply)
	}
	if counters.reminder != 0 {
		t.Fatalf("reminder should not run on quick approval, got %d", counters.reminder)
	}
}

func TestFormApprovalWorkflowRejectsEmptyFormInstanceID(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	env.RegisterWorkflow(workflows.FormApprovalWorkflow)

	env.ExecuteWorkflow(workflows.FormApprovalWorkflow, domain.FormApprovalWorkflowStart{
		TenantID: "tenant-1",
	})

	if err := env.GetWorkflowError(); err == nil {
		t.Fatal("expected workflow to fail for empty form_instance_id")
	}
}

func TestFormApprovalWorkflowReminderTimerFires(t *testing.T) {
	env, counters := newFormApprovalWorkflowEnv(t, formApprovalWorkflowScript{
		initial:     activeFormApprovalProjection(1),
		afterLoad:   domain.FormApprovalProjection{TenantID: "tenant-1", FormInstanceID: "fi-1", FormStatus: "cancelled", RunStatus: domain.WorkflowRunStatusCancelled},
		afterSignal: domain.FormApprovalProjection{TenantID: "tenant-1", FormInstanceID: "fi-1", FormStatus: "cancelled", RunStatus: domain.WorkflowRunStatusCancelled},
	})

	env.ExecuteWorkflow(workflows.FormApprovalWorkflow, formApprovalStart())

	if err := env.GetWorkflowError(); err != nil {
		t.Fatal(err)
	}
	if counters.reminder != 1 {
		t.Fatalf("expected reminder activity once, got %d", counters.reminder)
	}
}

func TestFormApprovalWorkflowRejectSignalCompletes(t *testing.T) {
	env, counters := newFormApprovalWorkflowEnv(t, formApprovalWorkflowScript{
		afterSignal: domain.FormApprovalProjection{TenantID: "tenant-1", FormInstanceID: "fi-1", FormStatus: "rejected", RunStatus: domain.WorkflowRunStatusCompleted},
	})
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(domain.FormApprovalWorkflowSignalName, domain.FormApprovalWorkflowSignal{
			TenantID:       "tenant-1",
			FormInstanceID: "fi-1",
			AccountID:      "acct-admin",
			Action:         domain.FormApprovalWorkflowActionReject,
		})
	}, time.Millisecond)

	env.ExecuteWorkflow(workflows.FormApprovalWorkflow, formApprovalStart())

	if err := env.GetWorkflowError(); err != nil {
		t.Fatal(err)
	}
	if counters.apply != 1 {
		t.Fatalf("expected reject projection activity once, got %d", counters.apply)
	}
}

func TestFormApprovalWorkflowReturnSignalCompletes(t *testing.T) {
	env, counters := newFormApprovalWorkflowEnv(t, formApprovalWorkflowScript{
		afterSignal: domain.FormApprovalProjection{TenantID: "tenant-1", FormInstanceID: "fi-1", FormStatus: "returned", RunStatus: domain.WorkflowRunStatusReturned},
	})
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(domain.FormApprovalWorkflowSignalName, domain.FormApprovalWorkflowSignal{
			TenantID:       "tenant-1",
			FormInstanceID: "fi-1",
			AccountID:      "acct-admin",
			Action:         domain.FormApprovalWorkflowActionReturn,
		})
	}, time.Millisecond)

	env.ExecuteWorkflow(workflows.FormApprovalWorkflow, formApprovalStart())

	if err := env.GetWorkflowError(); err != nil {
		t.Fatal(err)
	}
	if counters.apply != 1 {
		t.Fatalf("expected return projection activity once, got %d", counters.apply)
	}
}

func TestFormApprovalWorkflowWithdrawSignalCompletes(t *testing.T) {
	env, counters := newFormApprovalWorkflowEnv(t, formApprovalWorkflowScript{
		afterSignal: domain.FormApprovalProjection{TenantID: "tenant-1", FormInstanceID: "fi-1", FormStatus: "cancelled", RunStatus: domain.WorkflowRunStatusCancelled},
	})
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(domain.FormApprovalWorkflowSignalName, domain.FormApprovalWorkflowSignal{
			TenantID:       "tenant-1",
			FormInstanceID: "fi-1",
			AccountID:      "acct-applicant",
			Action:         domain.FormApprovalWorkflowActionWithdraw,
		})
	}, time.Millisecond)

	env.ExecuteWorkflow(workflows.FormApprovalWorkflow, formApprovalStart())

	if err := env.GetWorkflowError(); err != nil {
		t.Fatal(err)
	}
	if counters.apply != 1 {
		t.Fatalf("expected withdraw projection activity once, got %d", counters.apply)
	}
}

type formApprovalWorkflowScript struct {
	initial     domain.FormApprovalProjection
	afterLoad   domain.FormApprovalProjection
	afterSignal domain.FormApprovalProjection
}

type formApprovalWorkflowCounters struct {
	load     int
	apply    int
	reminder int
}

func newFormApprovalWorkflowEnv(t *testing.T, script formApprovalWorkflowScript) (*testsuite.TestWorkflowEnvironment, *formApprovalWorkflowCounters) {
	t.Helper()
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	env.RegisterWorkflow(workflows.FormApprovalWorkflow)
	counters := &formApprovalWorkflowCounters{}
	if script.initial.FormInstanceID == "" {
		script.initial = activeFormApprovalProjection(72)
	}
	if script.afterSignal.FormInstanceID == "" {
		script.afterSignal = domain.FormApprovalProjection{TenantID: "tenant-1", FormInstanceID: "fi-1", FormStatus: "approved", RunStatus: domain.WorkflowRunStatusCompleted}
	}
	env.RegisterActivityWithOptions(func(context.Context, domain.FormApprovalWorkflowStart) (domain.FormApprovalProjection, error) {
		counters.load++
		if counters.reminder > 0 && script.afterLoad.FormInstanceID != "" {
			return script.afterLoad, nil
		}
		return script.initial, nil
	}, activity.RegisterOptions{Name: workflows.ActivityNameLoadFormApprovalProjection})
	env.RegisterActivityWithOptions(func(context.Context, domain.FormApprovalWorkflowSignal) (domain.FormApprovalProjection, error) {
		counters.apply++
		return script.afterSignal, nil
	}, activity.RegisterOptions{Name: workflows.ActivityNameApplyFormApprovalSignal})
	env.RegisterActivityWithOptions(func(context.Context, domain.FormApprovalReminder) error {
		counters.reminder++
		return nil
	}, activity.RegisterOptions{Name: workflows.ActivityNameRecordFormApprovalReminder})
	return env, counters
}

func formApprovalStart() domain.FormApprovalWorkflowStart {
	return domain.FormApprovalWorkflowStart{
		TenantID:                "tenant-1",
		FormInstanceID:          "fi-1",
		RunID:                   "wfr-1",
		DefaultRemindAfterHours: domain.DefaultFormApprovalRemindAfterHours,
	}
}

func activeFormApprovalProjection(remindAfterHours int) domain.FormApprovalProjection {
	return domain.FormApprovalProjection{
		TenantID:               "tenant-1",
		FormInstanceID:         "fi-1",
		RunID:                  "wfr-1",
		FormStatus:             "in_review",
		RunStatus:              domain.WorkflowRunStatusRunning,
		CurrentStageID:         "stage-1",
		CurrentStageInstanceID: "wfs-1",
		CurrentStageLabel:      "主管審核",
		RemindAfterHours:       remindAfterHours,
	}
}
