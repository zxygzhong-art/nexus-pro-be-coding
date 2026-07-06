package v1_test

import (
	"context"
	"time"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/repository"
)

type (
	Tenant                    = domain.Tenant
	Account                   = domain.Account
	UserIdentity              = domain.UserIdentity
	UserGroup                 = domain.UserGroup
	PermissionSet             = domain.PermissionSet
	Permission                = domain.Permission
	OrgUnit                   = domain.OrgUnit
	Employee                  = domain.Employee
	LeaveBalance              = domain.LeaveBalance
	AttendanceWorksite        = domain.AttendanceWorksite
	AttendanceShift           = domain.AttendanceShift
	AttendanceShiftAssignment = domain.AttendanceShiftAssignment
	LeaveRequest              = domain.LeaveRequest
	AttendanceClockRecord     = domain.AttendanceClockRecord
	FormTemplate              = domain.FormTemplate
	KnowledgeArticle          = domain.KnowledgeArticle
	PlatformFormColumn        = domain.PlatformFormColumn
	PlatformFormItem          = domain.PlatformFormItem
)

const (
	fixtureClockDirectionIn     = "clock_in"
	fixtureClockDirectionOut    = "clock_out"
	fixtureClockRecordAccepted  = "accepted"
	fixtureClockSourceGeofence  = "geofence"
	fixtureClockDeviceDashboard = "fixture-dashboard"
	fixtureDashboardLeaveReason = "Dashboard fixture sample"
)

