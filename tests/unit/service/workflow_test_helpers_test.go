package service_test

import (
	"context"
	"testing"
	"time"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/repository/memory"
	"nexus-pro-be/internal/service"
)

func workflowEnabledTemplateSchema(assigneeAccountIDs ...string) map[string]any {
	stage := map[string]any{
		"id":     "stage-approver",
		"type":   "approver",
		"label":  "直屬主管",
		"detail": "由直屬主管審覈",
	}
	if len(assigneeAccountIDs) > 0 {
		ids := make([]any, 0, len(assigneeAccountIDs))
		for _, id := range assigneeAccountIDs {
			ids = append(ids, id)
		}
		stage["config"] = map[string]any{"account_ids": ids}
	} else {
		stage["config"] = map[string]any{"role": "manager"}
	}
	return map[string]any{
		"workspace_design": map[string]any{
			"enabled": true,
			"stages":  []map[string]any{stage},
		},
	}
}

func workflowTemplateWithSchema(key, name string, schema map[string]any) domain.FormTemplate {
	return domain.FormTemplate{
		Key:    key,
		Name:   name,
		Schema: schema,
	}
}

func startWorkflowRunForTest(t *testing.T, svc *service.Service, store *memory.Store, tenantID, formInstanceID, applicantAccountID string) {
	t.Helper()
	instance, ok, err := store.GetFormInstance(context.Background(), tenantID, formInstanceID)
	if err != nil || !ok {
		t.Fatalf("form instance lookup failed ok=%v err=%v", ok, err)
	}
	template, ok, err := store.GetFormTemplate(context.Background(), tenantID, instance.TemplateID)
	if err != nil || !ok {
		t.Fatalf("form template lookup failed ok=%v err=%v", ok, err)
	}
	applicant, ok, err := store.GetAccount(context.Background(), tenantID, applicantAccountID)
	if err != nil || !ok {
		t.Fatalf("applicant account lookup failed ok=%v err=%v", ok, err)
	}
	if err := svc.Workflow().StartWorkflowRun(domain.RequestContext{TenantID: tenantID, AccountID: applicantAccountID}, instance, template, applicant); err != nil {
		t.Fatal(err)
	}
}

// grantDirectAttendanceCreatePermission enables the compatibility create endpoints for the workflow applicant fixture.
func grantDirectAttendanceCreatePermission(t *testing.T, store *memory.Store) {
	t.Helper()
	permissionSet, ok, err := store.GetPermissionSet(t.Context(), "tenant-1", "ps-workflow-applicant")
	if err != nil || !ok {
		t.Fatalf("permission set lookup failed ok=%v err=%v", ok, err)
	}
	permissionSet.Permissions = append(permissionSet.Permissions, domain.Permission{
		Resource: "attendance.leave", Action: "create", Scope: "self",
	})
	if err := store.UpsertPermissionSet(t.Context(), permissionSet); err != nil {
		t.Fatal(err)
	}
}

// newDirectAttendanceWorkflowService seeds the minimum review contract required by compatibility submissions.
func newDirectAttendanceWorkflowService(t *testing.T, store *memory.Store, now time.Time, templateKeys ...string) *service.Service {
	t.Helper()
	if err := store.UpsertAccount(t.Context(), domain.Account{
		ID: "acct-direct-reviewer", TenantID: "tenant-1", DisplayName: "Direct Reviewer", Status: "active", CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	for _, templateKey := range templateKeys {
		if err := store.UpsertFormTemplate(t.Context(), domain.FormTemplate{
			ID: "ft-" + templateKey, TenantID: "tenant-1", Key: templateKey, Name: templateKey,
			Schema: workflowEnabledTemplateSchema("acct-direct-reviewer"), CreatedAt: now,
		}); err != nil {
			t.Fatal(err)
		}
	}
	svc, _ := newServiceWithFakeFormApprovalWorkflows(store, service.Options{Now: func() time.Time { return now }})
	return svc
}
