package service_test

import (
	"context"
	"testing"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/repository/memory"
	"nexus-pro-be/internal/service"
)

func workflowEnabledTemplateSchema(assigneeAccountIDs ...string) map[string]any {
	stage := map[string]any{
		"id":     "stage-approver",
		"type":   "approver",
		"label":  "直属主管",
		"detail": "由直属主管审核",
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
