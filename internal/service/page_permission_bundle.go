package service

import (
	"strings"

	"nexus-pro-be/internal/domain"
)

type pagePermissionResource struct {
	applicationCode ApplicationCode
	resourceType    ResourceType
	actions         []Action
}

var pagePermissionBundles = map[string][]pagePermissionResource{
	"workspace.overview": {
		{applicationCode: AppHR, resourceType: ResourceEmployee, actions: []Action{ActionRead}},
	},
	"hr.employees": {
		{applicationCode: AppHR, resourceType: ResourceEmployee, actions: []Action{ActionRead, ActionCreate, ActionUpdate, ActionDelete, ActionExport, ActionImport, ActionInvite, ActionUpdateStatus, ActionStatusTransition}},
		{applicationCode: AppHR, resourceType: ResourceOrgUnit, actions: []Action{ActionRead}},
		{applicationCode: AppHR, resourceType: ResourcePosition, actions: []Action{ActionRead}},
	},
	"hr.org_units": {
		{applicationCode: AppHR, resourceType: ResourceOrgUnit, actions: []Action{ActionRead, ActionCreate, ActionUpdate}},
		{applicationCode: AppHR, resourceType: ResourcePosition, actions: []Action{ActionRead}},
	},
	"hr.positions": {
		{applicationCode: AppHR, resourceType: ResourcePosition, actions: []Action{ActionRead, ActionCreate, ActionUpdate, ActionDelete}},
		{applicationCode: AppHR, resourceType: ResourceOrgUnit, actions: []Action{ActionRead}},
	},
	"hr.organization": {
		{applicationCode: AppHR, resourceType: ResourceEmployee, actions: []Action{ActionRead, ActionUpdate}},
		{applicationCode: AppHR, resourceType: ResourceOrgUnit, actions: []Action{ActionRead}},
		{applicationCode: AppHR, resourceType: ResourcePosition, actions: []Action{ActionRead}},
	},
	"hr.turnover": {
		{applicationCode: AppHR, resourceType: ResourceEmployee, actions: []Action{ActionRead, ActionExport}},
	},
	"attendance.overview": {
		{applicationCode: AppAttendance, resourceType: ResourceAttendanceClock, actions: []Action{ActionRead, ActionExport, ActionImport}},
	},
	"attendance.clock": {
		{applicationCode: AppAttendance, resourceType: ResourceAttendanceClock, actions: []Action{ActionRead, ActionExport}},
		{applicationCode: AppAttendance, resourceType: ResourceAttendanceCorrection, actions: []Action{ActionCreate}},
	},
	"attendance.leave_policy": {
		{applicationCode: AppAttendance, resourceType: ResourceLeave, actions: []Action{ActionRead, ActionUpdate}},
	},
	"workflow.forms": {
		{applicationCode: AppWorkflow, resourceType: ResourceType("form_template"), actions: []Action{ActionRead, ActionCreate, ActionUpdate, ActionDelete}},
	},
	"agents.models": {
		{applicationCode: AppAgent, resourceType: ResourceModel, actions: []Action{ActionRead, ActionCreate, ActionUpdate, ActionDelete}},
	},
	"agents.definitions": {
		{applicationCode: AppAgent, resourceType: ResourceDefinition, actions: []Action{ActionRead, ActionCreate, ActionUpdate, ActionDelete}},
		{applicationCode: AppAgent, resourceType: ResourceModel, actions: []Action{ActionRead}},
		{applicationCode: AppAgent, resourceType: ResourceTool, actions: []Action{ActionRead}},
		{applicationCode: AppAgent, resourceType: ResourceType("knowledge_base"), actions: []Action{ActionRead}},
		{applicationCode: AppHR, resourceType: ResourceOrgUnit, actions: []Action{ActionRead}},
		{applicationCode: AppIAM, resourceType: ResourceAssumableRole, actions: []Action{ActionRead}},
	},
	"agents.knowledge_bases": {
		{applicationCode: AppAgent, resourceType: ResourceType("knowledge_base"), actions: []Action{ActionRead, ActionCreate, ActionUpdate, ActionDelete}},
	},
	"agents.tools": {
		{applicationCode: AppAgent, resourceType: ResourceTool, actions: []Action{ActionRead}},
	},
	"iam.members": {
		{applicationCode: AppIAM, resourceType: ResourcePermissionAssign, actions: []Action{ActionRead, ActionCreate, ActionDelete}},
		{applicationCode: AppIAM, resourceType: ResourceIAMAccount, actions: []Action{ActionRead}},
		{applicationCode: AppIAM, resourceType: ResourceUserGroup, actions: []Action{ActionRead}},
		{applicationCode: AppIAM, resourceType: ResourcePermissionSet, actions: []Action{ActionRead}},
		{applicationCode: AppIAM, resourceType: ResourceDataScope, actions: []Action{ActionRead}},
		{applicationCode: AppIAM, resourceType: ResourceType("authz"), actions: []Action{Action("explain")}},
	},
	"iam.user_groups": {
		{applicationCode: AppIAM, resourceType: ResourceUserGroup, actions: []Action{ActionRead, ActionCreate, ActionUpdate, ActionDelete}},
		{applicationCode: AppIAM, resourceType: ResourceIAMAccount, actions: []Action{ActionRead}},
		{applicationCode: AppIAM, resourceType: ResourcePermissionSet, actions: []Action{ActionRead}},
	},
	"iam.permission_sets": {
		{applicationCode: AppIAM, resourceType: ResourcePermissionSet, actions: []Action{ActionRead, ActionCreate, ActionUpdate, ActionDelete}},
		{applicationCode: AppIAM, resourceType: ResourcePermission, actions: []Action{ActionRead}},
		{applicationCode: AppIAM, resourceType: ResourcePermissionPackage, actions: []Action{ActionRead}},
	},
	"iam.assignments": {
		{applicationCode: AppIAM, resourceType: ResourcePermissionAssign, actions: []Action{ActionRead, ActionCreate, ActionDelete}},
		{applicationCode: AppIAM, resourceType: ResourceIAMAccount, actions: []Action{ActionRead}},
		{applicationCode: AppIAM, resourceType: ResourceUserGroup, actions: []Action{ActionRead}},
		{applicationCode: AppIAM, resourceType: ResourcePermissionSet, actions: []Action{ActionRead}},
		{applicationCode: AppIAM, resourceType: ResourceDataScope, actions: []Action{ActionRead}},
		{applicationCode: AppIAM, resourceType: ResourceAssumableRole, actions: []Action{ActionRead}},
	},
	"iam.assumable_roles": {
		{applicationCode: AppIAM, resourceType: ResourceAssumableRole, actions: []Action{ActionRead, ActionCreate, ActionUpdate, ActionDelete, ActionAssume}},
		{applicationCode: AppIAM, resourceType: ResourcePermissionSet, actions: []Action{ActionRead}},
	},
	"iam.policies": {
		{applicationCode: AppIAM, resourceType: ResourceDataScope, actions: []Action{ActionRead, ActionCreate, ActionUpdate, ActionDelete}},
		{applicationCode: AppIAM, resourceType: ResourceFieldPolicy, actions: []Action{ActionRead, ActionCreate, ActionUpdate, ActionDelete}},
	},
	"audit.logs": {
		{applicationCode: AppAudit, resourceType: ResourceType("audit_log"), actions: []Action{ActionRead}},
	},
	"workbench": {
		{applicationCode: AppPlatform, resourceType: ResourceType("me"), actions: []Action{ActionRead}},
	},
	"agents.runs": {
		{applicationCode: AppAgent, resourceType: ResourceType("run"), actions: []Action{ActionRead, ActionCreate, ActionUpdate, ActionDelete}},
	},
	"workflow.instances": {
		{applicationCode: AppWorkflow, resourceType: ResourceFormInstance, actions: []Action{ActionRead, ActionCreate, ActionUpdate, ActionDelete, ActionSubmit, ActionApprove}},
	},
}

