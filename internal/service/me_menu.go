package service

func menuKeysFromPermissions(perms []Permission) []string {
	keys := make([]string, 0, len(perms))
	for _, perm := range perms {
		if perm.MenuKey != "" {
			keys = append(keys, perm.MenuKey)
		}
	}
	return keys
}

func filterMenus(nodes []MenuNode, allowed map[string]struct{}) []MenuNode {
	out := make([]MenuNode, 0)
	for _, node := range nodes {
		children := filterMenus(node.Children, allowed)
		_, ok := allowed[node.Key]
		if ok || len(children) > 0 {
			copyNode := MenuNode{Key: node.Key, Label: node.Label, Path: node.Path}
			if len(children) > 0 {
				copyNode.Children = children
			}
			out = append(out, copyNode)
		}
	}
	return out
}

var defaultMenuCatalog = []MenuNode{
	{Key: "workbench", Label: "工作台", Path: "/"},
	{
		Key:   "hr",
		Label: "HR 主数据",
		Path:  "/hr",
		Children: []MenuNode{
			{Key: "hr.employees", Label: "员工", Path: "/hr/employees"},
			{Key: "hr.org_units", Label: "组织", Path: "/org/units"},
			{Key: "attendance", Label: "假勤", Path: "/attendance"},
		},
	},
	{
		Key:   "workflow",
		Label: "表单审批",
		Path:  "/workflows",
		Children: []MenuNode{
			{Key: "workflow.forms", Label: "动态表单", Path: "/forms/templates"},
			{Key: "workflow.instances", Label: "流程实例", Path: "/workflows/forms"},
			{Key: "attendance.leave", Label: "请假申请", Path: "/attendance/leave-requests"},
		},
	},
	{
		Key:   "iam",
		Label: "权限中心",
		Path:  "/iam",
		Children: []MenuNode{
			{Key: "iam.user_groups", Label: "用户组", Path: "/iam/user-groups"},
			{Key: "iam.permission_sets", Label: "权限集合", Path: "/iam/permission-sets"},
			{Key: "iam.assumable_roles", Label: "可承担身份", Path: "/iam/assumable-roles"},
		},
	},
	{
		Key:   "agents",
		Label: "AI Agent",
		Path:  "/agents",
		Children: []MenuNode{
			{Key: "agents.runs", Label: "Agent Runs", Path: "/agents/runs"},
		},
	},
	{
		Key:   "audit",
		Label: "审计中心",
		Path:  "/audit-logs",
	},
}
