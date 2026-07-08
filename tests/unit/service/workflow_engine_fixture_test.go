package service_test

import (
	"context"
	"testing"
	"time"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/repository/memory"
	"nexus-pro-be/internal/service"
)

func newWorkflowEngineFixture(t *testing.T, now time.Time, reviewerAccountID string) (*service.Service, domain.RequestContext, *memory.Store) {
	svc, ctx, store, _ := newWorkflowEngineFixtureWithFake(t, now, reviewerAccountID)
	return svc, ctx, store
}

func newWorkflowEngineFixtureWithFake(t *testing.T, now time.Time, reviewerAccountID string) (*service.Service, domain.RequestContext, *memory.Store, *fakeFormApprovalWorkflowClient) {
	t.Helper()
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-workflow-applicant",
		TenantID: "tenant-1",
		Name:     "Workflow Applicant",
		Permissions: []domain.Permission{
			{Resource: "workflow.form_instance", Action: "submit", Scope: "self"},
			{Resource: "workflow.form_instance", Action: "read", Scope: "self"},
		},
		CreatedAt: now,
	})
	_ = store.UpsertEmployee(context.Background(), domain.Employee{
		ID:        "emp-applicant",
		TenantID:  "tenant-1",
		Name:      "Applicant One",
		AccountID: "acct-applicant",
		Status:    "active",
		CreatedAt: now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{
		ID:                     "acct-applicant",
		TenantID:               "tenant-1",
		DisplayName:            "Applicant One",
		EmployeeID:             "emp-applicant",
		Status:                 "active",
		DirectPermissionSetIDs: []string{"ps-workflow-applicant"},
		CreatedAt:              now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{
		ID:          reviewerAccountID,
		TenantID:    "tenant-1",
		DisplayName: "Reviewer",
		Status:      "active",
		CreatedAt:   now,
	})
	_ = store.UpsertFormTemplate(context.Background(), domain.FormTemplate{
		ID:        "ft-leave",
		TenantID:  "tenant-1",
		Key:       "leave-request",
		Name:      "请假申请单",
		Schema:    workflowEnabledTemplateSchema(reviewerAccountID),
		CreatedAt: now,
	})
	svc, fake := newServiceWithFakeFormApprovalWorkflows(store, service.Options{Now: func() time.Time { return now.Add(time.Hour) }})
	return svc, domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-applicant"}, store, fake
}
