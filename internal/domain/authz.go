package domain

import "strings"

// Effect 表示 effect。
type Effect string

// Severity 表示 severity。
type Severity string

// PrincipalType 表示 principal type。
type PrincipalType string

// Scope 表示範圍。
type Scope string

// ApplicationCode 表示 application 碼。
type ApplicationCode string

// ResourceType 表示 resource type。
type ResourceType string

// Action 表示 action。
type Action string

// FieldPolicyEffect 表示欄位政策 effect。
type FieldPolicyEffect string

// EventType 表示事件 type。
type EventType string

// 下列常數定義此模組使用的固定值。
const (
	EffectAllow Effect = "allow"
	EffectDeny  Effect = "deny"
)

// 下列常數定義此模組使用的固定值。
const (
	SeverityLow      Severity = "low"
	SeverityMedium   Severity = "medium"
	SeverityHigh     Severity = "high"
	SeverityCritical Severity = "critical"
)

// 下列常數定義此模組使用的固定值。
const (
	PrincipalTypeAccount       PrincipalType = "account"
	PrincipalTypeUserGroup     PrincipalType = "user_group"
	PrincipalTypeAssumableRole PrincipalType = "assumable_role"
)

// 下列常數定義此模組使用的固定值。
const (
	ScopeAll               Scope = "all"
	ScopeSelf              Scope = "self"
	ScopeOwn               Scope = "own"
	ScopeTenant            Scope = "tenant"
	ScopeObject            Scope = "object"
	ScopeDepartment        Scope = "department"
	ScopeDepartmentSubtree Scope = "department_subtree"
	ScopeDirectReports     Scope = "direct_reports"
	ScopeAssignedOrgUnits  Scope = "assigned_org_units"
	ScopeCustomCondition   Scope = "custom_condition"
	ScopeSystem            Scope = "system"
)

// 下列常數定義此模組使用的固定值。
const (
	AppPlatform   ApplicationCode = "platform"
	AppHR         ApplicationCode = "hr"
	AppIAM        ApplicationCode = "iam"
	AppAttendance ApplicationCode = "attendance"
	AppAgent      ApplicationCode = "agent"
	AppWorkflow   ApplicationCode = "workflow"
	AppAudit      ApplicationCode = "audit"
)

// DefaultApplications 保存預設 application 目錄。
var DefaultApplications = []struct {
	Code        ApplicationCode
	Name        string
	Description string
}{
	{Code: AppPlatform, Name: "Platform", Description: "Personal portal and platform aggregate APIs."},
	{Code: AppHR, Name: "HR", Description: "People domain and employee management APIs."},
	{Code: AppIAM, Name: "IAM", Description: "Identity and access management APIs."},
	{Code: AppAttendance, Name: "Attendance", Description: "Attendance, leave and clocking APIs."},
	{Code: AppAgent, Name: "Agent", Description: "AI agent runtime APIs."},
	{Code: AppWorkflow, Name: "Workflow", Description: "Workflow and form APIs."},
	{Code: AppAudit, Name: "Audit", Description: "Audit log APIs."},
}

// 下列常數定義此模組使用的固定值。
const (
	ResourceEmployee             ResourceType = "employee"
	ResourceEmployeeSync         ResourceType = "employee_sync"
	ResourceOrgUnit              ResourceType = "org_unit"
	ResourcePosition             ResourceType = "position"
	ResourceLeave                ResourceType = "leave"
	ResourceAttendanceWorksite   ResourceType = "worksite"
	ResourceAttendanceClock      ResourceType = "clock"
	ResourceAttendanceCorrection ResourceType = "correction"
	ResourceUserGroup            ResourceType = "user_group"
	ResourceIAMAccount           ResourceType = "account"
	ResourcePermission           ResourceType = "permission"
	ResourcePermissionPackage    ResourceType = "permission_package"
	ResourcePermissionSet        ResourceType = "permission_set"
	ResourcePermissionAssign     ResourceType = "permission_set_assignment"
	ResourceDataScope            ResourceType = "data_scope"
	ResourceFieldPolicy          ResourceType = "field_policy"
	ResourceApplication          ResourceType = "application"
	ResourceResourceType         ResourceType = "resource_type"
	ResourceOutboxEvent          ResourceType = "outbox_event"
	ResourceAssumableRole        ResourceType = "assumable_role"
	ResourceTool                 ResourceType = "tool"
	ResourceDefinition           ResourceType = "definition"
	ResourceUsage                ResourceType = "usage"
	ResourceModel                ResourceType = "model"
	ResourceEmployeeCollection   ResourceType = "employee_collection"
	ResourceFormInstance         ResourceType = "form_instance"
	ResourceFormDefinitionDraft  ResourceType = "form_definition_draft"
	ResourceNotification         ResourceType = "notification"
)

