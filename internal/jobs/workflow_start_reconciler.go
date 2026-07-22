package jobs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"nexus-pro-api/internal/repository"
)

const (
	defaultWorkflowStartReconcileInterval = time.Minute
	defaultWorkflowStartReconcileBatch    = 100
)

type WorkflowStartReconcileHandler interface {
	ReconcilePendingWorkflowStarts(context.Context, string, int) (int, error)
}

type WorkflowStartReconciler struct {
	store   repository.Store
	handler WorkflowStartReconcileHandler
	logger  *slog.Logger
}

func NewWorkflowStartReconciler(store repository.Store, handler WorkflowStartReconcileHandler, logger *slog.Logger) *WorkflowStartReconciler {
	if logger == nil {
		logger = slog.Default()
	}
	return &WorkflowStartReconciler{store: store, handler: handler, logger: logger}
}

func (r *WorkflowStartReconciler) Run(ctx context.Context) {
	r.reconcileAndLog(ctx)
	ticker := time.NewTicker(defaultWorkflowStartReconcileInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.reconcileAndLog(ctx)
		}
	}
}

func (r *WorkflowStartReconciler) ReconcileAllTenants(ctx context.Context) (int, error) {
	if r == nil || r.store == nil || r.handler == nil {
		return 0, errors.New("workflow start reconciler requires store and handler")
	}
	tenants, err := r.store.ListTenants(ctx)
	if err != nil {
		return 0, err
	}
	processed := 0
	var errs []error
	for _, tenant := range tenants {
		count, err := r.handler.ReconcilePendingWorkflowStarts(ctx, tenant.ID, defaultWorkflowStartReconcileBatch)
		processed += count
		if err != nil {
			errs = append(errs, fmt.Errorf("tenant %s: %w", tenant.ID, err))
		}
	}
	return processed, errors.Join(errs...)
}

func (r *WorkflowStartReconciler) reconcileAndLog(ctx context.Context) {
	processed, err := r.ReconcileAllTenants(ctx)
	if err != nil {
		r.logger.WarnContext(ctx, "workflow start reconciliation incomplete", "error", err)
	}
	if processed > 0 {
		r.logger.InfoContext(ctx, "workflow starts reconciled", "runs", processed)
	}
}
