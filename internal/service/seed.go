package service

import (
	"context"
	"time"

	"nexus-pro-be/internal/repository"
)

// SeedDemo inserts deterministic demo data for local development and tests.
func SeedDemo(store repository.Store) {
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
			{Resource: "workflow.form_template", Action: "read", Scope: "all", MenuKey: "workflow.forms"},
			{Resource: "workflow.form_template", Action: "create", Scope: "all", MenuKey: "workflow.forms"},
			{Resource: "workflow.form_instance", Action: "submit", Scope: "all", MenuKey: "workflow.instances"},
			{Resource: "workflow.form_instance", Action: "approve", Scope: "all", MenuKey: "workflow.instances"},
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
			{Resource: "hr.employee", Action: "read", Scope: "self", MenuKey: "hr.employees"},
			{Resource: "attendance.leave", Action: "create", Scope: "self", MenuKey: "attendance.leave"},
			{Resource: "workflow.form_instance", Action: "submit", Scope: "self", MenuKey: "workflow.instances"},
			{Resource: "agent.run", Action: "read", Scope: "all", MenuKey: "agents.runs"},
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
			{Resource: "iam.permission_set", Action: "read", Scope: "all", MenuKey: "iam.permission_sets"},
		},
		CreatedAt: now.Add(2 * time.Minute),
	}
	_ = store.UpsertPermissionSet(ctx, adminSet)
	_ = store.UpsertPermissionSet(ctx, employeeSet)
	_ = store.UpsertPermissionSet(ctx, auditSet)

	_ = store.UpsertUserGroup(ctx, UserGroup{
		ID:               "ug-hr",
		TenantID:         "demo",
		Name:             "HR Team",
		Description:      "人力资源管理组",
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

	_ = store.UpsertOrgUnit(ctx, OrgUnit{
		ID:        "ou-hq",
		TenantID:  "demo",
		Code:      "HQ",
		Name:      "总部",
		Path:      []string{"ou-hq"},
		CreatedAt: now,
	})
	_ = store.UpsertOrgUnit(ctx, OrgUnit{
		ID:        "ou-ops",
		TenantID:  "demo",
		Code:      "OPS",
		Name:      "运营中心",
		ParentID:  "ou-hq",
		Path:      []string{"ou-hq", "ou-ops"},
		CreatedAt: now.Add(time.Minute),
	})

	hire := now.Add(-24 * time.Hour)
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

	_ = store.UpsertFormTemplate(ctx, FormTemplate{
		ID:          "ft-leave",
		TenantID:    "demo",
		Key:         "leave-request",
		Name:        "请假申请",
		Description: "请假与审批流程模板",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"leave_type": map[string]any{"type": "string"},
				"start_at":   map[string]any{"type": "string"},
				"end_at":     map[string]any{"type": "string"},
				"hours":      map[string]any{"type": "number"},
			},
		},
		CreatedAt: now,
	})
	_ = store.UpsertFormTemplate(ctx, FormTemplate{
		ID:          "ft-proof",
		TenantID:    "demo",
		Key:         "employment-certificate",
		Name:        "在职证明",
		Description: "在职证明模板",
		Schema:      map[string]any{"type": "object"},
		CreatedAt:   now.Add(time.Minute),
	})

	_ = store.UpsertKnowledgeArticle(ctx, KnowledgeArticle{
		ID:        "kb-leave-policy",
		TenantID:  "demo",
		Title:     "请假政策",
		Content:   "员工请假需提前发起申请，余额不足时需要直属主管和HR确认。年假优先扣减年假余额，支持按小时申请。",
		Tags:      []string{"请假", "假勤", "policy"},
		CreatedAt: now,
	})
	_ = store.UpsertKnowledgeArticle(ctx, KnowledgeArticle{
		ID:        "kb-hr-handbook",
		TenantID:  "demo",
		Title:     "员工手册",
		Content:   "员工手册说明组织、考勤、审批与审计规则，所有高危操作需要二次确认。",
		Tags:      []string{"手册", "审批", "audit"},
		CreatedAt: now.Add(time.Minute),
	})

	// An additional tenant to validate tenant isolation.
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
