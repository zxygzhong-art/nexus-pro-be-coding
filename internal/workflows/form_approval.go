package workflows

import (
	"context"
	"strings"
	"time"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/service"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"go.temporal.io/sdk/workflow"
)

const (
	formApprovalVersionChangeID = "form-approval-workflow-v1"

	ActivityNameLoadFormApprovalProjection = "LoadFormApprovalProjection"
	ActivityNameApplyFormApprovalSignal    = "ApplyFormApprovalSignal"
	ActivityNameRecordFormApprovalReminder = "RecordFormApprovalReminder"
)

// FormApprovalWorkflow is the Temporal authority for a submitted form approval.
func FormApprovalWorkflow(ctx workflow.Context, input domain.FormApprovalWorkflowStart) error {
	if err := domain.ValidateFormApprovalWorkflowStart(input); err != nil {
		return err
	}
	workflow.GetVersion(ctx, formApprovalVersionChangeID, workflow.DefaultVersion, 1)
	ctx = workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		StartToCloseTimeout: 30 * time.Second,
	})

	var projection domain.FormApprovalProjection
	if err := workflow.ExecuteActivity(ctx, ActivityNameLoadFormApprovalProjection, input).Get(ctx, &projection); err != nil {
		return err
	}

	signalCh := workflow.GetSignalChannel(ctx, domain.FormApprovalWorkflowSignalName)
	remindedStages := map[string]bool{}
	for {
		if formApprovalProjectionTerminal(projection) {
			return nil
		}

		stageKey := projection.CurrentStageInstanceID
		if stageKey == "" {
			stageKey = projection.CurrentStageID
		}
		if stageKey == "" || remindedStages[stageKey] {
			var signal domain.FormApprovalWorkflowSignal
			signalCh.Receive(ctx, &signal)
			next, err := applyFormApprovalSignal(ctx, projection, signal)
			if err != nil {
				return err
			}
			projection = next
			continue
		}

		var signal domain.FormApprovalWorkflowSignal
		gotSignal := false
		timerFired := false
		selector := workflow.NewSelector(ctx)
		selector.AddReceive(signalCh, func(c workflow.ReceiveChannel, _ bool) {
			c.Receive(ctx, &signal)
			gotSignal = true
		})
		selector.AddFuture(workflow.NewTimer(ctx, formApprovalReminderDuration(input, projection)), func(f workflow.Future) {
			_ = f.Get(ctx, nil)
			timerFired = true
		})
		selector.Select(ctx)

		if gotSignal {
			next, err := applyFormApprovalSignal(ctx, projection, signal)
			if err != nil {
				return err
			}
			projection = next
			continue
		}
		if timerFired {
			reminder := domain.FormApprovalReminder{
				TenantID:               projection.TenantID,
				FormInstanceID:         projection.FormInstanceID,
				RunID:                  projection.RunID,
				CurrentStageID:         projection.CurrentStageID,
				CurrentStageInstanceID: projection.CurrentStageInstanceID,
				CurrentStageLabel:      projection.CurrentStageLabel,
			}
			if err := workflow.ExecuteActivity(ctx, ActivityNameRecordFormApprovalReminder, reminder).Get(ctx, nil); err != nil {
				return err
			}
			remindedStages[stageKey] = true
			if err := workflow.ExecuteActivity(ctx, ActivityNameLoadFormApprovalProjection, input).Get(ctx, &projection); err != nil {
				return err
			}
		}
	}
}

// Activities contains activity implementations that delegate side effects to services.
type Activities struct {
	Service *service.Service
}

// LoadFormApprovalProjection reads the current form approval projection.
func (a *Activities) LoadFormApprovalProjection(ctx context.Context, input domain.FormApprovalWorkflowStart) (projection domain.FormApprovalProjection, err error) {
	ctx, span := workflowActivitySpan(ctx, "temporal.activity.workflow.form.load_projection", input.TenantID, input.FormInstanceID)
	defer finishWorkflowActivitySpan(span, err)
	if a == nil || a.Service == nil {
		return domain.FormApprovalProjection{}, nil
	}
	projection, err = a.Service.Workflow().LoadTemporalFormApprovalProjection(domain.RequestContext{
		Context:  ctx,
		TenantID: input.TenantID,
	}, input.FormInstanceID)
	return projection, nonRetryableActivityError(err)
}

