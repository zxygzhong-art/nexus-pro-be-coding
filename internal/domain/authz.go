package domain

import "strings"

// Effect describes whether a permission grants or denies access.
type Effect string

// Severity describes the audit severity for an authorization-relevant action.
type Severity string

// PrincipalType identifies the kind of IAM principal receiving permissions.
type PrincipalType string

// Scope identifies the data visibility boundary for a permission.
type Scope string

// ApplicationCode identifies the product area owning a permission.
type ApplicationCode string

// ResourceType identifies the kind of protected resource.
type ResourceType string

// Action identifies the operation requested on a resource.
type Action string

// FieldPolicyEffect identifies how a field policy transforms field visibility.
type FieldPolicyEffect string

// EventType identifies domain events emitted for audit or authorization sync.
type EventType string

// Permission effects.
const (
	EffectAllow Effect = "allow"
	EffectDeny  Effect = "deny"
)

// Audit severities.
const (
	SeverityLow      Severity = "low"
	SeverityMedium   Severity = "medium"
	SeverityHigh     Severity = "high"
	SeverityCritical Severity = "critical"
)

// IAM principal types.
const (
	PrincipalTypeAccount       PrincipalType = "account"
	PrincipalTypeUserGroup     PrincipalType = "user_group"
	PrincipalTypeAssumableRole PrincipalType = "assumable_role"
)

// Data scopes used by authorization decisions.
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

// Application codes used in route policies and permissions.
const (
	AppPlatform   ApplicationCode = "platform"
	AppHR         ApplicationCode = "hr"
	AppIAM        ApplicationCode = "iam"
	AppAttendance ApplicationCode = "attendance"
	AppAgent      ApplicationCode = "agent"
	AppWorkflow   ApplicationCode = "workflow"
	AppAudit      ApplicationCode = "audit"
)

// Resource types used in route policies and permissions.
const (
	ResourceEmployee           ResourceType = "employee"
	ResourceEmployeeImport     ResourceType = "employee_import_session"
	ResourceOrgUnit            ResourceType = "org_unit"
	ResourceLeave              ResourceType = "leave"
	ResourceUserGroup          ResourceType = "user_group"
	ResourcePermissionSet      ResourceType = "permission_set"
	ResourcePermissionAssign   ResourceType = "permission_set_assignment"
	ResourceDataScope          ResourceType = "data_scope"
	ResourceFieldPolicy        ResourceType = "field_policy"
	ResourceAssumableRole      ResourceType = "assumable_role"
	ResourceTool               ResourceType = "tool"
	ResourceKnowledgeArticle   ResourceType = "knowledge_article"
	ResourceEmployeeCollection ResourceType = "employee_collection"
	ResourceFormInstance       ResourceType = "form_instance"
)

// Action values used in route policies and permissions.
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

// Field policy effects.
const (
	FieldPolicyEffectAllow    FieldPolicyEffect = "allow"
	FieldPolicyEffectDeny     FieldPolicyEffect = "deny"
	FieldPolicyEffectMask     FieldPolicyEffect = "mask"
	FieldPolicyEffectHide     FieldPolicyEffect = "hide"
	FieldPolicyEffectReadonly FieldPolicyEffect = "readonly"
)

// OpenFGA relationship event types.
const (
	EventOpenFGARelationshipWrite  EventType = "openfga.relationship.write"
	EventOpenFGARelationshipDelete EventType = "openfga.relationship.delete"
)

// RiskLevel describes whether an action needs stronger approval handling.
type RiskLevel string

// Risk levels used by route policy metadata.
const (
	RiskNormal   RiskLevel = "normal"
	RiskHigh     RiskLevel = "high"
	RiskCritical RiskLevel = "critical"
)

// RoutePolicy binds an HTTP route to its authorization metadata.
type RoutePolicy struct {
	Name            string
	Method          string
	Path            string
	ApplicationCode string
	ResourceType    string
	Action          string
	RiskLevel       RiskLevel
}

// RelationshipCheck asks an external relationship engine about one tuple.
type RelationshipCheck struct {
	TenantID string
	Subject  string
	Relation string
	Object   string
}

// AuditEvent returns the canonical audit event name for the check request.
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