var pagePermissionAliases = map[string]string{
	"audit":               "audit.logs",
	"hr.reporting":        "hr.organization",
	"attendance":          "attendance.overview",
	"attendance.leave":    "attendance.leave_policy",
	"workspace.audit-log": "audit.logs",
}

var legacyMenuPrimaryReads = map[string]string{
	"attendance.corrections": permissionKey(AppAttendance, ResourceAttendanceCorrection, ActionRead),
	"attendance.worksites":   permissionKey(AppAttendance, ResourceAttendanceWorksite, ActionRead),
	"attendance.shifts":      permissionKey(AppAttendance, ResourceAttendanceShift, ActionRead),
}

// canonicalPageMenuKey 將舊選單 key 正規化成目前 workspace 頁面 key。
func canonicalPageMenuKey(menuKey string) string {
	menuKey = strings.TrimSpace(menuKey)
	if canonical, ok := pagePermissionAliases[menuKey]; ok {
		return canonical
	}
	return menuKey
}

// pagePermissionPrimaryRead 推導頁面主資源的 read 權限，缺少時整個 bundle fail closed。
func pagePermissionPrimaryRead(menuKey string) (string, bool) {
	canonicalMenuKey := canonicalPageMenuKey(menuKey)
	resources, ok := pagePermissionBundles[canonicalMenuKey]
	if !ok || len(resources) == 0 {
		return "", false
	}
	primary := resources[0]
	for _, action := range primary.actions {
		if action != ActionRead {
			continue
		}
		if _, exists := routePermissionRiskLevel(primary.applicationCode, primary.resourceType, action); !exists {
			return "", false
		}
		return permissionKey(primary.applicationCode, primary.resourceType, action), true
	}
	return "", false
}