// populateDemoFixture 驗證 populate demo fixture。
func populateDemoFixture(store repository.Store) {
	ctx := context.Background()
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)

	_ = store.UpsertTenant(ctx, Tenant{
		ID:        "demo",
		Name:      "Demo Tenant",
		CreatedAt: now,
	})
	_ = store.UpsertTenant(ctx, Tenant{
		ID:        "alpha",
		Name:      "Alpha Tenant",
		CreatedAt: now.Add(2 * time.Minute),
	})

	adminSet := PermissionSet{
		ID:       "ps-admin",
		TenantID: "demo",
		Name:     "Platform Admin",
		Permissions: []Permission{
			{Resource: "*", Action: "*", Scope: "all", MenuKey: "workbench"},
			{Resource: "hr.employee", Action: "read", Scope: "all", MenuKey: "hr.employees"},
			{Resource: "hr.employee", Action: "create", Scope: "all", MenuKey: "hr.employees"},
			{Resource: "hr.org_unit", Action: "read", Scope: "all", MenuKey: "hr.org_units"},
			{Resource: "hr.org_unit", Action: "create", Scope: "all", MenuKey: "hr.org_units"},
			{Resource: "attendance.leave", Action: "read", Scope: "all", MenuKey: "attendance.leave"},
			{Resource: "attendance.leave", Action: "create", Scope: "all", MenuKey: "attendance.leave"},
			{Resource: "attendance.leave", Action: "update", Scope: "all", MenuKey: "attendance.leave"},
			{Resource: "attendance.worksite", Action: "read", Scope: "all", MenuKey: "attendance.worksites"},
			{Resource: "attendance.worksite", Action: "create", Scope: "all", MenuKey: "attendance.worksites"},
			{Resource: "attendance.worksite", Action: "update", Scope: "all", MenuKey: "attendance.worksites"},
			{Resource: "attendance.shift", Action: "read", Scope: "all", MenuKey: "attendance.shifts"},
			{Resource: "attendance.shift", Action: "create", Scope: "all", MenuKey: "attendance.shifts"},
			{Resource: "attendance.shift", Action: "update", Scope: "all", MenuKey: "attendance.shifts"},
			{Resource: "attendance.shift_assignment", Action: "read", Scope: "all", MenuKey: "attendance.shift_assignments"},
			{Resource: "attendance.shift_assignment", Action: "create", Scope: "all", MenuKey: "attendance.shift_assignments"},
			{Resource: "attendance.clock", Action: "read", Scope: "all", MenuKey: "attendance.clock"},
			{Resource: "attendance.clock", Action: "create", Scope: "all", MenuKey: "attendance.clock"},
			{Resource: "attendance.correction", Action: "read", Scope: "all", MenuKey: "attendance.corrections"},
			{Resource: "attendance.correction", Action: "create", Scope: "all", MenuKey: "attendance.corrections"},
			{Resource: "attendance.correction", Action: "approve", Scope: "all", MenuKey: "attendance.corrections"},
			{Resource: "attendance.correction", Action: "update", Scope: "all", MenuKey: "attendance.corrections"},
			{Resource: "workflow.form_template", Action: "read", Scope: "all", MenuKey: "workflow.forms"},
			{Resource: "workflow.form_template", Action: "create", Scope: "all", MenuKey: "workflow.forms"},
			{Resource: "workflow.form_template", Action: "update", Scope: "all", MenuKey: "workflow.forms"},
			{Resource: "workflow.form_template", Action: "delete", Scope: "all", MenuKey: "workflow.forms"},
			{Resource: "workflow.form_instance", Action: "read", Scope: "all", MenuKey: "workflow.instances"},
			{Resource: "workflow.form_instance", Action: "create", Scope: "all", MenuKey: "workflow.instances"},
			{Resource: "workflow.form_instance", Action: "submit", Scope: "all", MenuKey: "workflow.instances"},
			{Resource: "workflow.form_instance", Action: "approve", Scope: "all", MenuKey: "workflow.instances"},
			{Resource: "workflow.form_instance", Action: "update", Scope: "all", MenuKey: "workflow.instances"},
			{Resource: "workflow.form_instance", Action: "delete", Scope: "all", MenuKey: "workflow.instances"},
			{Resource: "iam.user_group", Action: "read", Scope: "all", MenuKey: "iam.user_groups"},
			{Resource: "iam.user_group", Action: "create", Scope: "all", MenuKey: "iam.user_groups"},
			{Resource: "iam.permission_set", Action: "read", Scope: "all", MenuKey: "iam.permission_sets"},
			{Resource: "iam.permission_set", Action: "create", Scope: "all", MenuKey: "iam.permission_sets"},
			{Resource: "iam.permission_set_assignment", Action: "read", Scope: "all", MenuKey: "iam.permission_sets"},
			{Resource: "iam.permission_set_assignment", Action: "create", Scope: "all", MenuKey: "iam.permission_sets"},
			{Resource: "iam.assumable_role", Action: "read", Scope: "all", MenuKey: "iam.assumable_roles"},
			{Resource: "iam.assumable_role", Action: "create", Scope: "all", MenuKey: "iam.assumable_roles"},
			{Resource: "iam.assumable_role", Action: "assume", Target: "*", Scope: "all", MenuKey: "iam.assumable_roles"},
			{Resource: "agent.run", Action: "read", Scope: "all", MenuKey: "agents.runs"},
			{Resource: "agent.run", Action: "create", Scope: "all", MenuKey: "agents.runs"},
			{Resource: "agent.tool", Action: "call", Target: "knowledge.search", Scope: "all", MenuKey: "agents.runs"},
			{Resource: "agent.knowledge_article", Action: "read", Scope: "all", MenuKey: "agents.runs"},
			{Resource: "audit.log", Action: "read", Scope: "all", MenuKey: "audit"},
		},
		CreatedAt: now,
	}
	employeeSet := PermissionSet{
		ID:       "ps-employee",
		TenantID: "demo",
		Name:     "Employee Self Service",
		Permissions: []Permission{
			{Resource: "me", Action: "read", Scope: "all", MenuKey: "workbench"},
			{Resource: "me", Action: "create", Scope: "all", MenuKey: "workbench"},
			{Resource: "me", Action: "update", Scope: "all", MenuKey: "workbench"},
			{Resource: "me", Action: "delete", Scope: "all", MenuKey: "workbench"},
			{Resource: "hr.employee", Action: "read", Scope: "self", MenuKey: "hr.employees"},
			{Resource: "attendance.leave", Action: "create", Scope: "self", MenuKey: "attendance.leave"},
			{Resource: "attendance.clock", Action: "read", Scope: "self", MenuKey: "attendance.clock"},
			{Resource: "attendance.clock", Action: "create", Scope: "self", MenuKey: "attendance.clock"},
			{Resource: "attendance.correction", Action: "read", Scope: "self", MenuKey: "attendance.corrections"},
			{Resource: "attendance.correction", Action: "create", Scope: "self", MenuKey: "attendance.corrections"},
			{Resource: "workflow.form_instance", Action: "read", Scope: "self", MenuKey: "workflow.instances"},
			{Resource: "workflow.form_instance", Action: "create", Scope: "self", MenuKey: "workflow.instances"},
			{Resource: "workflow.form_instance", Action: "submit", Scope: "self", MenuKey: "workflow.instances"},
			{Resource: "workflow.form_instance", Action: "update", Scope: "self", MenuKey: "workflow.instances"},
			{Resource: "workflow.form_instance", Action: "delete", Scope: "self", MenuKey: "workflow.instances"},
			{Resource: "agent.run", Action: "read", Scope: "own", MenuKey: "agents.runs"},
			{Resource: "agent.run", Action: "create", Scope: "all", MenuKey: "agents.runs"},
			{Resource: "agent.tool", Action: "call", Target: "knowledge.search", Scope: "all", MenuKey: "agents.runs"},
			{Resource: "agent.knowledge_article", Action: "read", Scope: "all", MenuKey: "agents.runs"},
		},
		CreatedAt: now.Add(time.Minute),
	}
	auditSet := PermissionSet{
		ID:       "ps-audit",
		TenantID: "demo",
		Name:     "Audit Viewer",
		Permissions: []Permission{
			{Resource: "me", Action: "read", Scope: "all", MenuKey: "workbench"},
			{Resource: "audit.log", Action: "read", Scope: "all", MenuKey: "audit"},
			{Resource: "audit.audit_log", Action: "read", Scope: "all", MenuKey: "audit"},
			{Resource: "iam.permission_set", Action: "read", Scope: "all", MenuKey: "iam.permission_sets"},
		},
		CreatedAt: now.Add(2 * time.Minute),
	}
	hrManagerSet := PermissionSet{
		ID:       "ps-hr-manager",
		TenantID: "demo",
		Name:     "HR Manager",
		Permissions: []Permission{
			{Resource: "me", Action: "read", Scope: "all", MenuKey: "workbench"},
			{Resource: "me", Action: "create", Scope: "all", MenuKey: "workbench"},
			{Resource: "me", Action: "update", Scope: "all", MenuKey: "workbench"},
			{Resource: "hr.employee", Action: "read", Scope: "all", MenuKey: "hr.employees"},
			{Resource: "hr.employee", Action: "create", Scope: "all", MenuKey: "hr.employees"},
			{Resource: "hr.employee", Action: "update", Scope: "all", MenuKey: "hr.employees"},
			{Resource: "hr.employee", Action: "invite", Scope: "all", MenuKey: "hr.employees"},
			{Resource: "hr.org_unit", Action: "read", Scope: "all", MenuKey: "hr.org_units"},
			{Resource: "hr.org_unit", Action: "create", Scope: "all", MenuKey: "hr.org_units"},
			{Resource: "attendance.leave", Action: "read", Scope: "all", MenuKey: "attendance.leave"},
			{Resource: "attendance.clock", Action: "read", Scope: "all", MenuKey: "attendance.clock"},
			{Resource: "workflow.form_template", Action: "read", Scope: "all", MenuKey: "workflow.forms"},
			{Resource: "workflow.form_instance", Action: "read", Scope: "all", MenuKey: "workflow.instances"},
			{Resource: "iam.permission_set_assignment", Action: "read", Scope: "all", MenuKey: "iam.permission_sets"},
			{Resource: "audit.log", Action: "read", Scope: "all", MenuKey: "audit"},
			{Resource: "audit.audit_log", Action: "read", Scope: "all", MenuKey: "audit"},
		},
		CreatedAt: now.Add(3 * time.Minute),
	}
	hrReadonlySet := PermissionSet{
		ID:       "ps-hr-readonly",
		TenantID: "demo",
		Name:     "HR Readonly",
		Permissions: []Permission{
			{Resource: "me", Action: "read", Scope: "all", MenuKey: "workbench"},
			{Resource: "hr.employee", Action: "read", Scope: "all", MenuKey: "hr.employees"},
			{Resource: "hr.org_unit", Action: "read", Scope: "all", MenuKey: "hr.org_units"},
			{Resource: "attendance.leave", Action: "read", Scope: "all", MenuKey: "attendance.leave"},
			{Resource: "attendance.clock", Action: "read", Scope: "all", MenuKey: "attendance.clock"},
			{Resource: "workflow.form_template", Action: "read", Scope: "all", MenuKey: "workflow.forms"},
			{Resource: "workflow.form_instance", Action: "read", Scope: "all", MenuKey: "workflow.instances"},
		},
		CreatedAt: now.Add(4 * time.Minute),
	}
	attendanceManagerSet := PermissionSet{
		ID:       "ps-attendance-manager",
		TenantID: "demo",
		Name:     "Attendance Manager",
		Permissions: []Permission{
			{Resource: "me", Action: "read", Scope: "all", MenuKey: "workbench"},
			{Resource: "me", Action: "create", Scope: "all", MenuKey: "workbench"},
			{Resource: "me", Action: "update", Scope: "all", MenuKey: "workbench"},
			{Resource: "hr.employee", Action: "read", Scope: "all", MenuKey: "hr.employees"},
			{Resource: "hr.org_unit", Action: "read", Scope: "all", MenuKey: "hr.org_units"},
			{Resource: "attendance.leave", Action: "read", Scope: "all", MenuKey: "attendance.leave"},
			{Resource: "attendance.leave", Action: "create", Scope: "all", MenuKey: "attendance.leave"},
			{Resource: "attendance.leave", Action: "update", Scope: "all", MenuKey: "attendance.leave"},
			{Resource: "attendance.worksite", Action: "read", Scope: "all", MenuKey: "attendance.worksites"},
			{Resource: "attendance.worksite", Action: "create", Scope: "all", MenuKey: "attendance.worksites"},
			{Resource: "attendance.worksite", Action: "update", Scope: "all", MenuKey: "attendance.worksites"},
			{Resource: "attendance.shift", Action: "read", Scope: "all", MenuKey: "attendance.shifts"},
			{Resource: "attendance.shift", Action: "create", Scope: "all", MenuKey: "attendance.shifts"},
			{Resource: "attendance.shift", Action: "update", Scope: "all", MenuKey: "attendance.shifts"},
			{Resource: "attendance.shift_assignment", Action: "read", Scope: "all", MenuKey: "attendance.shift_assignments"},
			{Resource: "attendance.shift_assignment", Action: "create", Scope: "all", MenuKey: "attendance.shift_assignments"},
			{Resource: "attendance.clock", Action: "read", Scope: "all", MenuKey: "attendance.clock"},
			{Resource: "attendance.clock", Action: "create", Scope: "all", MenuKey: "attendance.clock"},
			{Resource: "attendance.correction", Action: "read", Scope: "all", MenuKey: "attendance.corrections"},
			{Resource: "attendance.correction", Action: "create", Scope: "all", MenuKey: "attendance.corrections"},
			{Resource: "attendance.correction", Action: "approve", Scope: "all", MenuKey: "attendance.corrections"},
			{Resource: "attendance.correction", Action: "update", Scope: "all", MenuKey: "attendance.corrections"},
			{Resource: "workflow.form_instance", Action: "read", Scope: "all", MenuKey: "workflow.instances"},
		},
		CreatedAt: now.Add(5 * time.Minute),
	}
	workflowApproverSet := PermissionSet{
		ID:       "ps-workflow-approver",
		TenantID: "demo",
		Name:     "Workflow Approver",
		Permissions: []Permission{
			{Resource: "me", Action: "read", Scope: "all", MenuKey: "workbench"},
			{Resource: "me", Action: "create", Scope: "all", MenuKey: "workbench"},
			{Resource: "me", Action: "update", Scope: "all", MenuKey: "workbench"},
			{Resource: "hr.employee", Action: "read", Scope: "all", MenuKey: "hr.employees"},
			{Resource: "hr.org_unit", Action: "read", Scope: "all", MenuKey: "hr.org_units"},
			{Resource: "attendance.leave", Action: "read", Scope: "all", MenuKey: "attendance.leave"},
			{Resource: "attendance.clock", Action: "read", Scope: "all", MenuKey: "attendance.clock"},
			{Resource: "workflow.form_template", Action: "read", Scope: "all", MenuKey: "workflow.forms"},
			{Resource: "workflow.form_instance", Action: "read", Scope: "all", MenuKey: "workflow.instances"},
			{Resource: "workflow.form_instance", Action: "approve", Scope: "all", MenuKey: "workflow.instances"},
			{Resource: "workflow.form_instance", Action: "update", Scope: "all", MenuKey: "workflow.instances"},
		},
		CreatedAt: now.Add(6 * time.Minute),
	}
	securityAdminSet := PermissionSet{
		ID:       "ps-security-admin",
		TenantID: "demo",
		Name:     "Security Admin",
		Permissions: []Permission{
			{Resource: "me", Action: "read", Scope: "all", MenuKey: "workbench"},
			{Resource: "hr.employee", Action: "read", Scope: "all", MenuKey: "hr.employees"},
			{Resource: "hr.org_unit", Action: "read", Scope: "all", MenuKey: "hr.org_units"},
			{Resource: "iam.permission", Action: "read", Scope: "all", MenuKey: "iam.permission_sets"},
			{Resource: "iam.user_group", Action: "read", Scope: "all", MenuKey: "iam.user_groups"},
			{Resource: "iam.user_group", Action: "create", Scope: "all", MenuKey: "iam.user_groups"},
			{Resource: "iam.permission_set", Action: "read", Scope: "all", MenuKey: "iam.permission_sets"},
			{Resource: "iam.permission_set", Action: "create", Scope: "all", MenuKey: "iam.permission_sets"},
			{Resource: "iam.permission_set_assignment", Action: "read", Scope: "all", MenuKey: "iam.permission_sets"},
			{Resource: "iam.permission_set_assignment", Action: "create", Scope: "all", MenuKey: "iam.permission_sets"},
			{Resource: "iam.permission_set_assignment", Action: "update", Scope: "all", MenuKey: "iam.permission_sets"},
			{Resource: "iam.data_scope", Action: "read", Scope: "all", MenuKey: "iam.permission_sets"},
			{Resource: "iam.data_scope", Action: "create", Scope: "all", MenuKey: "iam.permission_sets"},
			{Resource: "iam.field_policy", Action: "read", Scope: "all", MenuKey: "iam.permission_sets"},
			{Resource: "iam.field_policy", Action: "create", Scope: "all", MenuKey: "iam.permission_sets"},
			{Resource: "iam.assumable_role", Action: "read", Scope: "all", MenuKey: "iam.assumable_roles"},
			{Resource: "iam.assumable_role", Action: "create", Scope: "all", MenuKey: "iam.assumable_roles"},
			{Resource: "iam.assumable_role", Action: "assume", Target: "*", Scope: "all", MenuKey: "iam.assumable_roles"},
			{Resource: "audit.log", Action: "read", Scope: "all", MenuKey: "audit"},
			{Resource: "audit.audit_log", Action: "read", Scope: "all", MenuKey: "audit"},
		},
		CreatedAt: now.Add(7 * time.Minute),
	}
	insightsViewerSet := PermissionSet{
		ID:       "ps-insights-viewer",
		TenantID: "demo",
		Name:     "Insights Viewer",
		Permissions: []Permission{
			{Resource: "me", Action: "read", Scope: "all", MenuKey: "workbench"},
			{Resource: "hr.employee", Action: "read", Scope: "all", MenuKey: "hr.employees"},
			{Resource: "hr.org_unit", Action: "read", Scope: "all", MenuKey: "hr.org_units"},
			{Resource: "attendance.leave", Action: "read", Scope: "all", MenuKey: "attendance.leave"},
			{Resource: "attendance.clock", Action: "read", Scope: "all", MenuKey: "attendance.clock"},
			{Resource: "workflow.form_instance", Action: "read", Scope: "all", MenuKey: "workflow.instances"},
			{Resource: "agent.run", Action: "read", Scope: "all", MenuKey: "agents.runs"},
			{Resource: "agent.knowledge_article", Action: "read", Scope: "all", MenuKey: "agents.runs"},
		},
		CreatedAt: now.Add(8 * time.Minute),
	}
	_ = store.UpsertPermissionSet(ctx, adminSet)
	_ = store.UpsertPermissionSet(ctx, employeeSet)
	_ = store.UpsertPermissionSet(ctx, auditSet)
	_ = store.UpsertPermissionSet(ctx, hrManagerSet)
	_ = store.UpsertPermissionSet(ctx, hrReadonlySet)
	_ = store.UpsertPermissionSet(ctx, attendanceManagerSet)
	_ = store.UpsertPermissionSet(ctx, workflowApproverSet)
	_ = store.UpsertPermissionSet(ctx, securityAdminSet)
	_ = store.UpsertPermissionSet(ctx, insightsViewerSet)

	_ = store.UpsertUserGroup(ctx, UserGroup{
		ID:               "ug-hr",
		TenantID:         "demo",
		Name:             "HR Team",
		Description:      "人力資源管理組",
		MemberAccountIDs: []string{"acct-admin"},
		PermissionSetIDs: []string{"ps-admin"},
		CreatedAt:        now,
	})
	_ = store.UpsertUserGroup(ctx, UserGroup{
		ID:               "ug-employee",
		TenantID:         "demo",
		Name:             "Employee",
		Description:      "普通员工",
		MemberAccountIDs: []string{"acct-employee"},
		PermissionSetIDs: []string{"ps-employee"},
		CreatedAt:        now.Add(time.Minute),
	})
	_ = store.UpsertUserGroup(ctx, UserGroup{
		ID:               "ug-audit",
		TenantID:         "demo",
		Name:             "Audit",
		Description:      "审计查看组",
		MemberAccountIDs: []string{"acct-audit"},
		PermissionSetIDs: []string{"ps-audit"},
		CreatedAt:        now.Add(2 * time.Minute),
	})
	_ = store.UpsertUserGroup(ctx, UserGroup{
		ID:               "ug-hr-manager",
		TenantID:         "demo",
		Name:             "HR Manager",
		Description:      "HR 管理员测试组",
		MemberAccountIDs: []string{"acct-hr-manager"},
		PermissionSetIDs: []string{"ps-hr-manager"},
		CreatedAt:        now.Add(3 * time.Minute),
	})
	_ = store.UpsertUserGroup(ctx, UserGroup{
		ID:               "ug-hr-readonly",
		TenantID:         "demo",
		Name:             "HR Readonly",
		Description:      "HR 只读测试组",
		MemberAccountIDs: []string{"acct-hr-readonly"},
		PermissionSetIDs: []string{"ps-hr-readonly"},
		CreatedAt:        now.Add(4 * time.Minute),
	})
	_ = store.UpsertUserGroup(ctx, UserGroup{
		ID:               "ug-attendance-manager",
		TenantID:         "demo",
		Name:             "Attendance Manager",
		Description:      "假勤管理员测试组",
		MemberAccountIDs: []string{"acct-attendance-manager"},
		PermissionSetIDs: []string{"ps-attendance-manager"},
		CreatedAt:        now.Add(5 * time.Minute),
	})
	_ = store.UpsertUserGroup(ctx, UserGroup{
		ID:               "ug-workflow-approver",
		TenantID:         "demo",
		Name:             "Workflow Approver",
		Description:      "流程审批测试组",
		MemberAccountIDs: []string{"acct-workflow-approver"},
		PermissionSetIDs: []string{"ps-workflow-approver"},
		CreatedAt:        now.Add(6 * time.Minute),
	})
	_ = store.UpsertUserGroup(ctx, UserGroup{
		ID:               "ug-security-admin",
		TenantID:         "demo",
		Name:             "Security Admin",
		Description:      "安全管理员测试组",
		MemberAccountIDs: []string{"acct-security-admin"},
		PermissionSetIDs: []string{"ps-security-admin"},
		CreatedAt:        now.Add(7 * time.Minute),
	})
	_ = store.UpsertUserGroup(ctx, UserGroup{
		ID:               "ug-insights-viewer",
		TenantID:         "demo",
		Name:             "Insights Viewer",
		Description:      "数据看板只读测试组",
		MemberAccountIDs: []string{"acct-insights-viewer"},
		PermissionSetIDs: []string{"ps-insights-viewer"},
		CreatedAt:        now.Add(8 * time.Minute),
	})

	_ = store.UpsertAccount(ctx, Account{
		ID:                     "acct-admin",
		TenantID:               "demo",
		DisplayName:            "Demo Admin",
		Email:                  "admin@demo.local",
		EmployeeID:             "emp-admin",
		Status:                 "active",
		UserGroupIDs:           []string{"ug-hr"},
		DirectPermissionSetIDs: []string{"ps-admin"},
		CreatedAt:              now,
	})
	_ = store.UpsertAccount(ctx, Account{
		ID:                     "acct-employee",
		TenantID:               "demo",
		DisplayName:            "Demo Employee",
		Email:                  "employee@demo.local",
		EmployeeID:             "emp-employee",
		Status:                 "active",
		UserGroupIDs:           []string{"ug-employee"},
		DirectPermissionSetIDs: []string{"ps-employee"},
		CreatedAt:              now.Add(time.Minute),
	})
	_ = store.UpsertAccount(ctx, Account{
		ID:                     "acct-audit",
		TenantID:               "demo",
		DisplayName:            "Demo Auditor",
		Email:                  "audit@demo.local",
		EmployeeID:             "emp-audit",
		Status:                 "active",
		UserGroupIDs:           []string{"ug-audit"},
		DirectPermissionSetIDs: []string{"ps-audit"},
		CreatedAt:              now.Add(2 * time.Minute),
	})
	_ = store.UpsertAccount(ctx, Account{
		ID:                     "acct-hr-manager",
		TenantID:               "demo",
		DisplayName:            "Demo HR Manager",
		Email:                  "hr.manager@demo.local",
		EmployeeID:             "emp-hr-manager",
		Status:                 "active",
		UserGroupIDs:           []string{"ug-hr-manager"},
		DirectPermissionSetIDs: []string{"ps-hr-manager"},
		CreatedAt:              now.Add(3 * time.Minute),
	})
	_ = store.UpsertAccount(ctx, Account{
		ID:                     "acct-hr-readonly",
		TenantID:               "demo",
		DisplayName:            "Demo HR Readonly",
		Email:                  "hr.readonly@demo.local",
		EmployeeID:             "emp-hr-readonly",
		Status:                 "active",
		UserGroupIDs:           []string{"ug-hr-readonly"},
		DirectPermissionSetIDs: []string{"ps-hr-readonly"},
		CreatedAt:              now.Add(4 * time.Minute),
	})
	_ = store.UpsertAccount(ctx, Account{
		ID:                     "acct-attendance-manager",
		TenantID:               "demo",
		DisplayName:            "Demo Attendance Manager",
		Email:                  "attendance.manager@demo.local",
		EmployeeID:             "emp-attendance-manager",
		Status:                 "active",
		UserGroupIDs:           []string{"ug-attendance-manager"},
		DirectPermissionSetIDs: []string{"ps-attendance-manager"},
		CreatedAt:              now.Add(5 * time.Minute),
	})
	_ = store.UpsertAccount(ctx, Account{
		ID:                     "acct-workflow-approver",
		TenantID:               "demo",
		DisplayName:            "Demo Workflow Approver",
		Email:                  "workflow.approver@demo.local",
		EmployeeID:             "emp-workflow-approver",
		Status:                 "active",
		UserGroupIDs:           []string{"ug-workflow-approver"},
		DirectPermissionSetIDs: []string{"ps-workflow-approver"},
		CreatedAt:              now.Add(6 * time.Minute),
	})
	_ = store.UpsertAccount(ctx, Account{
		ID:                     "acct-security-admin",
		TenantID:               "demo",
		DisplayName:            "Demo Security Admin",
		Email:                  "security.admin@demo.local",
		EmployeeID:             "emp-security-admin",
		Status:                 "active",
		UserGroupIDs:           []string{"ug-security-admin"},
		DirectPermissionSetIDs: []string{"ps-security-admin"},
		CreatedAt:              now.Add(7 * time.Minute),
	})
	_ = store.UpsertAccount(ctx, Account{
		ID:                     "acct-insights-viewer",
		TenantID:               "demo",
		DisplayName:            "Demo Insights Viewer",
		Email:                  "insights.viewer@demo.local",
		EmployeeID:             "emp-insights-viewer",
		Status:                 "active",
		UserGroupIDs:           []string{"ug-insights-viewer"},
		DirectPermissionSetIDs: []string{"ps-insights-viewer"},
		CreatedAt:              now.Add(8 * time.Minute),
	})
	_ = store.UpsertAccount(ctx, Account{
		ID:                     "acct-disabled",
		TenantID:               "demo",
		DisplayName:            "Demo Disabled",
		Email:                  "disabled@demo.local",
		EmployeeID:             "emp-disabled",
		Status:                 "disabled",
		UserGroupIDs:           []string{"ug-employee"},
		DirectPermissionSetIDs: []string{"ps-employee"},
		CreatedAt:              now.Add(9 * time.Minute),
	})
	_ = store.UpsertUserIdentity(ctx, UserIdentity{
		ID:        "uid-keycloak-admin",
		TenantID:  "demo",
		AccountID: "acct-admin",
		Provider:  "keycloak",
		Subject:   "acct-admin",
		Email:     "admin@demo.local",
		CreatedAt: now,
	})
	_ = store.UpsertUserIdentity(ctx, UserIdentity{
		ID:        "uid-keycloak-employee",
		TenantID:  "demo",
		AccountID: "acct-employee",
		Provider:  "keycloak",
		Subject:   "acct-employee",
		Email:     "employee@demo.local",
		CreatedAt: now.Add(time.Minute),
	})
	_ = store.UpsertUserIdentity(ctx, UserIdentity{
		ID:        "uid-keycloak-audit",
		TenantID:  "demo",
		AccountID: "acct-audit",
		Provider:  "keycloak",
		Subject:   "acct-audit",
		Email:     "audit@demo.local",
		CreatedAt: now.Add(2 * time.Minute),
	})
	_ = store.UpsertUserIdentity(ctx, UserIdentity{
		ID:        "uid-keycloak-hr-manager",
		TenantID:  "demo",
		AccountID: "acct-hr-manager",
		Provider:  "keycloak",
		Subject:   "acct-hr-manager",
		Email:     "hr.manager@demo.local",
		CreatedAt: now.Add(3 * time.Minute),
	})
	_ = store.UpsertUserIdentity(ctx, UserIdentity{
		ID:        "uid-keycloak-hr-readonly",
		TenantID:  "demo",
		AccountID: "acct-hr-readonly",
		Provider:  "keycloak",
		Subject:   "acct-hr-readonly",
		Email:     "hr.readonly@demo.local",
		CreatedAt: now.Add(4 * time.Minute),
	})
	_ = store.UpsertUserIdentity(ctx, UserIdentity{
		ID:        "uid-keycloak-attendance-manager",
		TenantID:  "demo",
		AccountID: "acct-attendance-manager",
		Provider:  "keycloak",
		Subject:   "acct-attendance-manager",
		Email:     "attendance.manager@demo.local",
		CreatedAt: now.Add(5 * time.Minute),
	})
	_ = store.UpsertUserIdentity(ctx, UserIdentity{
		ID:        "uid-keycloak-workflow-approver",
		TenantID:  "demo",
		AccountID: "acct-workflow-approver",
		Provider:  "keycloak",
		Subject:   "acct-workflow-approver",
		Email:     "workflow.approver@demo.local",
		CreatedAt: now.Add(6 * time.Minute),
	})
	_ = store.UpsertUserIdentity(ctx, UserIdentity{
		ID:        "uid-keycloak-security-admin",
		TenantID:  "demo",
		AccountID: "acct-security-admin",
		Provider:  "keycloak",
		Subject:   "acct-security-admin",
		Email:     "security.admin@demo.local",
		CreatedAt: now.Add(7 * time.Minute),
	})
	_ = store.UpsertUserIdentity(ctx, UserIdentity{
		ID:        "uid-keycloak-insights-viewer",
		TenantID:  "demo",
		AccountID: "acct-insights-viewer",
		Provider:  "keycloak",
		Subject:   "acct-insights-viewer",
		Email:     "insights.viewer@demo.local",
		CreatedAt: now.Add(8 * time.Minute),
	})
	_ = store.UpsertUserIdentity(ctx, UserIdentity{
		ID:        "uid-keycloak-disabled",
		TenantID:  "demo",
		AccountID: "acct-disabled",
		Provider:  "keycloak",
		Subject:   "acct-disabled",
		Email:     "disabled@demo.local",
		CreatedAt: now.Add(9 * time.Minute),
	})

	_ = store.UpsertOrgUnit(ctx, OrgUnit{
		ID:        "ou-hq",
		TenantID:  "demo",
		Code:      "HQ",
		Name:      "總部",
		Path:      []string{"ou-hq"},
		CreatedAt: now,
	})
	_ = store.UpsertOrgUnit(ctx, OrgUnit{
		ID:        "ou-ops",
		TenantID:  "demo",
		Code:      "OPS",
		Name:      "營運中心",
		ParentID:  "ou-hq",
		Path:      []string{"ou-hq", "ou-ops"},
		CreatedAt: now.Add(time.Minute),
	})
	_ = store.UpsertOrgUnit(ctx, OrgUnit{
		ID:        "ou-hr",
		TenantID:  "demo",
		Code:      "HR",
		Name:      "人力資源部",
		ParentID:  "ou-hq",
		Path:      []string{"ou-hq", "ou-hr"},
		CreatedAt: now.Add(2 * time.Minute),
	})
	_ = store.UpsertOrgUnit(ctx, OrgUnit{
		ID:        "ou-finance",
		TenantID:  "demo",
		Code:      "FIN",
		Name:      "財務中心",
		ParentID:  "ou-hq",
		Path:      []string{"ou-hq", "ou-finance"},
		CreatedAt: now.Add(3 * time.Minute),
	})
	_ = store.UpsertOrgUnit(ctx, OrgUnit{
		ID:        "ou-sales",
		TenantID:  "demo",
		Code:      "SALES",
		Name:      "銷售中心",
		ParentID:  "ou-hq",
		Path:      []string{"ou-hq", "ou-sales"},
		CreatedAt: now.Add(4 * time.Minute),
	})
	_ = store.UpsertOrgUnit(ctx, OrgUnit{
		ID:        "ou-security",
		TenantID:  "demo",
		Code:      "SEC",
		Name:      "安全合规部",
		ParentID:  "ou-hq",
		Path:      []string{"ou-hq", "ou-security"},
		CreatedAt: now.Add(5 * time.Minute),
	})

	hire := now.Add(-24 * time.Hour)
	probationHire := now.Add(-14 * 24 * time.Hour)
	recentHire := now.Add(-7 * 24 * time.Hour)
	resignDate := now.Add(-3 * 24 * time.Hour)
	_ = store.UpsertEmployee(ctx, Employee{
		ID:               "emp-admin",
		TenantID:         "demo",
		EmployeeNo:       "E0001",
		Name:             "Demo Admin",
		CompanyEmail:     "admin@demo.local",
		Phone:            "0911000001",
		OrgUnitID:        "ou-hq",
		AccountID:        "acct-admin",
		Position:         "Platform Owner",
		Category:         "full_time",
		Status:           "active",
		EmploymentStatus: "active",
		HireDate:         &hire,
		BasicInfo:        map[string]any{"name": "Demo Admin", "company_email": "admin@demo.local", "nationality_type": "local", "national_id": "A123456789"},
		EmploymentInfo:   map[string]any{"org_unit_id": "ou-hq", "position": "Platform Owner", "category": "full_time"},
		ContactInfo:      map[string]any{"mobile_phone": "0911000001"},
		InsuranceInfo:    map[string]any{"labor_insurance_salary": "45800"},
		CreatedAt:        now,
		UpdatedAt:        now,
	})
	_ = store.UpsertEmployee(ctx, Employee{
		ID:               "emp-employee",
		TenantID:         "demo",
		EmployeeNo:       "E0002",
		Name:             "Demo Employee",
		CompanyEmail:     "employee@demo.local",
		Phone:            "0911000002",
		OrgUnitID:        "ou-ops",
		AccountID:        "acct-employee",
		Position:         "HR Specialist",
		Category:         "full_time",
		Status:           "active",
		EmploymentStatus: "active",
		HireDate:         &hire,
		BasicInfo:        map[string]any{"name": "Demo Employee", "company_email": "employee@demo.local", "nationality_type": "local", "national_id": "B123456789"},
		EmploymentInfo:   map[string]any{"org_unit_id": "ou-ops", "position": "HR Specialist", "category": "full_time"},
		ContactInfo:      map[string]any{"mobile_phone": "0911000002"},
		InsuranceInfo:    map[string]any{"labor_insurance_salary": "42000"},
		CreatedAt:        now.Add(time.Minute),
		UpdatedAt:        now.Add(time.Minute),
	})
	_ = store.UpsertEmployee(ctx, Employee{
		ID:               "emp-audit",
		TenantID:         "demo",
		EmployeeNo:       "E0003",
		Name:             "Demo Auditor",
		CompanyEmail:     "audit@demo.local",
		Phone:            "0911000003",
		OrgUnitID:        "ou-hq",
		AccountID:        "acct-audit",
		Position:         "Internal Auditor",
		Category:         "full_time",
		Status:           "active",
		EmploymentStatus: "active",
		HireDate:         &hire,
		BasicInfo:        map[string]any{"name": "Demo Auditor", "company_email": "audit@demo.local", "nationality_type": "local", "national_id": "C123456789"},
		EmploymentInfo:   map[string]any{"org_unit_id": "ou-hq", "position": "Internal Auditor", "category": "full_time"},
		ContactInfo:      map[string]any{"mobile_phone": "0911000003"},
		InsuranceInfo:    map[string]any{"labor_insurance_salary": "50000"},
		CreatedAt:        now.Add(2 * time.Minute),
		UpdatedAt:        now.Add(2 * time.Minute),
	})
	_ = store.UpsertEmployee(ctx, Employee{
		ID:                "emp-hr-manager",
		TenantID:          "demo",
		EmployeeNo:        "E0004",
		Name:              "Demo HR Manager",
		CompanyEmail:      "hr.manager@demo.local",
		Phone:             "0911000004",
		OrgUnitID:         "ou-hr",
		AccountID:         "acct-hr-manager",
		ManagerEmployeeID: "emp-admin",
		Position:          "HR Manager",
		Category:          "full_time",
		Status:            "active",
		EmploymentStatus:  "active",
		HireDate:          &hire,
		BasicInfo:         map[string]any{"name": "Demo HR Manager", "company_email": "hr.manager@demo.local", "nationality_type": "local", "national_id": "D123456789"},
		EmploymentInfo:    map[string]any{"org_unit_id": "ou-hr", "position": "HR Manager", "category": "full_time", "manager_employee_id": "emp-admin"},
		ContactInfo:       map[string]any{"mobile_phone": "0911000004"},
		InsuranceInfo:     map[string]any{"labor_insurance_salary": "62000"},
		CreatedAt:         now.Add(3 * time.Minute),
		UpdatedAt:         now.Add(3 * time.Minute),
	})
	_ = store.UpsertEmployee(ctx, Employee{
		ID:                "emp-hr-readonly",
		TenantID:          "demo",
		EmployeeNo:        "E0005",
		Name:              "Demo HR Readonly",
		CompanyEmail:      "hr.readonly@demo.local",
		Phone:             "0911000005",
		OrgUnitID:         "ou-hr",
		AccountID:         "acct-hr-readonly",
		ManagerEmployeeID: "emp-hr-manager",
		Position:          "HR Analyst",
		Category:          "contractor",
		Status:            "probation",
		EmploymentStatus:  "probation",
		HireDate:          &probationHire,
		BasicInfo:         map[string]any{"name": "Demo HR Readonly", "company_email": "hr.readonly@demo.local", "nationality_type": "local", "national_id": "E123456789"},
		EmploymentInfo:    map[string]any{"org_unit_id": "ou-hr", "position": "HR Analyst", "category": "contractor", "manager_employee_id": "emp-hr-manager"},
		ContactInfo:       map[string]any{"mobile_phone": "0911000005"},
		InsuranceInfo:     map[string]any{"labor_insurance_salary": "46000"},
		CreatedAt:         now.Add(4 * time.Minute),
		UpdatedAt:         now.Add(4 * time.Minute),
	})
	_ = store.UpsertEmployee(ctx, Employee{
		ID:                "emp-attendance-manager",
		TenantID:          "demo",
		EmployeeNo:        "E0006",
		Name:              "Demo Attendance Manager",
		CompanyEmail:      "attendance.manager@demo.local",
		Phone:             "0911000006",
		OrgUnitID:         "ou-ops",
		AccountID:         "acct-attendance-manager",
		ManagerEmployeeID: "emp-admin",
		Position:          "Attendance Manager",
		Category:          "full_time",
		Status:            "active",
		EmploymentStatus:  "active",
		HireDate:          &hire,
		BasicInfo:         map[string]any{"name": "Demo Attendance Manager", "company_email": "attendance.manager@demo.local", "nationality_type": "local", "national_id": "F123456789"},
		EmploymentInfo:    map[string]any{"org_unit_id": "ou-ops", "position": "Attendance Manager", "category": "full_time", "manager_employee_id": "emp-admin"},
		ContactInfo:       map[string]any{"mobile_phone": "0911000006"},
		InsuranceInfo:     map[string]any{"labor_insurance_salary": "56000"},
		CreatedAt:         now.Add(5 * time.Minute),
		UpdatedAt:         now.Add(5 * time.Minute),
	})
	_ = store.UpsertEmployee(ctx, Employee{
		ID:                "emp-workflow-approver",
		TenantID:          "demo",
		EmployeeNo:        "E0007",
		Name:              "Demo Workflow Approver",
		CompanyEmail:      "workflow.approver@demo.local",
		Phone:             "0911000007",
		OrgUnitID:         "ou-finance",
		AccountID:         "acct-workflow-approver",
		ManagerEmployeeID: "emp-admin",
		Position:          "Finance Approver",
		Category:          "full_time",
		Status:            "active",
		EmploymentStatus:  "active",
		HireDate:          &hire,
		BasicInfo:         map[string]any{"name": "Demo Workflow Approver", "company_email": "workflow.approver@demo.local", "nationality_type": "local", "national_id": "G123456789"},
		EmploymentInfo:    map[string]any{"org_unit_id": "ou-finance", "position": "Finance Approver", "category": "full_time", "manager_employee_id": "emp-admin"},
		ContactInfo:       map[string]any{"mobile_phone": "0911000007"},
		InsuranceInfo:     map[string]any{"labor_insurance_salary": "58000"},
		CreatedAt:         now.Add(6 * time.Minute),
		UpdatedAt:         now.Add(6 * time.Minute),
	})
	_ = store.UpsertEmployee(ctx, Employee{
		ID:                "emp-security-admin",
		TenantID:          "demo",
		EmployeeNo:        "E0008",
		Name:              "Demo Security Admin",
		CompanyEmail:      "security.admin@demo.local",
		Phone:             "0911000008",
		OrgUnitID:         "ou-security",
		AccountID:         "acct-security-admin",
		ManagerEmployeeID: "emp-admin",
		Position:          "Security Admin",
		Category:          "full_time",
		Status:            "active",
		EmploymentStatus:  "active",
		HireDate:          &hire,
		BasicInfo:         map[string]any{"name": "Demo Security Admin", "company_email": "security.admin@demo.local", "nationality_type": "local", "national_id": "H123456789"},
		EmploymentInfo:    map[string]any{"org_unit_id": "ou-security", "position": "Security Admin", "category": "full_time", "manager_employee_id": "emp-admin"},
		ContactInfo:       map[string]any{"mobile_phone": "0911000008"},
		InsuranceInfo:     map[string]any{"labor_insurance_salary": "60000"},
		CreatedAt:         now.Add(7 * time.Minute),
		UpdatedAt:         now.Add(7 * time.Minute),
	})
	_ = store.UpsertEmployee(ctx, Employee{
		ID:                "emp-insights-viewer",
		TenantID:          "demo",
		EmployeeNo:        "E0009",
		Name:              "Demo Insights Viewer",
		CompanyEmail:      "insights.viewer@demo.local",
		Phone:             "0911000009",
		OrgUnitID:         "ou-sales",
		AccountID:         "acct-insights-viewer",
		ManagerEmployeeID: "emp-admin",
		Position:          "Business Analyst",
		Category:          "part_time",
		Status:            "onboarding",
		EmploymentStatus:  "onboarding",
		HireDate:          &recentHire,
		BasicInfo:         map[string]any{"name": "Demo Insights Viewer", "company_email": "insights.viewer@demo.local", "nationality_type": "local", "national_id": "I123456789"},
		EmploymentInfo:    map[string]any{"org_unit_id": "ou-sales", "position": "Business Analyst", "category": "part_time", "manager_employee_id": "emp-admin"},
		ContactInfo:       map[string]any{"mobile_phone": "0911000009"},
		InsuranceInfo:     map[string]any{"labor_insurance_salary": "36000"},
		CreatedAt:         now.Add(8 * time.Minute),
		UpdatedAt:         now.Add(8 * time.Minute),
	})
	_ = store.UpsertEmployee(ctx, Employee{
		ID:                "emp-disabled",
		TenantID:          "demo",
		EmployeeNo:        "E0010",
		Name:              "Demo Disabled",
		CompanyEmail:      "disabled@demo.local",
		Phone:             "0911000010",
		OrgUnitID:         "ou-ops",
		AccountID:         "acct-disabled",
		ManagerEmployeeID: "emp-attendance-manager",
		Position:          "Former Specialist",
		Category:          "full_time",
		Status:            "resigned",
		EmploymentStatus:  "resigned",
		HireDate:          &hire,
		ResignDate:        &resignDate,
		BasicInfo:         map[string]any{"name": "Demo Disabled", "company_email": "disabled@demo.local", "nationality_type": "local", "national_id": "J123456789"},
		EmploymentInfo:    map[string]any{"org_unit_id": "ou-ops", "position": "Former Specialist", "category": "full_time", "manager_employee_id": "emp-attendance-manager", "resign_reason": "demo_disabled_account"},
		ContactInfo:       map[string]any{"mobile_phone": "0911000010"},
		InsuranceInfo:     map[string]any{"labor_insurance_salary": "42000"},
		CreatedAt:         now.Add(9 * time.Minute),
		UpdatedAt:         now.Add(9 * time.Minute),
	})

	_ = store.UpsertLeaveBalance(ctx, LeaveBalance{
		ID:             "lb-1",
		TenantID:       "demo",
		EmployeeID:     "emp-employee",
		LeaveType:      "annual",
		RemainingHours: 96,
		UpdatedAt:      now,
	})
	_ = store.UpsertLeaveBalance(ctx, LeaveBalance{
		ID:             "lb-2",
		TenantID:       "demo",
		EmployeeID:     "emp-employee",
		LeaveType:      "sick",
		RemainingHours: 40,
		UpdatedAt:      now,
	})
	for _, balance := range []LeaveBalance{
		{ID: "lb-hr-manager-annual", TenantID: "demo", EmployeeID: "emp-hr-manager", LeaveType: "annual", RemainingHours: 120, UpdatedAt: now.Add(3 * time.Minute)},
		{ID: "lb-hr-readonly-annual", TenantID: "demo", EmployeeID: "emp-hr-readonly", LeaveType: "annual", RemainingHours: 64, UpdatedAt: now.Add(4 * time.Minute)},
		{ID: "lb-attendance-manager-annual", TenantID: "demo", EmployeeID: "emp-attendance-manager", LeaveType: "annual", RemainingHours: 88, UpdatedAt: now.Add(5 * time.Minute)},
		{ID: "lb-workflow-approver-annual", TenantID: "demo", EmployeeID: "emp-workflow-approver", LeaveType: "annual", RemainingHours: 72, UpdatedAt: now.Add(6 * time.Minute)},
		{ID: "lb-security-admin-annual", TenantID: "demo", EmployeeID: "emp-security-admin", LeaveType: "annual", RemainingHours: 80, UpdatedAt: now.Add(7 * time.Minute)},
		{ID: "lb-insights-viewer-annual", TenantID: "demo", EmployeeID: "emp-insights-viewer", LeaveType: "annual", RemainingHours: 40, UpdatedAt: now.Add(8 * time.Minute)},
	} {
		_ = store.UpsertLeaveBalance(ctx, balance)
	}

	_ = store.UpsertAttendanceWorksite(ctx, AttendanceWorksite{
		ID:           "aws-demo-hq",
		TenantID:     "demo",
		Name:         "Demo HQ",
		Address:      "Demo headquarters",
		Latitude:     25.033964,
		Longitude:    121.564468,
		RadiusMeters: 300,
		Status:       "active",
		CreatedAt:    now,
		UpdatedAt:    now,
	})
	_ = store.UpsertAttendanceShift(ctx, AttendanceShift{
		ID:                     "ash-day",
		TenantID:               "demo",
		Name:                   "Day Shift",
		ClockInStart:           "08:00",
		ClockInEnd:             "10:00",
		ClockOutStart:          "17:00",
		ClockOutEnd:            "19:00",
		LateGraceMinutes:       10,
		EarlyLeaveGraceMinutes: 10,
		Status:                 "active",
		CreatedAt:              now,
		UpdatedAt:              now,
	})
	for _, employeeID := range []string{
		"emp-admin",
		"emp-employee",
		"emp-audit",
		"emp-hr-manager",
		"emp-hr-readonly",
		"emp-attendance-manager",
		"emp-workflow-approver",
		"emp-security-admin",
		"emp-insights-viewer",
		"emp-zxy1",
	} {
		_ = store.UpsertAttendanceShiftAssignment(ctx, AttendanceShiftAssignment{
			ID:            "asa-" + employeeID,
			TenantID:      "demo",
			EmployeeID:    employeeID,
			ShiftID:       "ash-day",
			WorksiteID:    "aws-demo-hq",
			EffectiveFrom: now.Add(-30 * 24 * time.Hour),
			Status:        "active",
			CreatedAt:     now,
			UpdatedAt:     now,
		})
	}
	dashboardDate := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	_ = store.UpsertLeaveRequest(ctx, LeaveRequest{
		ID:         "lr-demo-hr-readonly-20260701",
		TenantID:   "demo",
		EmployeeID: "emp-hr-readonly",
		LeaveType:  "annual",
		StartAt:    dashboardDate.Add(9 * time.Hour),
		EndAt:      dashboardDate.Add(18 * time.Hour),
		Hours:      8,
		Reason:     fixtureDashboardLeaveReason,
		Status:     "approved",
		CreatedAt:  now.Add(10 * time.Minute),
	})
	for _, record := range []AttendanceClockRecord{
		{
			ID:                "acr-emp-admin-20260701-in",
			TenantID:          "demo",
			EmployeeID:        "emp-admin",
			ShiftAssignmentID: "asa-emp-admin",
			ShiftID:           "ash-day",
			WorksiteID:        "aws-demo-hq",
			WorkDate:          "2026-07-01",
			Direction:         fixtureClockDirectionIn,
			ClockedAt:         dashboardDate.Add(8*time.Hour + 55*time.Minute),
			Latitude:          25.033964,
			Longitude:         121.564468,
			AccuracyMeters:    12,
			DistanceMeters:    0,
			RecordStatus:      fixtureClockRecordAccepted,
			Source:            fixtureClockSourceGeofence,
			DeviceID:          fixtureClockDeviceDashboard,
			CreatedAt:         now.Add(10 * time.Minute),
		},
		{
			ID:                "acr-emp-admin-20260701-out",
			TenantID:          "demo",
			EmployeeID:        "emp-admin",
			ShiftAssignmentID: "asa-emp-admin",
			ShiftID:           "ash-day",
			WorksiteID:        "aws-demo-hq",
			WorkDate:          "2026-07-01",
			Direction:         fixtureClockDirectionOut,
			ClockedAt:         dashboardDate.Add(18 * time.Hour),
			Latitude:          25.033964,
			Longitude:         121.564468,
			AccuracyMeters:    18,
			DistanceMeters:    0,
			RecordStatus:      fixtureClockRecordAccepted,
			Source:            fixtureClockSourceGeofence,
			DeviceID:          fixtureClockDeviceDashboard,
			CreatedAt:         now.Add(10*time.Minute + 30*time.Second),
		},
		{
			ID:                "acr-emp-hr-manager-20260701-in",
			TenantID:          "demo",
			EmployeeID:        "emp-hr-manager",
			ShiftAssignmentID: "asa-emp-hr-manager",
			ShiftID:           "ash-day",
			WorksiteID:        "aws-demo-hq",
			WorkDate:          "2026-07-01",
			Direction:         fixtureClockDirectionIn,
			ClockedAt:         dashboardDate.Add(9*time.Hour + 5*time.Minute),
			Latitude:          25.033964,
			Longitude:         121.564468,
			AccuracyMeters:    20,
			DistanceMeters:    0,
			RecordStatus:      fixtureClockRecordAccepted,
			Source:            fixtureClockSourceGeofence,
			DeviceID:          fixtureClockDeviceDashboard,
			CreatedAt:         now.Add(11 * time.Minute),
		},
		{
			ID:                "acr-emp-attendance-manager-20260701-in",
			TenantID:          "demo",
			EmployeeID:        "emp-attendance-manager",
			ShiftAssignmentID: "asa-emp-attendance-manager",
			ShiftID:           "ash-day",
			WorksiteID:        "aws-demo-hq",
			WorkDate:          "2026-07-01",
			Direction:         fixtureClockDirectionIn,
			ClockedAt:         dashboardDate.Add(8*time.Hour + 48*time.Minute),
			Latitude:          25.033964,
			Longitude:         121.564468,
			AccuracyMeters:    9,
			DistanceMeters:    0,
			RecordStatus:      fixtureClockRecordAccepted,
			Source:            fixtureClockSourceGeofence,
			DeviceID:          fixtureClockDeviceDashboard,
			CreatedAt:         now.Add(12 * time.Minute),
		},
	} {
		_ = store.UpsertAttendanceClockRecord(ctx, record)
	}

	_ = store.UpsertFormTemplate(ctx, FormTemplate{
		ID:          "ft-leave",
		TenantID:    "demo",
		Key:         "leave-request",
		Name:        "請假申請單",
		Description: "請假與審批流程模板",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"leave_type": map[string]any{"type": "string"},
				"start_at":   map[string]any{"type": "string"},
				"end_at":     map[string]any{"type": "string"},
				"hours":      map[string]any{"type": "number"},
			},
			"workspace_design": map[string]any{
				"enabled": true,
				"stages": []map[string]any{{
					"id":     "stage-admin",
					"type":   "approver",
					"label":  "審核",
					"detail": "由管理員審核",
					"config": map[string]any{"account_ids": []any{"acct-zxy1", "acct-admin"}},
				}},
			},
		},
		CreatedAt: now,
	})
	populateFixturePlatformFormTemplates(ctx, store, now)
	_ = store.UpsertFormTemplate(ctx, FormTemplate{
		ID:          "ft-proof",
		TenantID:    "demo",
		Key:         "employment-certificate",
		Name:        "在職證明",
		Description: "在職證明模板",
		Schema:      map[string]any{"type": "object"},
		CreatedAt:   now.Add(time.Minute),
	})

	_ = store.UpsertKnowledgeArticle(ctx, KnowledgeArticle{
		ID:        "kb-leave-policy",
		TenantID:  "demo",
		Title:     "請假政策",
		Content:   "員工請假需提前發起申請，餘額不足時需要直屬主管和 HR 確認。年假優先扣減年假餘額，支援按小時申請。",
		Tags:      []string{"請假", "假勤", "policy"},
		CreatedAt: now,
	})
	_ = store.UpsertKnowledgeArticle(ctx, KnowledgeArticle{
		ID:        "kb-hr-handbook",
		TenantID:  "demo",
		Title:     "員工手冊",
		Content:   "員工手冊說明組織、考勤、審批與審計規則，所有高危操作需要二次確認。",
		Tags:      []string{"手冊", "審批", "audit"},
		CreatedAt: now.Add(time.Minute),
	})

	// 額外 tenant 用於驗證 tenant 隔離。
	_ = store.UpsertPermissionSet(ctx, PermissionSet{
		ID:       "ps-alpha-basic",
		TenantID: "alpha",
		Name:     "Alpha Basic",
		Permissions: []Permission{
			{Resource: "me", Action: "read", Scope: "all", MenuKey: "workbench"},
		},
		CreatedAt: now,
	})
	_ = store.UpsertAccount(ctx, Account{
		ID:                     "acct-alpha-admin",
		TenantID:               "alpha",
		DisplayName:            "Alpha Admin",
		Email:                  "admin@alpha.local",
		EmployeeID:             "emp-alpha-admin",
		Status:                 "active",
		DirectPermissionSetIDs: []string{"ps-alpha-basic"},
		CreatedAt:              now,
	})
	_ = store.UpsertEmployee(ctx, Employee{
		ID:        "emp-alpha-admin",
		TenantID:  "alpha",
		Name:      "Alpha Admin",
		Status:    "active",
		CreatedAt: now,
	})
}