// DefaultRoutePolicies is the source-of-truth authorization metadata for API routes.
var DefaultRoutePolicies = []RoutePolicy{
	{Name: "me.read", Method: "GET", Path: "/v1/me", ApplicationCode: "platform", ResourceType: "me", Action: "read"},
	{Name: "me.menus", Method: "GET", Path: "/v1/me/menus", ApplicationCode: "platform", ResourceType: "me", Action: "read"},
	{Name: "authz.check", Method: "POST", Path: "/v1/authz/check", ApplicationCode: "iam", ResourceType: "authz", Action: "check"},
	{Name: "authz.batch_check", Method: "POST", Path: "/v1/authz/batch-check", ApplicationCode: "iam", ResourceType: "authz", Action: "check"},
	{Name: "authz.explain", Method: "POST", Path: "/v1/authz/explain", ApplicationCode: "iam", ResourceType: "authz", Action: "explain"},
	{Name: "authz.simulate", Method: "POST", Path: "/v1/authz/simulate", ApplicationCode: "iam", ResourceType: "authz", Action: "simulate", RiskLevel: RiskHigh},
	{Name: "iam.permission.read", Method: "GET", Path: "/v1/iam/permissions", ApplicationCode: "iam", ResourceType: "permission", Action: "read"},
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
	{Name: "hr.employee.export_download", Method: "GET", Path: "/v1/hr/employees/export", ApplicationCode: "hr", ResourceType: "employee", Action: "export", RiskLevel: RiskHigh},
	{Name: "hr.employee.export", Method: "POST", Path: "/v1/hr/employees/export", ApplicationCode: "hr", ResourceType: "employee", Action: "export", RiskLevel: RiskHigh},
	{Name: "hr.employee.batch_delete", Method: "POST", Path: "/v1/hr/employees/batch-delete", ApplicationCode: "hr", ResourceType: "employee", Action: "delete", RiskLevel: RiskHigh},
	{Name: "hr.employee.delete", Method: "DELETE", Path: "/v1/hr/employees/:id", ApplicationCode: "hr", ResourceType: "employee", Action: "delete", RiskLevel: RiskHigh},
	{Name: "hr.employee.update_status", Method: "PATCH", Path: "/v1/hr/employees/:id/status", ApplicationCode: "hr", ResourceType: "employee", Action: "update_status", RiskLevel: RiskHigh},
	{Name: "hr.employee.invite", Method: "POST", Path: "/v1/hr/employees/:id/invite", ApplicationCode: "hr", ResourceType: "employee", Action: "invite", RiskLevel: RiskHigh},
	{Name: "hr.employee.status_transition", Method: "POST", Path: "/v1/hr/employees/:id/status-transition", ApplicationCode: "hr", ResourceType: "employee", Action: "status_transition", RiskLevel: RiskHigh},
	{Name: "hr.org_unit.read", Method: "GET", Path: "/v1/org/units", ApplicationCode: "hr", ResourceType: "org_unit", Action: "read"},
	{Name: "hr.org_unit.create", Method: "POST", Path: "/v1/org/units", ApplicationCode: "hr", ResourceType: "org_unit", Action: "create"},
	{Name: "attendance.leave.read_balance", Method: "GET", Path: "/v1/attendance/leave-balances", ApplicationCode: "attendance", ResourceType: "leave", Action: "read"},
	{Name: "attendance.leave.read_request", Method: "GET", Path: "/v1/attendance/leave-requests", ApplicationCode: "attendance", ResourceType: "leave", Action: "read"},
	{Name: "attendance.leave.create", Method: "POST", Path: "/v1/attendance/leave-requests", ApplicationCode: "attendance", ResourceType: "leave", Action: "create"},
	{Name: "workflow.form_template.read", Method: "GET", Path: "/v1/forms/templates", ApplicationCode: "workflow", ResourceType: "form_template", Action: "read"},
	{Name: "workflow.form_template.create", Method: "POST", Path: "/v1/forms/templates", ApplicationCode: "workflow", ResourceType: "form_template", Action: "create"},
	{Name: "workflow.form_instance.submit", Method: "POST", Path: "/v1/workflows/forms/:id/submit", ApplicationCode: "workflow", ResourceType: "form_instance", Action: "submit"},
	{Name: "workflow.form_instance.approve", Method: "POST", Path: "/v1/workflows/forms/:id/approve", ApplicationCode: "workflow", ResourceType: "form_instance", Action: "approve"},
	{Name: "agent.run.read", Method: "GET", Path: "/v1/agents/runs", ApplicationCode: "agent", ResourceType: "run", Action: "read"},
	{Name: "agent.run.create", Method: "POST", Path: "/v1/agents/runs", ApplicationCode: "agent", ResourceType: "run", Action: "create", RiskLevel: RiskHigh},
	{Name: "audit.log.read", Method: "GET", Path: "/v1/audit-logs", ApplicationCode: "audit", ResourceType: "audit_log", Action: "read", RiskLevel: RiskHigh},
}
