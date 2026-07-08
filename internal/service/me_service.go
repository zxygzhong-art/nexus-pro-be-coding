package service

import (
	"sort"
	"strings"
)

// MeService 定義 me 服務的資料結構。
type MeService struct {
	*Service
	store meStore
}

// Me 處理 me 的服務流程。
func (c *Service) Me() MeService {
	return MeService{Service: c, store: c.store}
}

// Resolve 解析對應的服務流程。
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
			emp := c.enrichEmployeeProfile(ctx, v)
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

// ListMenus 列出 menus 的服務流程。
func (c MeService) ListMenus(ctx RequestContext) ([]MenuNode, error) {
	me, err := c.Resolve(ctx)
	if err != nil {
		return nil, err
	}
	allowed := map[string]struct{}{}
	for _, key := range me.EffectiveMenuKeys {
		allowed[key] = struct{}{}
	}
	nodes := defaultMenuCatalog
	items, err := c.store.ListMenuItems(goContext(ctx), ctx.TenantID)
	if err != nil {
		return nil, err
	}
	if len(items) > 0 {
		nodes = menuTreeFromItems(items)
	}
	return filterMenus(nodes, allowed), nil
}

// menuKeysFromPermissions 處理 menu keys 來源 權限。
func menuKeysFromPermissions(perms []Permission) []string {
	keys := make([]string, 0, len(perms))
	for _, perm := range perms {
		if perm.MenuKey != "" {
			keys = append(keys, perm.MenuKey)
		}
	}
	return keys
}

// filterMenus 處理篩選 menus。
func filterMenus(nodes []MenuNode, allowed map[string]struct{}) []MenuNode {
	out := make([]MenuNode, 0)
	for _, node := range nodes {
		children := filterMenus(node.Children, allowed)
		_, ok := allowed[node.Key]
		if ok || len(children) > 0 {
			copyNode := MenuNode{Key: node.Key, Label: node.Label, Path: node.Path, Icon: node.Icon}
			if len(children) > 0 {
				copyNode.Children = children
			}
			out = append(out, copyNode)
		}
	}
	return out
}

// menuTreeFromItems 從落庫選單項建立樹。
func menuTreeFromItems(items []MenuItem) []MenuNode {
	children := map[string][]MenuItem{}
	for _, item := range items {
		children[item.ParentKey] = append(children[item.ParentKey], item)
	}
	for parentKey := range children {
		sort.SliceStable(children[parentKey], func(i, j int) bool {
			if children[parentKey][i].SortOrder != children[parentKey][j].SortOrder {
				return children[parentKey][i].SortOrder < children[parentKey][j].SortOrder
			}
			return children[parentKey][i].Key < children[parentKey][j].Key
		})
	}
	visited := map[string]struct{}{}
	return menuNodesFromChildren("", children, visited)
}

// menuNodesFromChildren 遞迴建立選單子節點。
func menuNodesFromChildren(parentKey string, children map[string][]MenuItem, visited map[string]struct{}) []MenuNode {
	items := children[parentKey]
	out := make([]MenuNode, 0, len(items))
	for _, item := range items {
		if _, ok := visited[item.Key]; ok {
			continue
		}
		visited[item.Key] = struct{}{}
		node := MenuNode{
			Key:      item.Key,
			Label:    item.Label,
			Path:     item.Path,
			Icon:     item.Icon,
			Children: menuNodesFromChildren(item.Key, children, visited),
		}
		out = append(out, node)
	}
	return out
}

var defaultMenuCatalog = []MenuNode{
	{Key: "workbench", Label: "工作台", Path: "/"},
	{
		Key:   "hr",
		Label: "HR 主資料",
		Path:  "/workspace",
		Children: []MenuNode{
			{Key: "hr.employees", Label: "員工", Path: "/workspace/employees"},
			{Key: "hr.org_units", Label: "組織單元", Path: "/workspace/org-units"},
			{Key: "hr.positions", Label: "崗位", Path: "/workspace/positions"},
			{Key: "hr.reporting", Label: "匯報關係", Path: "/workspace/organization"},
			{
				Key:   "attendance",
				Label: "假勤",
				Path:  "/workspace/attendance",
				Children: []MenuNode{
					{Key: "attendance.clock", Label: "上下班打卡", Path: "/workspace/clock"},
					{Key: "attendance.corrections", Label: "補卡申請", Path: "/workspace/clock"},
					{Key: "attendance.leave", Label: "請假申請", Path: "/workspace/leave-policy"},
					{Key: "attendance.worksites", Label: "辦公地點", Path: "/workspace/leave-policy"},
					{Key: "attendance.shifts", Label: "班次規則", Path: "/workspace/leave-policy"},
					{Key: "attendance.shift_assignments", Label: "員工排班", Path: "/workspace/leave-policy"},
				},
			},
		},
	},
	{
		Key:   "workflow",
		Label: "表單審批",
		Path:  "/workspace/forms",
		Children: []MenuNode{
			{Key: "workflow.forms", Label: "動態表單", Path: "/workspace/forms"},
			{Key: "workflow.instances", Label: "流程實例", Path: "/workspace/forms"},
		},
	},
	{
		Key:   "iam",
		Label: "權限中心",
		Path:  "/iam/permission-sets",
		Children: []MenuNode{
			{Key: "iam.user_groups", Label: "使用者群組", Path: "/iam/user-groups"},
			{Key: "iam.permission_sets", Label: "權限集合", Path: "/iam/permission-sets"},
			{Key: "iam.assumable_roles", Label: "可承擔身分", Path: "/iam/assumable-roles"},
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
		Label: "審計中心",
		Path:  "/workspace/audit-log",
	},
}

func (c MeService) enrichEmployeeProfile(ctx RequestContext, employee Employee) Employee {
	if employee.EmploymentInfo == nil {
		employee.EmploymentInfo = map[string]any{}
	}
	if employee.Position != "" {
		employee.EmploymentInfo["job_title"] = employee.Position
		employee.EmploymentInfo["position"] = employee.Position
	}
	orgUnitID := strings.TrimSpace(employee.OrgUnitID)
	if orgUnitID == "" && employee.EmploymentInfo != nil {
		if value, ok := employee.EmploymentInfo["org_unit_id"].(string); ok {
			orgUnitID = strings.TrimSpace(value)
		}
	}
	if orgUnitID != "" {
		if ou, ok, err := c.store.GetOrgUnit(goContext(ctx), ctx.TenantID, orgUnitID); err == nil && ok {
			employee.EmploymentInfo["department_name"] = ou.Name
			employee.EmploymentInfo["org_unit_name"] = ou.Name
		}
	}
	return employee
}