// menuPrimaryReadPermissionKey 回傳 navigation menu 顯示所需的主 read 權限。
func menuPrimaryReadPermissionKey(menuKey string) (string, bool) {
	menuKey = strings.TrimSpace(menuKey)
	if primaryRead, ok := pagePermissionPrimaryRead(menuKey); ok {
		return primaryRead, true
	}
	primaryRead, ok := legacyMenuPrimaryReads[menuKey]
	return primaryRead, ok
}

// routePermissionRiskLevel 驗證 resource/action 確實存在於路由政策並回傳最高風險分級。
func routePermissionRiskLevel(applicationCode ApplicationCode, resourceType ResourceType, action Action) (string, bool) {
	found := false
	riskLevel := string(domain.RiskNormal)
	for _, policy := range domain.DefaultRoutePolicies {
		if !strings.EqualFold(policy.ApplicationCode, string(applicationCode)) ||
			!strings.EqualFold(policy.ResourceType, string(resourceType)) ||
			!strings.EqualFold(policy.Action, string(action)) {
			continue
		}
		found = true
		riskLevel = maxRiskLevel(riskLevel, normalizedRiskLevel(policy.RiskLevel))
	}
	return riskLevel, found
}

// canonicalMenuKeyForPermission 將具體操作歸到其主要 workspace 頁面。
func canonicalMenuKeyForPermission(permission Permission) string {
	permission = normalizePermission(permission)
	if permission.PermissionType == PermissionTypeMenu {
		menuKey := strings.TrimSpace(permission.MenuKey)
		if menuKey == "" {
			menuKey = permission.Resource
		}
		return canonicalPageMenuKey(menuKey)
	}
	key := permissionKey(permission.ApplicationCode, permission.ResourceType, permission.Action)
	switch {
	case strings.HasPrefix(key, "hr.employee."):
		return "hr.employees"
	case strings.HasPrefix(key, "hr.org_unit."):
		return "hr.org_units"
	case strings.HasPrefix(key, "hr.position."):
		return "hr.positions"
	case strings.HasPrefix(key, "hr.employment_contract."):
		return "hr.employees"
	case key == "attendance.clock.export", key == "attendance.clock.import":
		return "attendance.overview"
	case strings.HasPrefix(key, "attendance.clock."), strings.HasPrefix(key, "attendance.correction."):
		return "attendance.clock"
	case strings.HasPrefix(key, "attendance.leave."), strings.HasPrefix(key, "attendance.worksite."), strings.HasPrefix(key, "attendance.shift."):
		return "attendance.leave_policy"
	case strings.HasPrefix(key, "workflow.form_template."):
		return "workflow.forms"
	case strings.HasPrefix(key, "workflow.form_instance."):
		return "workflow.instances"
	case strings.HasPrefix(key, "agent.model."):
		return "agents.models"
	case strings.HasPrefix(key, "agent.definition."):
		return "agents.definitions"
	case strings.HasPrefix(key, "agent.knowledge_base."):
		return "agents.knowledge_bases"
	case key == "agent.tool.read":
		return "agents.tools"
	case strings.HasPrefix(key, "agent.tool."):
		return "agents.runs"
	case strings.HasPrefix(key, "agent.run."):
		return "agents.runs"
	case strings.HasPrefix(key, "iam.user_group."):
		return "iam.user_groups"
	case strings.HasPrefix(key, "iam.permission_set_assignment."):
		return "iam.assignments"
	case strings.HasPrefix(key, "iam.permission_set.") || strings.HasPrefix(key, "iam.permission.") || strings.HasPrefix(key, "iam.permission_package."):
		return "iam.permission_sets"
	case strings.HasPrefix(key, "iam.assumable_role."):
		return "iam.assumable_roles"
	case strings.HasPrefix(key, "iam.data_scope."), strings.HasPrefix(key, "iam.field_policy."):
		return "iam.policies"
	case strings.HasPrefix(key, "audit.audit_log."):
		return "audit.logs"
	case key == "platform.me.read":
		return "workbench"
	default:
		return ""
	}
}
