package domain

import "strings"

// agent_catalog.go — Agent 工具目錄與在地化輔助（純函式，service 與 service/agent 共用）。

// AgentToolCatalog 是 Agent 工具的靜態目錄（授權 tuple 開通與後台清單的共同真相）。
func AgentToolCatalog() []AgentToolMeta {
	return []AgentToolMeta{
		{Value: "knowledge.search", Label: "Knowledge Search", Description: "Search tenant knowledge content.", Readonly: true, RequiredPermission: "agent.tool.call:knowledge.search"},
		{Value: "get_my_profile", Label: "My Profile", Description: "Read current account profile.", Readonly: true, RequiredPermission: "agent.tool.call:get_my_profile"},
		{Value: "list_employees", Label: "List Employees", Description: "Read employee summaries.", Readonly: true, RequiredPermission: "agent.tool.call:list_employees"},
		{Value: "get_employee", Label: "Get Employee", Description: "Read one employee summary.", Readonly: true, RequiredPermission: "agent.tool.call:get_employee"},
		{Value: "my_leave_balances", Label: "My Leave Balances", Description: "Read current account leave balances.", Readonly: true, RequiredPermission: "agent.tool.call:my_leave_balances"},
		{Value: "check_leave_eligibility", Label: "Check Leave Eligibility", Description: "Check leave policy and choose balance reservation or the non-blocking no-balance fallback.", Readonly: true, RequiredPermission: "agent.tool.call:check_leave_eligibility"},
		{Value: "my_clock_records", Label: "My Clock Records", Description: "Read current account clock records.", Readonly: true, RequiredPermission: "agent.tool.call:my_clock_records"},
		{Value: "my_attendance_summary", Label: "My Attendance Summary", Description: "Read the current employee's monthly attendance summary.", Readonly: true, RequiredPermission: "agent.tool.call:my_attendance_summary"},
		{Value: "my_form_history", Label: "My Form History", Description: "Read the current account's own form application history.", Readonly: true, RequiredPermission: "agent.tool.call:my_form_history"},
		{Value: "my_pending_reviews", Label: "My Pending Reviews", Description: "Read pending workflow reviews.", Readonly: true, RequiredPermission: "agent.tool.call:my_pending_reviews"},
		{Value: "workspace_insights", Label: "Workspace Insights", Description: "Read workspace insight reports.", Readonly: true, RequiredPermission: "agent.tool.call:workspace_insights"},
		{Value: "list_published_form_templates", Label: "Published Forms", Description: "List published forms available to the current account.", Readonly: true, RequiredPermission: "agent.tool.call:list_published_form_templates"},
		{Value: "get_published_form_template", Label: "Form Schema", Description: "Read an Agent-safe form schema and data sources.", Readonly: true, RequiredPermission: "agent.tool.call:get_published_form_template"},
		{Value: "create_form_draft", Label: "Create Form Draft", Description: "Create a reversible form draft for the current account.", Readonly: false, RequiredPermission: "agent.tool.call:create_form_draft"},
		{Value: "update_form_draft", Label: "Update Form Draft", Description: "Update a reversible form draft owned by the current account.", Readonly: false, RequiredPermission: "agent.tool.call:update_form_draft"},
		{Value: "preview_form_submission", Label: "Preview Form Submission", Description: "Validate a draft and prepare explicit user confirmation.", Readonly: true, RequiredPermission: "agent.tool.call:preview_form_submission"},
		{Value: "prepare_bulk_review", Label: "Prepare Bulk Review", Description: "Prepare a fixed review batch for explicit user confirmation.", Readonly: true, RequiredPermission: "agent.tool.call:prepare_bulk_review"},
		{Value: "form.get_capabilities", Label: "Form Builder Capabilities", Description: "Read controlled form schema, widgets, data-source metadata, and workflow roles.", Readonly: true, RequiredPermission: "agent.tool.call:form.get_capabilities"},
		{Value: "form.get_data_source_schema", Label: "Form Data Source Schema", Description: "Read metadata-only data-source fields for form authoring.", Readonly: true, RequiredPermission: "agent.tool.call:form.get_data_source_schema"},
		{Value: "form.create_draft", Label: "Create Form Definition Draft", Description: "Create a reversible Agent-authored form definition draft.", Readonly: false, RequiredPermission: "agent.tool.call:form.create_draft"},
		{Value: "form.update_draft", Label: "Update Form Definition Draft", Description: "Update an Agent-authored form definition draft with revision protection.", Readonly: false, RequiredPermission: "agent.tool.call:form.update_draft"},
		{Value: "form.validate_draft", Label: "Validate Form Definition Draft", Description: "Validate and compile a controlled form definition draft.", Readonly: true, RequiredPermission: "agent.tool.call:form.validate_draft"},
		{Value: "form.preview_draft", Label: "Preview Form Definition Draft", Description: "Preview a controlled form definition draft.", Readonly: true, RequiredPermission: "agent.tool.call:form.preview_draft"},
		{Value: "form.simulate_workflow", Label: "Simulate Form Workflow", Description: "Simulate the form approval path without starting a real workflow.", Readonly: true, RequiredPermission: "agent.tool.call:form.simulate_workflow"},
	}
}

// LocalizedSuggestedQuestions 依 locale 選擇建議問題文案。
func LocalizedSuggestedQuestions(
	items []LocalizedAgentSuggestedQuestion,
	locale string,
	fallback []string,
) []string {
	locale = PreferredLocaleWithDefault(locale)
	result := make([]string, 0, len(items))
	for _, item := range items {
		value := strings.TrimSpace(item.Translations[locale])
		if value == "" {
			value = strings.TrimSpace(item.Translations[DefaultPreferredLocale])
		}
		if value == "" {
			value = strings.TrimSpace(item.Translations[PreferredLocaleENUS])
		}
		if value != "" {
			result = append(result, value)
		}
	}
	if len(result) == 0 {
		return uniqueStrings(fallback)
	}
	return result
}

// uniqueStrings 去重去空（domain 內部小工具，與 service 同名函式相同語意）。
func uniqueStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, v := range values {
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}
