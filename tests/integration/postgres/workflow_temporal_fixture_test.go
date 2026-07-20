package postgres_integration_test

import (
	"context"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/repository"
	"nexus-pro-api/internal/service"
)

type integrationFormApprovalWorkflowClient struct {
	service *service.Service
	started map[string]domain.FormApprovalWorkflowStart
}

// newIntegrationServiceWithFormApprovalWorkflows keeps Postgres tests synchronous while preserving Temporal fail-closed production behavior.
func newIntegrationServiceWithFormApprovalWorkflows(store repository.Store, options service.Options) *service.Service {
	fake := &integrationFormApprovalWorkflowClient{started: map[string]domain.FormApprovalWorkflowStart{}}
	options.FormApprovalWorkflows = fake
	svc := service.New(store, options)
	fake.service = svc
	return svc
}

// StartFormApprovalWorkflow records the deterministic workflow identity used by later test signals.
func (c *integrationFormApprovalWorkflowClient) StartFormApprovalWorkflow(_ context.Context, start domain.FormApprovalWorkflowStart) error {
	c.started[domain.FormApprovalWorkflowID(start.TenantID, start.FormInstanceID)] = start
	return nil
}

// SignalFormApprovalWorkflow applies the same projection update that a Temporal worker activity would persist.
func (c *integrationFormApprovalWorkflowClient) SignalFormApprovalWorkflow(ctx context.Context, signal domain.FormApprovalWorkflowSignal) error {
	workflowID := domain.FormApprovalWorkflowID(signal.TenantID, signal.FormInstanceID)
	if _, ok := c.started[workflowID]; !ok {
		return domain.ErrFormApprovalWorkflowNotFound
	}
	_, err := c.service.Workflow().ApplyTemporalFormApprovalSignal(domain.RequestContext{
		Context:   ctx,
		TenantID:  signal.TenantID,
		AccountID: signal.AccountID,
		RequestID: signal.RequestID,
		TraceID:   signal.TraceID,
	}, signal)
	return err
}
