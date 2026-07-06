package service_test

import "nexus-pro-be/internal/domain"

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