// populateFixturePlatformFormTemplates 驗證 populate fixture 平台表單範本。
func populateFixturePlatformFormTemplates(ctx context.Context, store repository.Store, now time.Time) {
	offset := 2
	for _, column := range platformFormColumns() {
		for _, item := range column.Items {
			if item.ID == "leave-request" {
				continue
			}
			_ = store.UpsertFormTemplate(ctx, FormTemplate{
				ID:          "ft-" + item.ID,
				TenantID:    "demo",
				Key:         item.ID,
				Name:        item.Title,
				Description: item.Desc,
				Schema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"subject":     map[string]any{"type": "string"},
						"description": map[string]any{"type": "string"},
						"needed_at":   map[string]any{"type": "string"},
						"desc":        map[string]any{"type": "string"},
					},
				},
				CreatedAt: now.Add(time.Duration(offset) * time.Minute),
			})
			offset++
		}
	}
}

// platformFormColumns 驗證平台表單 columns。
func platformFormColumns() []PlatformFormColumn {
	return []PlatformFormColumn{
		{Title: "人事考勤類", Emoji: "👥", Items: []PlatformFormItem{
			{ID: "leave-request", Emoji: "🗓️", Title: "請假申請單", Desc: "特休 / 事假 / 病假 / 公假"},
			{ID: "overtime-approval", Emoji: "⏰", Title: "加班核准申請單", Desc: "平日延時、假日加班皆可使用"},
			{ID: "punch-fix", Emoji: "🕒", Title: "HR-005 補卡單", Desc: "漏打卡或打卡異常補登"},
		}},
		{Title: "人資相關", Emoji: "👥", Items: []PlatformFormItem{
			{ID: "job-change", Emoji: "📋", Title: "人事/職務/薪資異動單", Desc: "異動職務、調薪、調動"},
			{ID: "headcount-request", Emoji: "➕", Title: "iKala 人員增補申請單", Desc: "新增職缺與招募"},
			{ID: "resignation", Emoji: "👋", Title: "離職及退休申請單", Desc: "離職、退休手續辦理"},
		}},
		{Title: "財會相關", Emoji: "💰", Items: []PlatformFormItem{
			{ID: "expense-claim", Emoji: "💸", Title: "費用報支申請單", Desc: "日常費用核銷"},
			{ID: "prepayment", Emoji: "💵", Title: "預支款申請單", Desc: "出差或專案預支款"},
			{ID: "advance-reimburse", Emoji: "💳", Title: "員工代墊款請領清單", Desc: "員工代墊費用請領"},
		}},
		{Title: "行政相關", Emoji: "🧾", Items: []PlatformFormItem{
			{ID: "travel-request", Emoji: "🛫", Title: "國內外出差申請表", Desc: "出差行程預先申請"},
			{ID: "business-card", Emoji: "📇", Title: "名片申請單", Desc: "新印或補印名片"},
			{ID: "memo", Emoji: "📝", Title: "簽呈", Desc: "通用簽呈"},
		}},
	}
}
