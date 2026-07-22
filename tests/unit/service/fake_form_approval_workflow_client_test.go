package service_test

import (
	"context"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/repository"
	"nexus-pro-api/internal/service"
)

type fakeFormApprovalWorkflowClient struct {
	service   *service.Service
	started   map[string]domain.FormApprovalWorkflowStart
	missing   map[string]bool
	starts    []domain.FormApprovalWorkflowStart
	signals   []domain.FormApprovalWorkflowSignal
	startErr  error
	failStart bool
	startSeen chan<- struct{}
	startGate <-chan struct{}
}

func newServiceWithFakeFormApprovalWorkflows(store repository.Store, options service.Options) (*service.Service, *fakeFormApprovalWorkflowClient) {
	fake := &fakeFormApprovalWorkflowClient{
		started: map[string]domain.FormApprovalWorkflowStart{},
		missing: map[string]bool{},
	}
	options.FormApprovalWorkflows = fake
	svc := service.New(store, options)
	fake.service = svc
	return svc, fake
}

func (c *fakeFormApprovalWorkflowClient) StartFormApprovalWorkflow(_ context.Context, start domain.FormApprovalWorkflowStart) error {
	c.starts = append(c.starts, start)
	if c.startSeen != nil {
		c.startSeen <- struct{}{}
	}
	if c.startGate != nil {
		<-c.startGate
	}
	if c.failStart {
		if c.startErr != nil {
			return c.startErr
		}
		return domain.E(503, "temporal_workflow_unavailable", "forced temporal start failure")
	}
	c.started[domain.FormApprovalWorkflowID(start.TenantID, start.FormInstanceID)] = start
	return nil
}

func (c *fakeFormApprovalWorkflowClient) SignalFormApprovalWorkflow(ctx context.Context, signal domain.FormApprovalWorkflowSignal) error {
	c.signals = append(c.signals, signal)
	workflowID := domain.FormApprovalWorkflowID(signal.TenantID, signal.FormInstanceID)
	if c.missing[workflowID] {
		return domain.ErrFormApprovalWorkflowNotFound
	}
	if _, ok := c.started[workflowID]; !ok {
		projection, err := c.service.Workflow().LoadTemporalFormApprovalProjection(domain.RequestContext{
			Context:  ctx,
			TenantID: signal.TenantID,
		}, signal.FormInstanceID)
		if err != nil {
			return err
		}
		if projection.RunID == "" {
			return domain.ErrFormApprovalWorkflowNotFound
		}
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

func (c *fakeFormApprovalWorkflowClient) forgetWorkflow(tenantID, formInstanceID string) {
	workflowID := domain.FormApprovalWorkflowID(tenantID, formInstanceID)
	delete(c.started, workflowID)
	c.missing[workflowID] = true
}
