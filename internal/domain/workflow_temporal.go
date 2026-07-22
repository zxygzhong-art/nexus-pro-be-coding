package domain

import (
	"errors"
	"strings"
	"time"
)

const (
	FormApprovalWorkflowSignalName      = "form_approval_signal"
	WorkflowStartRequestedEventType     = "workflow.form_approval.start_requested"
	WorkflowStartAggregateType          = "workflow_run"
	DefaultFormApprovalRemindAfterHours = 72
	FormApprovalWorkflowActionSubmit    = "submit"
	FormApprovalWorkflowActionApprove   = "approve"
	FormApprovalWorkflowActionReject    = "reject"
	FormApprovalWorkflowActionReturn    = "return"
	FormApprovalWorkflowActionWithdraw  = "withdraw"
)

var ErrFormApprovalWorkflowNotFound = errors.New("form approval workflow not found")

// FormApprovalWorkflowStart is the deterministic input used to start one form approval workflow.
type FormApprovalWorkflowStart struct {
	TenantID                string `json:"tenant_id"`
	FormInstanceID          string `json:"form_instance_id"`
	RunID                   string `json:"run_id,omitempty"`
	WorkflowID              string `json:"workflow_id,omitempty"`
	StageDefinitionsJSON    string `json:"stage_definitions_json,omitempty"`
	DefaultRemindAfterHours int    `json:"default_remind_after_hours,omitempty"`
}

// FormApprovalWorkflowSignal carries review actions into a running workflow.
type FormApprovalWorkflowSignal struct {
	TenantID           string `json:"tenant_id"`
	FormInstanceID     string `json:"form_instance_id"`
	RunID              string `json:"run_id,omitempty"`
	WorkflowID         string `json:"workflow_id,omitempty"`
	AccountID          string `json:"account_id,omitempty"`
	Action             string `json:"action"`
	Reason             string `json:"reason,omitempty"`
	RequestID          string `json:"request_id,omitempty"`
	TraceID            string `json:"trace_id,omitempty"`
	IdempotencyKey     string `json:"idempotency_key,omitempty"`
	CommandFingerprint string `json:"command_fingerprint,omitempty"`
}

// FormApprovalWorkflowExecution is the identity returned after a durable ensure operation.
type FormApprovalWorkflowExecution struct {
	WorkflowID string `json:"workflow_id"`
	RunID      string `json:"run_id,omitempty"`
}

// WorkflowStartRequestedPayload is the versioned local outbox payload.
type WorkflowStartRequestedPayload struct {
	RunID          string `json:"run_id"`
	FormInstanceID string `json:"form_instance_id"`
	WorkflowID     string `json:"temporal_workflow_id"`
}

// FormApprovalProjection describes the latest query projection after an activity updates tables.
type FormApprovalProjection struct {
	TenantID               string    `json:"tenant_id"`
	FormInstanceID         string    `json:"form_instance_id"`
	RunID                  string    `json:"run_id,omitempty"`
	FormStatus             string    `json:"form_status,omitempty"`
	RunStatus              string    `json:"run_status,omitempty"`
	CurrentStageID         string    `json:"current_stage_id,omitempty"`
	CurrentStageInstanceID string    `json:"current_stage_instance_id,omitempty"`
	CurrentStageLabel      string    `json:"current_stage_label,omitempty"`
	RemindAfterHours       int       `json:"remind_after_hours,omitempty"`
	UpdatedAt              time.Time `json:"updated_at,omitempty"`
}

// FormApprovalReminder asks an activity to notify pending approvers for the current stage.
type FormApprovalReminder struct {
	TenantID               string `json:"tenant_id"`
	FormInstanceID         string `json:"form_instance_id"`
	RunID                  string `json:"run_id,omitempty"`
	CurrentStageID         string `json:"current_stage_id,omitempty"`
	CurrentStageInstanceID string `json:"current_stage_instance_id,omitempty"`
	CurrentStageLabel      string `json:"current_stage_label,omitempty"`
}

// FormApprovalWorkflowID returns the ADR-defined cross-tenant-safe workflow ID.
func FormApprovalWorkflowID(tenantID, formInstanceID string) string {
	return strings.TrimSpace(tenantID) + ":" + strings.TrimSpace(formInstanceID)
}

// FormApprovalWorkflowIDForRun isolates resubmissions while preserving legacy IDs.
func FormApprovalWorkflowIDForRun(tenantID, formInstanceID, runID string) string {
	base := FormApprovalWorkflowID(tenantID, formInstanceID)
	if strings.TrimSpace(runID) == "" {
		return base
	}
	return base + ":" + strings.TrimSpace(runID)
}

// ResolveFormApprovalWorkflowID prefers the persisted identity over derived fallbacks.
func ResolveFormApprovalWorkflowID(workflowID, tenantID, formInstanceID, runID string) string {
	if resolved := strings.TrimSpace(workflowID); resolved != "" {
		return resolved
	}
	return FormApprovalWorkflowIDForRun(tenantID, formInstanceID, runID)
}

// ValidateFormApprovalWorkflowStart rejects workflow inputs that cannot load a projection.
func ValidateFormApprovalWorkflowStart(input FormApprovalWorkflowStart) error {
	if strings.TrimSpace(input.TenantID) == "" {
		return BadRequest("tenant_id is required")
	}
	if strings.TrimSpace(input.FormInstanceID) == "" {
		return BadRequest("form_instance_id is required")
	}
	return nil
}
