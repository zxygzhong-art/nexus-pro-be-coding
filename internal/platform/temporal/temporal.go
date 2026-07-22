package temporal

import (
	"context"
	"strings"
	"time"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/platform/temporal/workflows"

	sdkclient "go.temporal.io/sdk/client"
	sdkworker "go.temporal.io/sdk/worker"
)

// Config contains the small Temporal surface used by this service.
type Config struct {
	HostPort  string
	Namespace string
	TaskQueue string
}

// Dial opens a Temporal SDK client for the configured namespace.
func Dial(ctx context.Context, cfg Config) (sdkclient.Client, error) {
	return sdkclient.DialContext(ctx, sdkclient.Options{
		HostPort:  strings.TrimSpace(cfg.HostPort),
		Namespace: strings.TrimSpace(cfg.Namespace),
	})
}

// FormApprovalClient adapts the SDK client to the service-layer workflow interface.
type FormApprovalClient struct {
	client    sdkclient.Client
	taskQueue string
}

// NewFormApprovalClient returns a form approval workflow starter/signaler.
func NewFormApprovalClient(client sdkclient.Client, taskQueue string) FormApprovalClient {
	return FormApprovalClient{client: client, taskQueue: strings.TrimSpace(taskQueue)}
}

// StartFormApprovalWorkflow starts the workflow and atomically sends the submit signal.
func (c FormApprovalClient) StartFormApprovalWorkflow(ctx context.Context, input domain.FormApprovalWorkflowStart) error {
	_, err := c.EnsureFormApprovalWorkflow(ctx, input)
	return err
}

// EnsureFormApprovalWorkflow idempotently starts or signals the persisted execution identity.
func (c FormApprovalClient) EnsureFormApprovalWorkflow(ctx context.Context, input domain.FormApprovalWorkflowStart) (domain.FormApprovalWorkflowExecution, error) {
	if c.client == nil {
		return domain.FormApprovalWorkflowExecution{}, domain.E(503, "temporal_workflow_unavailable", "temporal form approval workflow client is required")
	}
	if err := domain.ValidateFormApprovalWorkflowStart(input); err != nil {
		return domain.FormApprovalWorkflowExecution{}, err
	}
	workflowID := domain.ResolveFormApprovalWorkflowID(input.WorkflowID, input.TenantID, input.FormInstanceID, input.RunID)
	signal := domain.FormApprovalWorkflowSignal{
		TenantID:       input.TenantID,
		FormInstanceID: input.FormInstanceID,
		Action:         domain.FormApprovalWorkflowActionSubmit,
	}
	execution, err := c.client.SignalWithStartWorkflow(ctx, workflowID, domain.FormApprovalWorkflowSignalName, signal, sdkclient.StartWorkflowOptions{
		ID:        workflowID,
		TaskQueue: c.taskQueue,
	}, workflows.FormApprovalWorkflow, input)
	if err != nil {
		return domain.FormApprovalWorkflowExecution{}, err
	}
	return domain.FormApprovalWorkflowExecution{WorkflowID: execution.GetID(), RunID: execution.GetRunID()}, nil
}

// SignalFormApprovalWorkflow sends a review signal to an existing workflow.
func (c FormApprovalClient) SignalFormApprovalWorkflow(ctx context.Context, signal domain.FormApprovalWorkflowSignal) error {
	if c.client == nil {
		return domain.ErrFormApprovalWorkflowNotFound
	}
	workflowID := domain.ResolveFormApprovalWorkflowID(signal.WorkflowID, signal.TenantID, signal.FormInstanceID, signal.RunID)
	if err := c.client.SignalWorkflow(ctx, workflowID, "", domain.FormApprovalWorkflowSignalName, signal); err != nil {
		if temporalErrorLooksNotFound(err) {
			return domain.ErrFormApprovalWorkflowNotFound
		}
		return err
	}
	return nil
}

// NewWorker registers workflows and activities on one task queue.
func NewWorker(client sdkclient.Client, taskQueue string, activities *workflows.Activities) sdkworker.Worker {
	w := sdkworker.New(client, strings.TrimSpace(taskQueue), sdkworker.Options{WorkerStopTimeout: 5 * time.Second})
	w.RegisterWorkflow(workflows.FormApprovalWorkflow)
	if activities != nil {
		w.RegisterActivity(activities)
	}
	return w
}

func temporalErrorLooksNotFound(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "not found") || strings.Contains(text, "workflow execution already completed")
}