// 下列常數定義此模組使用的固定值。
const (
	ActionRead             Action = "read"
	ActionCreate           Action = "create"
	ActionUpdate           Action = "update"
	ActionDelete           Action = "delete"
	ActionExport           Action = "export"
	ActionImport           Action = "import"
	ActionAssume           Action = "assume"
	ActionInvite           Action = "invite"
	ActionSubmit           Action = "submit"
	ActionApprove          Action = "approve"
	ActionCall             Action = "call"
	ActionUpdateStatus     Action = "update_status"
	ActionStatusTransition Action = "status_transition"
)

// 下列常數定義此模組使用的固定值。
const (
	FieldPolicyEffectAllow    FieldPolicyEffect = "allow"
	FieldPolicyEffectDeny     FieldPolicyEffect = "deny"
	FieldPolicyEffectMask     FieldPolicyEffect = "mask"
	FieldPolicyEffectHide     FieldPolicyEffect = "hide"
	FieldPolicyEffectReadonly FieldPolicyEffect = "readonly"
)

// 下列常數定義此模組使用的固定值。
const (
	EventOpenFGARelationshipWrite  EventType = "openfga.relationship.write"
	EventOpenFGARelationshipDelete EventType = "openfga.relationship.delete"
)

// RiskLevel 表示 risk level。
type RiskLevel string

// 下列常數定義此模組使用的固定值。
const (
	RiskNormal   RiskLevel = "normal"
	RiskHigh     RiskLevel = "high"
	RiskCritical RiskLevel = "critical"
)

// RoutePolicy 定義路由政策的資料結構。
type RoutePolicy struct {
	Name            string
	Method          string
	Path            string
	ApplicationCode string
	ResourceType    string
	Action          string
	RiskLevel       RiskLevel
}

// RelationshipCheck 定義關係 check 的資料結構。
type RelationshipCheck struct {
	TenantID string
	Subject  string
	Relation string
	Object   string
}

// AuditEvent 處理稽覈事件。
func (r CheckRequest) AuditEvent() string {
	req := r
	if req.ApplicationCode == "" || req.ResourceType == "" {
		app, resourceType := splitResourceName(req.Resource)
		if req.ApplicationCode == "" {
			req.ApplicationCode = ApplicationCode(app)
		}
		if req.ResourceType == "" {
			req.ResourceType = ResourceType(resourceType)
		}
	}
	if req.ApplicationCode == "" {
		req.ApplicationCode = AppPlatform
	}
	if req.ResourceType == "" {
		req.ResourceType = ResourceType(req.Resource)
	}
	if req.ApplicationCode == "" || req.ResourceType == "" || req.Action == "" {
		return ""
	}
	return string(req.ApplicationCode) + "." + string(req.ResourceType) + "." + string(req.Action)
}