// ApplyFormApprovalSignal applies an approve/reject/return/withdraw signal to the read model.
func (a *Activities) ApplyFormApprovalSignal(ctx context.Context, signal domain.FormApprovalWorkflowSignal) (projection domain.FormApprovalProjection, err error) {
	ctx, span := workflowActivitySpan(ctx, "temporal.activity.workflow.form.apply_signal", signal.TenantID, signal.FormInstanceID)
	defer finishWorkflowActivitySpan(span, err)
	if a == nil || a.Service == nil {
		return domain.FormApprovalProjection{}, nil
	}
	projection, err = a.Service.Workflow().ApplyTemporalFormApprovalSignal(domain.RequestContext{
		Context:   ctx,
		TenantID:  signal.TenantID,
		AccountID: signal.AccountID,
		RequestID: signal.RequestID,
		TraceID:   signal.TraceID,
	}, signal)
	return projection, nonRetryableActivityError(err)
}

// RecordFormApprovalReminder writes a reminder notification and audit log.
func (a *Activities) RecordFormApprovalReminder(ctx context.Context, reminder domain.FormApprovalReminder) (err error) {
	ctx, span := workflowActivitySpan(ctx, "temporal.activity.workflow.form.reminder", reminder.TenantID, reminder.FormInstanceID)
	defer finishWorkflowActivitySpan(span, err)
	if a == nil || a.Service == nil {
		return nil
	}
	err = a.Service.Workflow().RecordTemporalFormApprovalReminder(domain.RequestContext{
		Context:   ctx,
		TenantID:  reminder.TenantID,
		AccountID: "system",
	}, reminder)
	return nonRetryableActivityError(err)
}

func applyFormApprovalSignal(ctx workflow.Context, current domain.FormApprovalProjection, signal domain.FormApprovalWorkflowSignal) (domain.FormApprovalProjection, error) {
	switch strings.TrimSpace(strings.ToLower(signal.Action)) {
	case "", domain.FormApprovalWorkflowActionSubmit:
		return current, nil
	case domain.FormApprovalWorkflowActionApprove, domain.FormApprovalWorkflowActionReject, domain.FormApprovalWorkflowActionReturn, domain.FormApprovalWorkflowActionWithdraw:
		var projection domain.FormApprovalProjection
		if err := workflow.ExecuteActivity(ctx, ActivityNameApplyFormApprovalSignal, signal).Get(ctx, &projection); err != nil {
			return domain.FormApprovalProjection{}, err
		}
		return projection, nil
	default:
		return current, nil
	}
}

func formApprovalProjectionTerminal(projection domain.FormApprovalProjection) bool {
	switch strings.TrimSpace(strings.ToLower(projection.FormStatus)) {
	case "approved", "rejected", "returned", "cancelled", "canceled":
		return true
	}
	switch strings.TrimSpace(strings.ToLower(projection.RunStatus)) {
	case domain.WorkflowRunStatusCompleted, domain.WorkflowRunStatusReturned, domain.WorkflowRunStatusCancelled:
		return true
	default:
		return false
	}
}

func formApprovalReminderDuration(input domain.FormApprovalWorkflowStart, projection domain.FormApprovalProjection) time.Duration {
	hours := projection.RemindAfterHours
	if hours <= 0 {
		hours = input.DefaultRemindAfterHours
	}
	if hours <= 0 {
		hours = domain.DefaultFormApprovalRemindAfterHours
	}
	return time.Duration(hours) * time.Hour
}

func workflowActivitySpan(ctx context.Context, name, tenantID, formInstanceID string) (context.Context, trace.Span) {
	ctx, span := otel.Tracer("nexus-pro-be/internal/workflows").Start(ctx, name)
	span.SetAttributes(
		attribute.String("tenant_id", tenantID),
		attribute.String("form_instance_id", formInstanceID),
	)
	return ctx, span
}

func finishWorkflowActivitySpan(span trace.Span, err error) {
	if span == nil {
		return
	}
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	span.End()
}
