package service

type MeService struct {
	*Service
	store meStore
}

func (c *Service) Me() MeService {
	return MeService{Service: c, store: c.store}
}

func (c MeService) Resolve(ctx RequestContext) (MeResponse, error) {
	account, tenant, err := c.resolveAccount(ctx)
	if err != nil {
		return MeResponse{}, err
	}
	decision, err := c.evaluateAuthz(ctx, account, CheckRequest{Resource: "me", Action: ActionRead})
	if err != nil {
		return MeResponse{}, err
	}
	if !decision.Allowed {
		return MeResponse{}, Forbidden(decision.Reason)
	}

	permissions, permissionSets, groups, err := c.resolveAccess(ctx, account)
	if err != nil {
		return MeResponse{}, err
	}

	var employee *Employee
	if account.EmployeeID != "" {
		v, ok, err := c.store.GetEmployee(goContext(ctx), ctx.TenantID, account.EmployeeID)
		if err != nil {
			return MeResponse{}, err
		}
		if ok {
			emp := v
			employee = &emp
		}
	}

	role, _, err := c.activeAssumableRole(ctx, account)
	if err != nil {
		return MeResponse{}, err
	}
	var assumedRole *AssumableRole
	if role != nil {
		assumedRole = role
	}

	effectiveMenuKeys := uniqueStrings(menuKeysFromPermissions(permissions))
	capabilities := uniqueStrings(capabilitiesFromPermissions(permissions))

	return MeResponse{
		Tenant:               tenant,
		Account:              account,
		Employee:             employee,
		AssumedRole:          assumedRole,
		UserGroups:           groups,
		PermissionSets:       permissionSets,
		EffectivePermissions: permissions,
		EffectiveMenuKeys:    effectiveMenuKeys,
		Capabilities:         capabilities,
	}, nil
}

func (c MeService) ListMenus(ctx RequestContext) ([]MenuNode, error) {
	me, err := c.Resolve(ctx)
	if err != nil {
		return nil, err
	}
	allowed := map[string]struct{}{}
	for _, key := range me.EffectiveMenuKeys {
		allowed[key] = struct{}{}
	}
	return filterMenus(defaultMenuCatalog, allowed), nil
}

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