// splitResourceName 拆分resource 名稱。
func splitResourceName(resource string) (string, string) {
	if resource == "" {
		return string(AppPlatform), ""
	}
	if resource == "*" {
		return "*", "*"
	}
	parts := strings.SplitN(resource, ".", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return string(AppPlatform), resource
}

// DefaultRoutePolicies 保存預設路由政策。
var DefaultRoutePolicies = []RoutePolicy{
	{Name: "me.read", Method: "GET", Path: "/v1/me", ApplicationCode: "platform", ResourceType: "me", Action: "read"},
	{Name: "me.profile.update", Method: "PATCH", Path: "/v1/me/profile", ApplicationCode: "platform", ResourceType: "me", Action: "update"},
	{Name: "me.preferences.update", Method: "PATCH", Path: "/v1/me/preferences", ApplicationCode: "platform", ResourceType: "me", Action: "update"},
	{Name: "me.password.update", Method: "PUT", Path: "/v1/me/password", ApplicationCode: "platform", ResourceType: "me", Action: "update", RiskLevel: RiskCritical},
	{Name: "iam.permission.read", Method: "GET", Path: "/v1/iam/permissions", ApplicationCode: "iam", ResourceType: "permission", Action: "read"},
	{Name: "iam.permission_package.read", Method: "GET", Path: "/v1/iam/permission-packages", ApplicationCode: "iam", ResourceType: "permission_package", Action: "read"},
	{Name: "iam.user_group.read", Method: "GET", Path: "/v1/iam/user-groups", ApplicationCode: "iam", ResourceType: "user_group", Action: "read"},
	{Name: "iam.user_group.create", Method: "POST", Path: "/v1/iam/user-groups", ApplicationCode: "iam", ResourceType: "user_group", Action: "create", RiskLevel: RiskCritical},
	{Name: "iam.user_group.update", Method: "PATCH", Path: "/v1/iam/user-groups/:id", ApplicationCode: "iam", ResourceType: "user_group", Action: "update", RiskLevel: RiskCritical},
	{Name: "iam.user_group.members.read", Method: "GET", Path: "/v1/iam/user-groups/:id/members", ApplicationCode: "iam", ResourceType: "user_group", Action: "read"},
	{Name: "iam.user_group.members.add", Method: "POST", Path: "/v1/iam/user-groups/:id/members", ApplicationCode: "iam", ResourceType: "user_group", Action: "update", RiskLevel: RiskCritical},
	{Name: "iam.user_group.members.remove", Method: "DELETE", Path: "/v1/iam/user-groups/:id/members/:accountId", ApplicationCode: "iam", ResourceType: "user_group", Action: "update", RiskLevel: RiskCritical},
	{Name: "iam.permission_set.read", Method: "GET", Path: "/v1/iam/permission-sets", ApplicationCode: "iam", ResourceType: "permission_set", Action: "read"},
	{Name: "iam.permission_set.create", Method: "POST", Path: "/v1/iam/permission-sets", ApplicationCode: "iam", ResourceType: "permission_set", Action: "create", RiskLevel: RiskCritical},
	{Name: "iam.permission_set.update", Method: "PATCH", Path: "/v1/iam/permission-sets/:id", ApplicationCode: "iam", ResourceType: "permission_set", Action: "update", RiskLevel: RiskCritical},
	{Name: "iam.account.read", Method: "GET", Path: "/v1/iam/accounts", ApplicationCode: "iam", ResourceType: "account", Action: "read"},
	{Name: "iam.account.options", Method: "GET", Path: "/v1/iam/accounts/options", ApplicationCode: "iam", ResourceType: "account", Action: "read"},
	{Name: "iam.permission_set.options", Method: "GET", Path: "/v1/iam/permission-sets/options", ApplicationCode: "iam", ResourceType: "permission_set", Action: "read"},
	{Name: "iam.user_group.options", Method: "GET", Path: "/v1/iam/user-groups/options", ApplicationCode: "iam", ResourceType: "user_group", Action: "read"},
	{Name: "iam.assumable_role.options", Method: "GET", Path: "/v1/iam/assumable-roles/options", ApplicationCode: "iam", ResourceType: "assumable_role", Action: "read"},
	{Name: "iam.permission_assignment.read", Method: "GET", Path: "/v1/iam/permission-set-assignments", ApplicationCode: "iam", ResourceType: "permission_set_assignment", Action: "read"},
	{Name: "iam.permission_assignment.create", Method: "POST", Path: "/v1/iam/permission-set-assignments", ApplicationCode: "iam", ResourceType: "permission_set_assignment", Action: "create", RiskLevel: RiskCritical},
	{Name: "iam.permission_assignment.delete", Method: "DELETE", Path: "/v1/iam/permission-set-assignments/:id", ApplicationCode: "iam", ResourceType: "permission_set_assignment", Action: "delete", RiskLevel: RiskCritical},
	{Name: "iam.data_scope.read", Method: "GET", Path: "/v1/iam/data-scopes", ApplicationCode: "iam", ResourceType: "data_scope", Action: "read"},
	{Name: "iam.data_scope.create", Method: "POST", Path: "/v1/iam/data-scopes", ApplicationCode: "iam", ResourceType: "data_scope", Action: "create", RiskLevel: RiskCritical},
	{Name: "iam.data_scope.update", Method: "PATCH", Path: "/v1/iam/data-scopes/:id", ApplicationCode: "iam", ResourceType: "data_scope", Action: "update", RiskLevel: RiskCritical},
	{Name: "iam.data_scope.delete", Method: "DELETE", Path: "/v1/iam/data-scopes/:id", ApplicationCode: "iam", ResourceType: "data_scope", Action: "delete", RiskLevel: RiskCritical},
	{Name: "iam.field_policy.read", Method: "GET", Path: "/v1/iam/field-policies", ApplicationCode: "iam", ResourceType: "field_policy", Action: "read"},
	{Name: "iam.field_policy.create", Method: "POST", Path: "/v1/iam/field-policies", ApplicationCode: "iam", ResourceType: "field_policy", Action: "create", RiskLevel: RiskCritical},
	{Name: "iam.field_policy.update", Method: "PATCH", Path: "/v1/iam/field-policies/:id", ApplicationCode: "iam", ResourceType: "field_policy", Action: "update", RiskLevel: RiskCritical},
	{Name: "iam.field_policy.delete", Method: "DELETE", Path: "/v1/iam/field-policies/:id", ApplicationCode: "iam", ResourceType: "field_policy", Action: "delete", RiskLevel: RiskCritical},
	{Name: "iam.assumable_role.read", Method: "GET", Path: "/v1/iam/assumable-roles", ApplicationCode: "iam", ResourceType: "assumable_role", Action: "read"},
	{Name: "iam.assumable_role.create", Method: "POST", Path: "/v1/iam/assumable-roles", ApplicationCode: "iam", ResourceType: "assumable_role", Action: "create", RiskLevel: RiskCritical},
	{Name: "iam.assumable_role.assume", Method: "POST", Path: "/v1/iam/assumable-roles/:id/assume", ApplicationCode: "iam", ResourceType: "assumable_role", Action: "assume", RiskLevel: RiskCritical},
	{Name: "iam.assumable_role_session.revoke_current", Method: "DELETE", Path: "/v1/iam/assumable-role-sessions/current", ApplicationCode: "platform", ResourceType: "me", Action: "read", RiskLevel: RiskCritical},
	{Name: "hr.position.read", Method: "GET", Path: "/v1/hr/positions", ApplicationCode: "hr", ResourceType: "position", Action: "read"},
	{Name: "hr.employee.read", Method: "GET", Path: "/v1/hr/employees", ApplicationCode: "hr", ResourceType: "employee", Action: "read"},
	{Name: "hr.employee.detail", Method: "GET", Path: "/v1/hr/employees/:id", ApplicationCode: "hr", ResourceType: "employee", Action: "read"},
	{Name: "hr.employee.update", Method: "PATCH", Path: "/v1/hr/employees/:id", ApplicationCode: "hr", ResourceType: "employee", Action: "update"},
	{Name: "hr.employee.stats", Method: "GET", Path: "/v1/hr/employees/stats", ApplicationCode: "hr", ResourceType: "employee", Action: "read"},
	{Name: "hr.employee.ehrms_sync", Method: "POST", Path: "/v1/hr/employees/ehrms/sync", ApplicationCode: "hr", ResourceType: "employee", Action: "import", RiskLevel: RiskCritical},
	{Name: "hr.employee.export_download", Method: "GET", Path: "/v1/hr/employees/export", ApplicationCode: "hr", ResourceType: "employee", Action: "export", RiskLevel: RiskCritical},
	{Name: "hr.org_unit.read", Method: "GET", Path: "/v1/org/units", ApplicationCode: "hr", ResourceType: "org_unit", Action: "read"},
	{Name: "hr.org_unit.create", Method: "POST", Path: "/v1/org/units", ApplicationCode: "hr", ResourceType: "org_unit", Action: "create"},
	{Name: "hr.org_unit.ehrms_sync", Method: "POST", Path: "/v1/org/units/ehrms/sync", ApplicationCode: "hr", ResourceType: "org_unit", Action: "create", RiskLevel: RiskCritical},
	{Name: "hr.org_unit.update", Method: "PATCH", Path: "/v1/org/units/:id", ApplicationCode: "hr", ResourceType: "org_unit", Action: "update"},
	{Name: "attendance.leave.read_balance", Method: "GET", Path: "/v1/attendance/leave-balances", ApplicationCode: "attendance", ResourceType: "leave", Action: "read"},
	{Name: "attendance.leave_type.read", Method: "GET", Path: "/v1/attendance/leave-types", ApplicationCode: "attendance", ResourceType: "leave", Action: "read"},
	{Name: "attendance.leave_type.set_enabled", Method: "PATCH", Path: "/v1/attendance/leave-types/:id", ApplicationCode: "attendance", ResourceType: "leave", Action: "update", RiskLevel: RiskHigh},
	{Name: "attendance.leave.read_request", Method: "GET", Path: "/v1/attendance/leave-requests", ApplicationCode: "attendance", ResourceType: "leave", Action: "read"},
	{Name: "attendance.policy.read", Method: "GET", Path: "/v1/attendance/policies/current", ApplicationCode: "attendance", ResourceType: "leave", Action: "read"},
	{Name: "attendance.policy.validate", Method: "POST", Path: "/v1/attendance/policies/validate", ApplicationCode: "attendance", ResourceType: "leave", Action: "update"},
	{Name: "attendance.policy.publish", Method: "POST", Path: "/v1/attendance/policies/publish", ApplicationCode: "attendance", ResourceType: "leave", Action: "update", RiskLevel: RiskCritical},
	{Name: "attendance.worksite.read", Method: "GET", Path: "/v1/attendance/worksites", ApplicationCode: "attendance", ResourceType: "worksite", Action: "read"},
	{Name: "attendance.worksite.create", Method: "POST", Path: "/v1/attendance/worksites", ApplicationCode: "attendance", ResourceType: "worksite", Action: "create"},
	{Name: "attendance.worksite.update", Method: "PATCH", Path: "/v1/attendance/worksites", ApplicationCode: "attendance", ResourceType: "worksite", Action: "update"},
	{Name: "attendance.clock.status", Method: "GET", Path: "/v1/attendance/clock-status", ApplicationCode: "attendance", ResourceType: "clock", Action: "read"},
	{Name: "attendance.clock.monthly_summary", Method: "GET", Path: "/v1/attendance/monthly-summary", ApplicationCode: "attendance", ResourceType: "clock", Action: "read"},
	{Name: "attendance.clock.read", Method: "GET", Path: "/v1/attendance/clock-records", ApplicationCode: "attendance", ResourceType: "clock", Action: "read"},
	{Name: "attendance.clock.create", Method: "POST", Path: "/v1/attendance/clock-records", ApplicationCode: "attendance", ResourceType: "clock", Action: "create"},
	{Name: "attendance.leave_type.ehrms_sync", Method: "POST", Path: "/v1/attendance/ehrms/leave-types/sync", ApplicationCode: "attendance", ResourceType: "clock", Action: "import", RiskLevel: RiskCritical},
	{Name: "attendance.ehrms.sync", Method: "POST", Path: "/v1/attendance/ehrms/sync", ApplicationCode: "attendance", ResourceType: "clock", Action: "import", RiskLevel: RiskCritical},
	{Name: "attendance.correction.read", Method: "GET", Path: "/v1/attendance/corrections", ApplicationCode: "attendance", ResourceType: "correction", Action: "read"},
	{Name: "attendance.correction.create", Method: "POST", Path: "/v1/attendance/corrections", ApplicationCode: "attendance", ResourceType: "correction", Action: "create"},
	{Name: "attendance.correction.approve", Method: "POST", Path: "/v1/attendance/corrections/:id/approve", ApplicationCode: "attendance", ResourceType: "correction", Action: "approve", RiskLevel: RiskHigh},
	{Name: "attendance.correction.reject", Method: "POST", Path: "/v1/attendance/corrections/:id/reject", ApplicationCode: "attendance", ResourceType: "correction", Action: "update", RiskLevel: RiskHigh},
	{Name: "platform.home.read", Method: "GET", Path: "/v1/platform/home", ApplicationCode: "platform", ResourceType: "me", Action: "read"},
	{Name: "platform.assistants.read", Method: "GET", Path: "/v1/platform/assistants", ApplicationCode: "platform", ResourceType: "me", Action: "read"},
	{Name: "platform.forms.read", Method: "GET", Path: "/v1/platform/forms", ApplicationCode: "platform", ResourceType: "me", Action: "read"},
	{Name: "platform.tasks.read", Method: "GET", Path: "/v1/platform/tasks", ApplicationCode: "platform", ResourceType: "me", Action: "read"},
	{Name: "platform.task_item.create", Method: "POST", Path: "/v1/platform/tasks/items", ApplicationCode: "platform", ResourceType: "me", Action: "create"},
	{Name: "platform.task_item.update", Method: "PATCH", Path: "/v1/platform/tasks/items/:id", ApplicationCode: "platform", ResourceType: "me", Action: "update"},
	{Name: "platform.task_item.delete", Method: "DELETE", Path: "/v1/platform/tasks/items/:id", ApplicationCode: "platform", ResourceType: "me", Action: "delete"},
	{Name: "platform.task_todo.create", Method: "POST", Path: "/v1/platform/tasks/todos", ApplicationCode: "platform", ResourceType: "me", Action: "create"},
	{Name: "platform.task_todo.update", Method: "PATCH", Path: "/v1/platform/tasks/todos/:id", ApplicationCode: "platform", ResourceType: "me", Action: "update"},
	{Name: "platform.task_todo.delete", Method: "DELETE", Path: "/v1/platform/tasks/todos/:id", ApplicationCode: "platform", ResourceType: "me", Action: "delete"},
	{Name: "platform.task_todo.convert", Method: "POST", Path: "/v1/platform/tasks/todos/:id/convert", ApplicationCode: "platform", ResourceType: "me", Action: "update"},
	{Name: "platform.notification.read", Method: "GET", Path: "/v1/notifications", ApplicationCode: "platform", ResourceType: "me", Action: "read"},
	{Name: "platform.notification.unread_count", Method: "GET", Path: "/v1/notifications/unread-count", ApplicationCode: "platform", ResourceType: "me", Action: "read"},
	{Name: "platform.notification.read_one", Method: "POST", Path: "/v1/notifications/:id/read", ApplicationCode: "platform", ResourceType: "me", Action: "update"},
	{Name: "platform.notification.read_all", Method: "POST", Path: "/v1/notifications/read-all", ApplicationCode: "platform", ResourceType: "me", Action: "update"},
	{Name: "workspace.overview.read", Method: "GET", Path: "/v1/workspace/overview", ApplicationCode: "hr", ResourceType: "employee", Action: "read"},
	{Name: "workspace.organization.read", Method: "GET", Path: "/v1/workspace/organization", ApplicationCode: "hr", ResourceType: "employee", Action: "read"},
	{Name: "workspace.org_units_directory.read", Method: "GET", Path: "/v1/workspace/org-units-directory", ApplicationCode: "hr", ResourceType: "org_unit", Action: "read"},
	{Name: "workspace.turnover.read", Method: "GET", Path: "/v1/workspace/turnover", ApplicationCode: "hr", ResourceType: "employee", Action: "read"},
	{Name: "workspace.turnover.export", Method: "GET", Path: "/v1/workspace/turnover/export", ApplicationCode: "hr", ResourceType: "employee", Action: "export", RiskLevel: RiskCritical},
	{Name: "workspace.attendance.read", Method: "GET", Path: "/v1/workspace/attendance", ApplicationCode: "attendance", ResourceType: "clock", Action: "read"},
	{Name: "workspace.attendance.abnormals", Method: "GET", Path: "/v1/workspace/attendance/abnormals", ApplicationCode: "attendance", ResourceType: "clock", Action: "read"},
	{Name: "workspace.attendance.export", Method: "GET", Path: "/v1/workspace/attendance/export", ApplicationCode: "attendance", ResourceType: "clock", Action: "export", RiskLevel: RiskCritical},
	{Name: "workspace.form.read", Method: "GET", Path: "/v1/workspace/forms", ApplicationCode: "workflow", ResourceType: "form_template", Action: "read"},
	{Name: "workspace.form.create", Method: "POST", Path: "/v1/workspace/forms", ApplicationCode: "workflow", ResourceType: "form_template", Action: "create"},
	{Name: "workspace.form.update", Method: "PATCH", Path: "/v1/workspace/forms/:id", ApplicationCode: "workflow", ResourceType: "form_template", Action: "update"},
	{Name: "workspace.form.publish", Method: "POST", Path: "/v1/workspace/forms/:id/publish", ApplicationCode: "workflow", ResourceType: "form_template", Action: "update"},
	{Name: "workspace.form.delete", Method: "DELETE", Path: "/v1/workspace/forms/:id", ApplicationCode: "workflow", ResourceType: "form_template", Action: "delete", RiskLevel: RiskHigh},
	{Name: "workspace.audit_logs.read", Method: "GET", Path: "/v1/workspace/audit-logs", ApplicationCode: "audit", ResourceType: "audit_log", Action: "read"},
	{Name: "workspace.audit_log_facets.read", Method: "GET", Path: "/v1/workspace/audit-logs/facets", ApplicationCode: "audit", ResourceType: "audit_log", Action: "read"},
	{Name: "workspace.insights.read", Method: "GET", Path: "/v1/workspace/insights", ApplicationCode: "hr", ResourceType: "employee", Action: "read"},
	{Name: "workspace.agent_model.read", Method: "GET", Path: "/v1/workspace/agent-models", ApplicationCode: "agent", ResourceType: "model", Action: "read"},
	{Name: "workspace.agent_model.create", Method: "POST", Path: "/v1/workspace/agent-models", ApplicationCode: "agent", ResourceType: "model", Action: "create", RiskLevel: RiskHigh},
	{Name: "workspace.agent_model.update", Method: "PATCH", Path: "/v1/workspace/agent-models/:id", ApplicationCode: "agent", ResourceType: "model", Action: "update", RiskLevel: RiskHigh},
	{Name: "workspace.agent_model.delete", Method: "DELETE", Path: "/v1/workspace/agent-models/:id", ApplicationCode: "agent", ResourceType: "model", Action: "delete", RiskLevel: RiskHigh},
	{Name: "workspace.agent_model.sync", Method: "POST", Path: "/v1/workspace/agent-models/:id/sync", ApplicationCode: "agent", ResourceType: "model", Action: "update", RiskLevel: RiskHigh},
	{Name: "workspace.agent_model.test", Method: "POST", Path: "/v1/workspace/agent-models/:id/test", ApplicationCode: "agent", ResourceType: "model", Action: "update"},
	{Name: "workspace.knowledge_base.read", Method: "GET", Path: "/v1/workspace/knowledge-bases", ApplicationCode: "agent", ResourceType: "knowledge_base", Action: "read"},
	{Name: "workspace.knowledge_base.create", Method: "POST", Path: "/v1/workspace/knowledge-bases", ApplicationCode: "agent", ResourceType: "knowledge_base", Action: "create", RiskLevel: RiskHigh},
	{Name: "workspace.knowledge_base.get", Method: "GET", Path: "/v1/workspace/knowledge-bases/:id", ApplicationCode: "agent", ResourceType: "knowledge_base", Action: "read"},
	{Name: "workspace.knowledge_base.update", Method: "PATCH", Path: "/v1/workspace/knowledge-bases/:id", ApplicationCode: "agent", ResourceType: "knowledge_base", Action: "update", RiskLevel: RiskHigh},
	{Name: "workspace.knowledge_base.delete", Method: "DELETE", Path: "/v1/workspace/knowledge-bases/:id", ApplicationCode: "agent", ResourceType: "knowledge_base", Action: "delete", RiskLevel: RiskHigh},
	{Name: "workspace.knowledge_document.read", Method: "GET", Path: "/v1/workspace/knowledge-bases/:id/documents", ApplicationCode: "agent", ResourceType: "knowledge_base", Action: "read"},
	{Name: "workspace.knowledge_document.get", Method: "GET", Path: "/v1/workspace/knowledge-bases/:id/documents/:document_id", ApplicationCode: "agent", ResourceType: "knowledge_base", Action: "read"},
	{Name: "workspace.knowledge_document.create", Method: "POST", Path: "/v1/workspace/knowledge-bases/:id/documents", ApplicationCode: "agent", ResourceType: "knowledge_base", Action: "update", RiskLevel: RiskHigh},
	{Name: "workspace.knowledge_document.update", Method: "PATCH", Path: "/v1/workspace/knowledge-bases/:id/documents/:document_id", ApplicationCode: "agent", ResourceType: "knowledge_base", Action: "update", RiskLevel: RiskHigh},
	{Name: "workspace.knowledge_document.delete", Method: "DELETE", Path: "/v1/workspace/knowledge-bases/:id/documents/:document_id", ApplicationCode: "agent", ResourceType: "knowledge_base", Action: "update", RiskLevel: RiskHigh},
	{Name: "workspace.knowledge_base.search", Method: "POST", Path: "/v1/workspace/knowledge-bases/:id/search", ApplicationCode: "agent", ResourceType: "knowledge_base", Action: "read"},
	{Name: "workspace.agent_definition.read", Method: "GET", Path: "/v1/workspace/agents", ApplicationCode: "agent", ResourceType: "definition", Action: "read"},
	{Name: "workspace.agent_definition.create", Method: "POST", Path: "/v1/workspace/agents", ApplicationCode: "agent", ResourceType: "definition", Action: "create", RiskLevel: RiskHigh},
	{Name: "workspace.agent_definition.update", Method: "PATCH", Path: "/v1/workspace/agents/:id", ApplicationCode: "agent", ResourceType: "definition", Action: "update", RiskLevel: RiskHigh},
	{Name: "workspace.agent_definition.publish", Method: "POST", Path: "/v1/workspace/agents/:id/publish", ApplicationCode: "agent", ResourceType: "definition", Action: "update", RiskLevel: RiskHigh},
	{Name: "workspace.agent_definition.unpublish", Method: "POST", Path: "/v1/workspace/agents/:id/unpublish", ApplicationCode: "agent", ResourceType: "definition", Action: "update", RiskLevel: RiskHigh},
	{Name: "workspace.agent_definition.delete", Method: "DELETE", Path: "/v1/workspace/agents/:id", ApplicationCode: "agent", ResourceType: "definition", Action: "delete", RiskLevel: RiskHigh},
	{Name: "workspace.agent_definition.trial", Method: "POST", Path: "/v1/workspace/agents/:id/trial", ApplicationCode: "agent", ResourceType: "definition", Action: "update"},
	{Name: "workspace.agent_definition.rollback", Method: "POST", Path: "/v1/workspace/agents/:id/rollback", ApplicationCode: "agent", ResourceType: "definition", Action: "update", RiskLevel: RiskHigh},
	{Name: "workspace.agent_definition.tools", Method: "GET", Path: "/v1/workspace/agents/tools", ApplicationCode: "agent", ResourceType: "tool", Action: "read"},
	{Name: "workspace.agent_usage.read", Method: "GET", Path: "/v1/workspace/agent-usage", ApplicationCode: "agent", ResourceType: "usage", Action: "read", RiskLevel: RiskHigh},
	{Name: "workspace.agent_usage_sessions.read", Method: "GET", Path: "/v1/workspace/agent-usage/:id/sessions", ApplicationCode: "agent", ResourceType: "usage", Action: "read", RiskLevel: RiskHigh},
	{Name: "workspace.agent_external_tool.read", Method: "GET", Path: "/v1/workspace/agents/external-tools", ApplicationCode: "agent", ResourceType: "tool", Action: "read"},
	{Name: "workspace.agent_external_tool.create", Method: "POST", Path: "/v1/workspace/agents/external-tools", ApplicationCode: "agent", ResourceType: "tool", Action: "create", RiskLevel: RiskHigh},
	{Name: "workspace.agent_external_tool.test", Method: "POST", Path: "/v1/workspace/agents/external-tools/:id/test", ApplicationCode: "agent", ResourceType: "tool", Action: "update", RiskLevel: RiskHigh},
	{Name: "workspace.agent_external_tool.discover", Method: "POST", Path: "/v1/workspace/agents/external-tools/:id/discover", ApplicationCode: "agent", ResourceType: "tool", Action: "update", RiskLevel: RiskHigh},
	{Name: "workspace.agent_external_tool.delete", Method: "DELETE", Path: "/v1/workspace/agents/external-tools/:id", ApplicationCode: "agent", ResourceType: "tool", Action: "delete", RiskLevel: RiskHigh},
	{Name: "workflow.form_definition_draft.read", Method: "GET", Path: "/v1/form-builder/drafts", ApplicationCode: "workflow", ResourceType: "form_definition_draft", Action: "read"},
	{Name: "workflow.form_definition_draft.submit_review", Method: "POST", Path: "/v1/form-builder/drafts/:id/submit-review", ApplicationCode: "workflow", ResourceType: "form_definition_draft", Action: "submit", RiskLevel: RiskHigh},
	{Name: "workflow.form_definition_draft.publish", Method: "POST", Path: "/v1/form-builder/drafts/:id/publish", ApplicationCode: "workflow", ResourceType: "form_definition_draft", Action: "approve", RiskLevel: RiskCritical},
	{Name: "workflow.form_data_sources.read", Method: "GET", Path: "/v1/workflows/form-data-sources", ApplicationCode: "workflow", ResourceType: "form_instance", Action: "read"},
	{Name: "workflow.form_instance.runtime_template", Method: "GET", Path: "/v1/workflows/form-templates/:key", ApplicationCode: "workflow", ResourceType: "form_instance", Action: "read"},
	{Name: "workflow.form_instance.detail", Method: "GET", Path: "/v1/workflows/forms/:id", ApplicationCode: "workflow", ResourceType: "form_instance", Action: "read"},
	{Name: "workflow.review_queue.read", Method: "GET", Path: "/v1/workflows/reviews", ApplicationCode: "workflow", ResourceType: "form_instance", Action: "read"},
	{Name: "workflow.review_queue.bulk_action", Method: "POST", Path: "/v1/workflows/reviews/bulk-action", ApplicationCode: "workflow", ResourceType: "form_instance", Action: "approve", RiskLevel: RiskHigh},
	{Name: "workflow.form_instance.draft_create", Method: "POST", Path: "/v1/workflows/forms/drafts", ApplicationCode: "workflow", ResourceType: "form_instance", Action: "submit"},
	{Name: "workflow.form_instance.workflow_state", Method: "GET", Path: "/v1/workflows/forms/:id/workflow", ApplicationCode: "workflow", ResourceType: "form_instance", Action: "read"},
	{Name: "workflow.form_instance.export", Method: "GET", Path: "/v1/workflows/forms/:id/export", ApplicationCode: "workflow", ResourceType: "form_instance", Action: "read"},
	{Name: "workflow.form_instance.update", Method: "PATCH", Path: "/v1/workflows/forms/:id", ApplicationCode: "workflow", ResourceType: "form_instance", Action: "update"},
	{Name: "workflow.form_instance.delete", Method: "DELETE", Path: "/v1/workflows/forms/:id", ApplicationCode: "workflow", ResourceType: "form_instance", Action: "delete"},
	{Name: "workflow.form_instance.submit", Method: "POST", Path: "/v1/workflows/forms/:id/submit", ApplicationCode: "workflow", ResourceType: "form_instance", Action: "submit"},
	{Name: "workflow.form_instance.approve", Method: "POST", Path: "/v1/workflows/forms/:id/approve", ApplicationCode: "workflow", ResourceType: "form_instance", Action: "approve", RiskLevel: RiskHigh},
	{Name: "workflow.form_instance.reject", Method: "POST", Path: "/v1/workflows/forms/:id/reject", ApplicationCode: "workflow", ResourceType: "form_instance", Action: "approve", RiskLevel: RiskHigh},
	{Name: "workflow.form_instance.return", Method: "POST", Path: "/v1/workflows/forms/:id/return", ApplicationCode: "workflow", ResourceType: "form_instance", Action: "approve", RiskLevel: RiskHigh},
	{Name: "workflow.form_instance.cancel", Method: "POST", Path: "/v1/workflows/forms/:id/cancel", ApplicationCode: "workflow", ResourceType: "form_instance", Action: "update"},
	{Name: "workflow.form_instance.duplicate", Method: "POST", Path: "/v1/workflows/forms/:id/duplicate", ApplicationCode: "workflow", ResourceType: "form_instance", Action: "submit"},
	{Name: "workflow.form_instance.files_upload", Method: "POST", Path: "/v1/workflows/forms/:id/files", ApplicationCode: "workflow", ResourceType: "form_instance", Action: "submit"},
	{Name: "workflow.form_instance.files_download", Method: "GET", Path: "/v1/workflows/forms/:id/files/:file_id", ApplicationCode: "workflow", ResourceType: "form_instance", Action: "read"},
	{Name: "workflow.form_instance.files_delete", Method: "DELETE", Path: "/v1/workflows/forms/:id/files/:file_id", ApplicationCode: "workflow", ResourceType: "form_instance", Action: "submit"},
	{Name: "agent.run.chat", Method: "POST", Path: "/v1/agents/chat", ApplicationCode: "agent", ResourceType: "run", Action: "create"},
	{Name: "agent.confirmation.execute", Method: "POST", Path: "/v1/agents/confirmations/:id/execute", ApplicationCode: "agent", ResourceType: "run", Action: "create", RiskLevel: RiskHigh},
	{Name: "agent.session.read", Method: "GET", Path: "/v1/agents/sessions", ApplicationCode: "agent", ResourceType: "run", Action: "read"},
	{Name: "agent.session.create", Method: "POST", Path: "/v1/agents/sessions", ApplicationCode: "agent", ResourceType: "run", Action: "create"},
	{Name: "agent.session.clear_context", Method: "POST", Path: "/v1/agents/sessions/:id/clear-context", ApplicationCode: "agent", ResourceType: "run", Action: "create"},
	{Name: "agent.session.delete", Method: "DELETE", Path: "/v1/agents/sessions/:id", ApplicationCode: "agent", ResourceType: "run", Action: "delete"},
	{Name: "agent.session.messages", Method: "GET", Path: "/v1/agents/sessions/:id/messages", ApplicationCode: "agent", ResourceType: "run", Action: "read"},
	{Name: "agent.session.file_upload", Method: "POST", Path: "/v1/agents/sessions/:id/files", ApplicationCode: "agent", ResourceType: "run", Action: "create"},
	{Name: "agent.session.file_download", Method: "GET", Path: "/v1/agents/sessions/:id/files/:file_id", ApplicationCode: "agent", ResourceType: "run", Action: "read"},
	{Name: "agent.session.file_delete", Method: "DELETE", Path: "/v1/agents/sessions/:id/files/:file_id", ApplicationCode: "agent", ResourceType: "run", Action: "create"},
	{Name: "agent.memory.read", Method: "GET", Path: "/v1/agents/memories", ApplicationCode: "agent", ResourceType: "run", Action: "read"},
}
