package domain

import (
	"errors"
	"strings"
	"time"
)

const (
	FormApprovalWorkflowSignalName      = "form_approval_signal"
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
	StageDefinitionsJSON    string `json:"stage_definitions_json,omitempty"`
	DefaultRemindAfterHours int    `json:"default_remind_after_hours,omitempty"`
}

// FormApprovalWorkflowSignal carries review actions into a running workflow.
type FormApprovalWorkflowSignal struct {
	TenantID       string `json:"tenant_id"`
	FormInstanceID string `json:"form_instance_id"`
	AccountID      string `json:"account_id,omitempty"`
	Action         string `json:"action"`
	Reason         string `json:"reason,omitempty"`
	RequestID      string `json:"request_id,omitempty"`
	TraceID        string `json:"trace_id,omitempty"`
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
