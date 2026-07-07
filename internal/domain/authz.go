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

// 下列常數定義此模組使用的固定值。
const (
	ResourceEmployee                  ResourceType = "employee"
	ResourceEmployeeImport            ResourceType = "employee_import_session"
	ResourceOrgUnit                   ResourceType = "org_unit"
	ResourcePosition                  ResourceType = "position"
	ResourceEmploymentContract        ResourceType = "employment_contract"
	ResourceLeave                     ResourceType = "leave"
	ResourceAttendanceWorksite        ResourceType = "worksite"
	ResourceAttendanceShift           ResourceType = "shift"
	ResourceAttendanceShiftAssignment ResourceType = "shift_assignment"
	ResourceAttendanceClock           ResourceType = "clock"
	ResourceAttendanceCorrection      ResourceType = "correction"
	ResourceUserGroup                 ResourceType = "user_group"
	ResourcePermission                ResourceType = "permission"
	ResourcePermissionSet             ResourceType = "permission_set"
	ResourcePermissionAssign          ResourceType = "permission_set_assignment"
	ResourceDataScope                 ResourceType = "data_scope"
	ResourceFieldPolicy               ResourceType = "field_policy"
	ResourceAssumableRole             ResourceType = "assumable_role"
	ResourceTool                      ResourceType = "tool"
	ResourceEmployeeCollection        ResourceType = "employee_collection"
	ResourceFormInstance              ResourceType = "form_instance"
	ResourceNotification              ResourceType = "notification"
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

// AuditEvent 處理稽核事件。
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
	{Name: "me.menus", Method: "GET", Path: "/v1/me/menus", ApplicationCode: "platform", ResourceType: "me", Action: "read"},
	{Name: "authz.check", Method: "POST", Path: "/v1/authz/check", ApplicationCode: "iam", ResourceType: "authz", Action: "check"},
	{Name: "authz.batch_check", Method: "POST", Path: "/v1/authz/batch-check", ApplicationCode: "iam", ResourceType: "authz", Action: "check"},
	{Name: "authz.explain", Method: "POST", Path: "/v1/authz/explain", ApplicationCode: "iam", ResourceType: "authz", Action: "explain"},
	{Name: "authz.simulate", Method: "POST", Path: "/v1/authz/simulate", ApplicationCode: "iam", ResourceType: "authz", Action: "simulate", RiskLevel: RiskHigh},
	{Name: "iam.permission.read", Method: "GET", Path: "/v1/iam/permissions", ApplicationCode: "iam", ResourceType: "permission", Action: "read"},
	{Name: "iam.roles.read", Method: "GET", Path: "/v1/iam/roles", ApplicationCode: "iam", ResourceType: "assumable_role", Action: "read"},
	{Name: "iam.role_bindings.read", Method: "GET", Path: "/v1/iam/role-bindings", ApplicationCode: "iam", ResourceType: "permission_set_assignment", Action: "read"},
	{Name: "iam.user_group.read", Method: "GET", Path: "/v1/iam/user-groups", ApplicationCode: "iam", ResourceType: "user_group", Action: "read"},
	{Name: "iam.user_group.create", Method: "POST", Path: "/v1/iam/user-groups", ApplicationCode: "iam", ResourceType: "user_group", Action: "create", RiskLevel: RiskHigh},
	{Name: "iam.permission_set.read", Method: "GET", Path: "/v1/iam/permission-sets", ApplicationCode: "iam", ResourceType: "permission_set", Action: "read"},
	{Name: "iam.permission_set.create", Method: "POST", Path: "/v1/iam/permission-sets", ApplicationCode: "iam", ResourceType: "permission_set", Action: "create", RiskLevel: RiskHigh},
	{Name: "iam.permission_assignment.read", Method: "GET", Path: "/v1/iam/permission-set-assignments", ApplicationCode: "iam", ResourceType: "permission_set_assignment", Action: "read"},
	{Name: "iam.permission_assignment.create", Method: "POST", Path: "/v1/iam/permission-set-assignments", ApplicationCode: "iam", ResourceType: "permission_set_assignment", Action: "create", RiskLevel: RiskHigh},
	{Name: "iam.data_scope.read", Method: "GET", Path: "/v1/iam/data-scopes", ApplicationCode: "iam", ResourceType: "data_scope", Action: "read"},
	{Name: "iam.data_scope.create", Method: "POST", Path: "/v1/iam/data-scopes", ApplicationCode: "iam", ResourceType: "data_scope", Action: "create", RiskLevel: RiskHigh},
	{Name: "iam.field_policy.read", Method: "GET", Path: "/v1/iam/field-policies", ApplicationCode: "iam", ResourceType: "field_policy", Action: "read"},
	{Name: "iam.field_policy.create", Method: "POST", Path: "/v1/iam/field-policies", ApplicationCode: "iam", ResourceType: "field_policy", Action: "create", RiskLevel: RiskHigh},
	{Name: "iam.assumable_role.read", Method: "GET", Path: "/v1/iam/assumable-roles", ApplicationCode: "iam", ResourceType: "assumable_role", Action: "read"},
	{Name: "iam.assumable_role.create", Method: "POST", Path: "/v1/iam/assumable-roles", ApplicationCode: "iam", ResourceType: "assumable_role", Action: "create", RiskLevel: RiskHigh},
	{Name: "iam.assumable_role.assume", Method: "POST", Path: "/v1/iam/assumable-roles/:id/assume", ApplicationCode: "iam", ResourceType: "assumable_role", Action: "assume", RiskLevel: RiskHigh},
	{Name: "hr.position.read", Method: "GET", Path: "/v1/hr/positions", ApplicationCode: "hr", ResourceType: "position", Action: "read"},
	{Name: "hr.position.create", Method: "POST", Path: "/v1/hr/positions", ApplicationCode: "hr", ResourceType: "position", Action: "create"},
	{Name: "hr.position.detail", Method: "GET", Path: "/v1/hr/positions/:id", ApplicationCode: "hr", ResourceType: "position", Action: "read"},
	{Name: "hr.position.update", Method: "PATCH", Path: "/v1/hr/positions/:id", ApplicationCode: "hr", ResourceType: "position", Action: "update"},
	{Name: "hr.position.delete", Method: "DELETE", Path: "/v1/hr/positions/:id", ApplicationCode: "hr", ResourceType: "position", Action: "delete", RiskLevel: RiskHigh},
	{Name: "hr.employee.read", Method: "GET", Path: "/v1/hr/employees", ApplicationCode: "hr", ResourceType: "employee", Action: "read"},
	{Name: "hr.employee.detail", Method: "GET", Path: "/v1/hr/employees/:id", ApplicationCode: "hr", ResourceType: "employee", Action: "read"},
	{Name: "hr.employee.create", Method: "POST", Path: "/v1/hr/employees", ApplicationCode: "hr", ResourceType: "employee", Action: "create"},
	{Name: "hr.employee.preview_create", Method: "POST", Path: "/v1/hr/employees/preview", ApplicationCode: "hr", ResourceType: "employee", Action: "create"},
	{Name: "hr.employee.update", Method: "PATCH", Path: "/v1/hr/employees/:id", ApplicationCode: "hr", ResourceType: "employee", Action: "update"},
	{Name: "hr.employee.preview_update", Method: "POST", Path: "/v1/hr/employees/:id/preview", ApplicationCode: "hr", ResourceType: "employee", Action: "update"},
	{Name: "hr.employee.avatar_update", Method: "POST", Path: "/v1/hr/employees/:id/avatar", ApplicationCode: "hr", ResourceType: "employee", Action: "update"},
	{Name: "hr.employee.avatar_delete", Method: "DELETE", Path: "/v1/hr/employees/:id/avatar", ApplicationCode: "hr", ResourceType: "employee", Action: "update"},
	{Name: "hr.employee.stats", Method: "GET", Path: "/v1/hr/employees/stats", ApplicationCode: "hr", ResourceType: "employee", Action: "read"},
	{Name: "hr.employee.options", Method: "GET", Path: "/v1/hr/employee-options", ApplicationCode: "hr", ResourceType: "employee", Action: "read"},
	{Name: "hr.employee.import_template", Method: "GET", Path: "/v1/hr/employees/import/template", ApplicationCode: "hr", ResourceType: "employee", Action: "read"},
	{Name: "hr.employee.import_preview", Method: "POST", Path: "/v1/hr/employees/import/preview", ApplicationCode: "hr", ResourceType: "employee", Action: "import", RiskLevel: RiskHigh},
	{Name: "hr.employee.import_confirm", Method: "POST", Path: "/v1/hr/employees/import/:id/confirm", ApplicationCode: "hr", ResourceType: "employee", Action: "import", RiskLevel: RiskHigh},
	{Name: "hr.employee.ehrms_sync", Method: "POST", Path: "/v1/hr/employees/ehrms/sync", ApplicationCode: "hr", ResourceType: "employee", Action: "import", RiskLevel: RiskHigh},
	{Name: "hr.employee.export_download", Method: "GET", Path: "/v1/hr/employees/export", ApplicationCode: "hr", ResourceType: "employee", Action: "export", RiskLevel: RiskHigh},
	{Name: "hr.employee.export", Method: "POST", Path: "/v1/hr/employees/export", ApplicationCode: "hr", ResourceType: "employee", Action: "export", RiskLevel: RiskHigh},
	{Name: "hr.employee.batch_delete", Method: "POST", Path: "/v1/hr/employees/batch-delete", ApplicationCode: "hr", ResourceType: "employee", Action: "delete", RiskLevel: RiskHigh},
	{Name: "hr.employee.delete", Method: "DELETE", Path: "/v1/hr/employees/:id", ApplicationCode: "hr", ResourceType: "employee", Action: "delete", RiskLevel: RiskHigh},
	{Name: "hr.employee.update_status", Method: "PATCH", Path: "/v1/hr/employees/:id/status", ApplicationCode: "hr", ResourceType: "employee", Action: "update_status", RiskLevel: RiskHigh},
	{Name: "hr.employee.invite", Method: "POST", Path: "/v1/hr/employees/:id/invite", ApplicationCode: "hr", ResourceType: "employee", Action: "invite", RiskLevel: RiskHigh},
	{Name: "hr.employee.status_transition", Method: "POST", Path: "/v1/hr/employees/:id/status-transition", ApplicationCode: "hr", ResourceType: "employee", Action: "status_transition", RiskLevel: RiskHigh},
	{Name: "hr.contract.read_employee", Method: "GET", Path: "/v1/hr/employees/:id/contracts", ApplicationCode: "hr", ResourceType: "employment_contract", Action: "read", RiskLevel: RiskHigh},
	{Name: "hr.contract.create", Method: "POST", Path: "/v1/hr/employees/:id/contracts", ApplicationCode: "hr", ResourceType: "employment_contract", Action: "create", RiskLevel: RiskHigh},
	{Name: "hr.contract.detail", Method: "GET", Path: "/v1/hr/contracts/:id", ApplicationCode: "hr", ResourceType: "employment_contract", Action: "read", RiskLevel: RiskHigh},
	{Name: "hr.contract.update", Method: "PATCH", Path: "/v1/hr/contracts/:id", ApplicationCode: "hr", ResourceType: "employment_contract", Action: "update", RiskLevel: RiskHigh},
	{Name: "hr.contract.delete", Method: "DELETE", Path: "/v1/hr/contracts/:id", ApplicationCode: "hr", ResourceType: "employment_contract", Action: "delete", RiskLevel: RiskHigh},
	{Name: "hr.org_unit.read", Method: "GET", Path: "/v1/org/units", ApplicationCode: "hr", ResourceType: "org_unit", Action: "read"},
	{Name: "hr.org_unit.create", Method: "POST", Path: "/v1/org/units", ApplicationCode: "hr", ResourceType: "org_unit", Action: "create"},
	{Name: "attendance.leave.read_balance", Method: "GET", Path: "/v1/attendance/leave-balances", ApplicationCode: "attendance", ResourceType: "leave", Action: "read"},
	{Name: "attendance.leave.read_request", Method: "GET", Path: "/v1/attendance/leave-requests", ApplicationCode: "attendance", ResourceType: "leave", Action: "read"},
	{Name: "attendance.leave.create", Method: "POST", Path: "/v1/attendance/leave-requests", ApplicationCode: "attendance", ResourceType: "leave", Action: "create"},
	{Name: "attendance.overtime.read", Method: "GET", Path: "/v1/attendance/overtime-requests", ApplicationCode: "attendance", ResourceType: "leave", Action: "read"},
	{Name: "attendance.overtime.create", Method: "POST", Path: "/v1/attendance/overtime-requests", ApplicationCode: "attendance", ResourceType: "leave", Action: "create"},
	{Name: "attendance.policy.read", Method: "GET", Path: "/v1/attendance/policies/current", ApplicationCode: "attendance", ResourceType: "leave", Action: "read"},
	{Name: "attendance.policy.update", Method: "PATCH", Path: "/v1/attendance/policies/current", ApplicationCode: "attendance", ResourceType: "leave", Action: "update", RiskLevel: RiskHigh},
	{Name: "attendance.worksite.read", Method: "GET", Path: "/v1/attendance/worksites", ApplicationCode: "attendance", ResourceType: "worksite", Action: "read"},
	{Name: "attendance.worksite.create", Method: "POST", Path: "/v1/attendance/worksites", ApplicationCode: "attendance", ResourceType: "worksite", Action: "create"},
	{Name: "attendance.worksite.update", Method: "PATCH", Path: "/v1/attendance/worksites", ApplicationCode: "attendance", ResourceType: "worksite", Action: "update"},
	{Name: "attendance.shift.read", Method: "GET", Path: "/v1/attendance/shifts", ApplicationCode: "attendance", ResourceType: "shift", Action: "read"},
	{Name: "attendance.shift.create", Method: "POST", Path: "/v1/attendance/shifts", ApplicationCode: "attendance", ResourceType: "shift", Action: "create"},
	{Name: "attendance.shift.update", Method: "PATCH", Path: "/v1/attendance/shifts", ApplicationCode: "attendance", ResourceType: "shift", Action: "update"},
	{Name: "attendance.shift_assignment.read", Method: "GET", Path: "/v1/attendance/shift-assignments", ApplicationCode: "attendance", ResourceType: "shift_assignment", Action: "read"},
	{Name: "attendance.shift_assignment.create", Method: "POST", Path: "/v1/attendance/shift-assignments", ApplicationCode: "attendance", ResourceType: "shift_assignment", Action: "create"},
	{Name: "attendance.clock.status", Method: "GET", Path: "/v1/attendance/clock-status", ApplicationCode: "attendance", ResourceType: "clock", Action: "read"},
	{Name: "attendance.clock.read", Method: "GET", Path: "/v1/attendance/clock-records", ApplicationCode: "attendance", ResourceType: "clock", Action: "read"},
	{Name: "attendance.clock.create", Method: "POST", Path: "/v1/attendance/clock-records", ApplicationCode: "attendance", ResourceType: "clock", Action: "create"},
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
	{Name: "workspace.read", Method: "GET", Path: "/v1/workspace", ApplicationCode: "hr", ResourceType: "employee", Action: "read"},
	{Name: "workspace.employees.read", Method: "GET", Path: "/v1/workspace/employees", ApplicationCode: "hr", ResourceType: "employee", Action: "read"},
	{Name: "workspace.organization.read", Method: "GET", Path: "/v1/workspace/organization", ApplicationCode: "hr", ResourceType: "employee", Action: "read"},
	{Name: "workspace.organization_manager.update", Method: "PATCH", Path: "/v1/workspace/organization/employees/:id/manager", ApplicationCode: "hr", ResourceType: "employee", Action: "update"},
	{Name: "workspace.turnover.read", Method: "GET", Path: "/v1/workspace/turnover", ApplicationCode: "hr", ResourceType: "employee", Action: "read"},
	{Name: "workspace.attendance.read", Method: "GET", Path: "/v1/workspace/attendance", ApplicationCode: "attendance", ResourceType: "clock", Action: "read"},
	{Name: "workspace.admins.read", Method: "GET", Path: "/v1/workspace/admins", ApplicationCode: "iam", ResourceType: "permission_set_assignment", Action: "read"},
	{Name: "workspace.admin.create", Method: "POST", Path: "/v1/workspace/admins", ApplicationCode: "iam", ResourceType: "permission_set_assignment", Action: "create", RiskLevel: RiskHigh},
	{Name: "workspace.admin.update", Method: "PATCH", Path: "/v1/workspace/admins/:id/permissions", ApplicationCode: "iam", ResourceType: "permission_set_assignment", Action: "update", RiskLevel: RiskHigh},
	{Name: "workspace.admin.delete", Method: "DELETE", Path: "/v1/workspace/admins/:id", ApplicationCode: "iam", ResourceType: "permission_set_assignment", Action: "delete", RiskLevel: RiskHigh},
	{Name: "workspace.form.read", Method: "GET", Path: "/v1/workspace/forms", ApplicationCode: "workflow", ResourceType: "form_template", Action: "read"},
	{Name: "workspace.form.create", Method: "POST", Path: "/v1/workspace/forms", ApplicationCode: "workflow", ResourceType: "form_template", Action: "create"},
	{Name: "workspace.form.update", Method: "PATCH", Path: "/v1/workspace/forms/:id", ApplicationCode: "workflow", ResourceType: "form_template", Action: "update"},
	{Name: "workspace.form.delete", Method: "DELETE", Path: "/v1/workspace/forms/:id", ApplicationCode: "workflow", ResourceType: "form_template", Action: "delete", RiskLevel: RiskHigh},
	{Name: "workspace.audit_logs.read", Method: "GET", Path: "/v1/workspace/audit-logs", ApplicationCode: "audit", ResourceType: "audit_log", Action: "read"},
	{Name: "workspace.insights.read", Method: "GET", Path: "/v1/workspace/insights", ApplicationCode: "hr", ResourceType: "employee", Action: "read"},
	{Name: "workflow.form_template.read", Method: "GET", Path: "/v1/forms/templates", ApplicationCode: "workflow", ResourceType: "form_template", Action: "read"},
	{Name: "workflow.form_template.create", Method: "POST", Path: "/v1/forms/templates", ApplicationCode: "workflow", ResourceType: "form_template", Action: "create"},
	{Name: "workflow.form_instance.read", Method: "GET", Path: "/v1/workflows/forms", ApplicationCode: "workflow", ResourceType: "form_instance", Action: "read"},
	{Name: "workflow.review_queue.read", Method: "GET", Path: "/v1/workflows/reviews", ApplicationCode: "workflow", ResourceType: "form_instance", Action: "read"},
	{Name: "workflow.review_queue.bulk_action", Method: "POST", Path: "/v1/workflows/reviews/bulk-action", ApplicationCode: "workflow", ResourceType: "form_instance", Action: "update"},
	{Name: "workflow.form_instance.draft_create", Method: "POST", Path: "/v1/workflows/forms/drafts", ApplicationCode: "workflow", ResourceType: "form_instance", Action: "submit"},
	{Name: "workflow.form_instance.workflow_state", Method: "GET", Path: "/v1/workflows/forms/:id/workflow", ApplicationCode: "workflow", ResourceType: "form_instance", Action: "read"},
	{Name: "workflow.form_instance.export", Method: "GET", Path: "/v1/workflows/forms/:id/export", ApplicationCode: "workflow", ResourceType: "form_instance", Action: "read"},
	{Name: "workflow.form_instance.update", Method: "PATCH", Path: "/v1/workflows/forms/:id", ApplicationCode: "workflow", ResourceType: "form_instance", Action: "update"},
	{Name: "workflow.form_instance.delete", Method: "DELETE", Path: "/v1/workflows/forms/:id", ApplicationCode: "workflow", ResourceType: "form_instance", Action: "delete"},
	{Name: "workflow.form_instance.submit", Method: "POST", Path: "/v1/workflows/forms/:id/submit", ApplicationCode: "workflow", ResourceType: "form_instance", Action: "submit"},
	{Name: "workflow.form_instance.approve", Method: "POST", Path: "/v1/workflows/forms/:id/approve", ApplicationCode: "workflow", ResourceType: "form_instance", Action: "approve"},
	{Name: "workflow.form_instance.reject", Method: "POST", Path: "/v1/workflows/forms/:id/reject", ApplicationCode: "workflow", ResourceType: "form_instance", Action: "update"},
	{Name: "workflow.form_instance.return", Method: "POST", Path: "/v1/workflows/forms/:id/return", ApplicationCode: "workflow", ResourceType: "form_instance", Action: "update"},
	{Name: "workflow.form_instance.cancel", Method: "POST", Path: "/v1/workflows/forms/:id/cancel", ApplicationCode: "workflow", ResourceType: "form_instance", Action: "update"},
	{Name: "workflow.form_instance.duplicate", Method: "POST", Path: "/v1/workflows/forms/:id/duplicate", ApplicationCode: "workflow", ResourceType: "form_instance", Action: "submit"},
	{Name: "agent.run.read", Method: "GET", Path: "/v1/agents/runs", ApplicationCode: "agent", ResourceType: "run", Action: "read"},
	{Name: "agent.run.create", Method: "POST", Path: "/v1/agents/runs", ApplicationCode: "agent", ResourceType: "run", Action: "create", RiskLevel: RiskHigh},
	{Name: "audit.log.read", Method: "GET", Path: "/v1/audit-logs", ApplicationCode: "audit", ResourceType: "audit_log", Action: "read"},
}
