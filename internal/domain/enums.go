package domain

import "strings"

type Effect string
type Severity string
type PrincipalType string
type Scope string
type ApplicationCode string
type ResourceType string
type Action string
type FieldPolicyEffect string
type AccountStatus string
type EventType string
type EmployeeStatus string
type EmployeeCategory string

const (
	EffectAllow Effect = "allow"
	EffectDeny  Effect = "deny"
)

const (
	SeverityLow      Severity = "low"
	SeverityMedium   Severity = "medium"
	SeverityHigh     Severity = "high"
	SeverityCritical Severity = "critical"
)

const (
	PrincipalTypeAccount       PrincipalType = "account"
	PrincipalTypeUserGroup     PrincipalType = "user_group"
	PrincipalTypeAssumableRole PrincipalType = "assumable_role"
)

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

const (
	AppPlatform   ApplicationCode = "platform"
	AppHR         ApplicationCode = "hr"
	AppIAM        ApplicationCode = "iam"
	AppAttendance ApplicationCode = "attendance"
	AppAgent      ApplicationCode = "agent"
	AppWorkflow   ApplicationCode = "workflow"
	AppAudit      ApplicationCode = "audit"
)

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

const (
	FieldPolicyEffectAllow    FieldPolicyEffect = "allow"
	FieldPolicyEffectDeny     FieldPolicyEffect = "deny"
	FieldPolicyEffectMask     FieldPolicyEffect = "mask"
	FieldPolicyEffectHide     FieldPolicyEffect = "hide"
	FieldPolicyEffectReadonly FieldPolicyEffect = "readonly"
)

const (
	AccountStatusActive        AccountStatus = "active"
	AccountStatusDisabled      AccountStatus = "disabled"
	AccountStatusPendingInvite AccountStatus = "pending_invite"
)

const (
	EventEmployeeCreated            EventType = "employee.created"
	EventEmployeeUpdated            EventType = "employee.updated"
	EventEmployeeInvited            EventType = "employee.invited"
	EventEmployeeImported           EventType = "employee.imported"
	EventEmployeeOffboarded         EventType = "employee.offboarded"
	EventEmployeeReinstated         EventType = "employee.reinstated"
	EventEmployeeStatusChanged      EventType = "employee.status_changed"
	EventEmployeeAuthzSubjectCreate EventType = "hr.employee.authz_subject.create"
	EventEmployeeAuthzSubjectUpdate EventType = "hr.employee.authz_subject.update"
	EventEmployeeAuthzSubjectInvite EventType = "hr.employee.authz_subject.invite"
	EventEmployeeAuthzSubjectImport EventType = "hr.employee.authz_subject.import"
	EventOpenFGARelationshipWrite   EventType = "openfga.relationship.write"
	EventOpenFGARelationshipDelete  EventType = "openfga.relationship.delete"
)

const (
	EmployeeStatusActive         EmployeeStatus = "active"
	EmployeeStatusProbation      EmployeeStatus = "probation"
	EmployeeStatusLeaveSuspended EmployeeStatus = "leave_suspended"
	EmployeeStatusOnboarding     EmployeeStatus = "onboarding"
	EmployeeStatusResigned       EmployeeStatus = "resigned"
	EmployeeStatusDeleted        EmployeeStatus = "deleted"
)

const (
	EmployeeCategoryFullTime   EmployeeCategory = "full_time"
	EmployeeCategoryPartTime   EmployeeCategory = "part_time"
	EmployeeCategoryIntern     EmployeeCategory = "intern"
	EmployeeCategoryContractor EmployeeCategory = "contractor"
	EmployeeCategoryOther      EmployeeCategory = "other"
)

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

func ParseEmployeeStatus(raw string) (EmployeeStatus, bool) {
	switch strings.TrimSpace(raw) {
	case "在職", "active":
		return EmployeeStatusActive, true
	case "試用中", "probation":
		return EmployeeStatusProbation, true
	case "留停", "on-leave", "leave_suspended":
		return EmployeeStatusLeaveSuspended, true
	case "待加入", "pending", "onboarding":
		return EmployeeStatusOnboarding, true
	case "離職", "resigned":
		return EmployeeStatusResigned, true
	case "已停用", "deleted":
		return EmployeeStatusDeleted, true
	default:
		return "", false
	}
}

func NormalizeEmployeeStatus(raw string) string {
	if status, ok := ParseEmployeeStatus(raw); ok {
		return string(status)
	}
	return strings.TrimSpace(raw)
}

func (s EmployeeStatus) Valid(includeDeleted bool) bool {
	switch s {
	case EmployeeStatusActive, EmployeeStatusProbation, EmployeeStatusLeaveSuspended, EmployeeStatusOnboarding, EmployeeStatusResigned:
		return true
	case EmployeeStatusDeleted:
		return includeDeleted
	default:
		return false
	}
}

func EmployeeStatuses(includeDeleted bool) []string {
	statuses := []string{
		string(EmployeeStatusActive),
		string(EmployeeStatusProbation),
		string(EmployeeStatusLeaveSuspended),
		string(EmployeeStatusOnboarding),
		string(EmployeeStatusResigned),
	}
	if includeDeleted {
		statuses = append(statuses, string(EmployeeStatusDeleted))
	}
	return statuses
}

func ParseEmployeeCategory(raw string) (EmployeeCategory, bool) {
	switch strings.TrimSpace(raw) {
	case "全職", "正職", "full-time", "full_time":
		return EmployeeCategoryFullTime, true
	case "兼職", "part-time", "part_time":
		return EmployeeCategoryPartTime, true
	case "實習", "intern":
		return EmployeeCategoryIntern, true
	case "約聘", "contract", "contractor":
		return EmployeeCategoryContractor, true
	case "其他", "other":
		return EmployeeCategoryOther, true
	default:
		return "", false
	}
}

func NormalizeEmployeeCategory(raw string) string {
	if category, ok := ParseEmployeeCategory(raw); ok {
		return string(category)
	}
	return strings.TrimSpace(raw)
}

func (c EmployeeCategory) Valid() bool {
	switch c {
	case EmployeeCategoryFullTime, EmployeeCategoryPartTime, EmployeeCategoryIntern, EmployeeCategoryContractor, EmployeeCategoryOther:
		return true
	default:
		return false
	}
}

func EmployeeCategories() []string {
	return []string{
		string(EmployeeCategoryFullTime),
		string(EmployeeCategoryPartTime),
		string(EmployeeCategoryIntern),
		string(EmployeeCategoryContractor),
		string(EmployeeCategoryOther),
	}
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
