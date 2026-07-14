package service

import (
	"sort"
	"strings"

	"nexus-pro-be/internal/utils"
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
	if v, ok, err := c.employeeForAccount(ctx, account); err != nil {
		return MeResponse{}, err
	} else if ok {
		emp := c.enrichEmployeeProfile(ctx, v)
		employee = &emp
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

// UpdateProfile applies the allowlisted self-service fields to the current user's linked employee.
func (c MeService) UpdateProfile(ctx RequestContext, input UpdateMeProfileInput) (MeResponse, error) {
	account, _, authzAudit, err := c.Authorize(ctx,
		CheckRequest{Resource: "me", Action: ActionUpdate, Scope: ScopeSelf},
		AuditTarget{Event: "me.profile.update", Resource: "me"},
	)
	if err != nil {
		return MeResponse{}, err
	}
	if err := c.withTransaction(ctx, func(tx MeService) error {
		next, ok, err := tx.employeeForAccount(ctx, account)
		if err != nil {
			return err
		}
		if !ok {
			return NotFound("employee profile", account.ID)
		}
		authzAudit.target.Target = next.ID
		next.BasicInfo = utils.CopyStringMap(next.BasicInfo)
		next.ContactInfo = utils.CopyStringMap(next.ContactInfo)
		changedFields := applyMeProfilePatch(&next, input)
		if len(changedFields) > 0 {
			next.UpdatedAt = tx.Now()
			if err := tx.store.UpsertEmployee(goContext(ctx), next); err != nil {
				return err
			}
			if err := tx.Service.HR().appendEmployeeEvent(ctx, string(EventEmployeeUpdated), next.ID, map[string]any{
				"employee_id":    next.ID,
				"source":         "self_service",
				"changed_fields": changedFields,
			}); err != nil {
				return err
			}
			if err := tx.audit(ctx, "platform.me.profile.update", string(ResourceEmployee), next.ID, string(SeverityMedium), map[string]any{
				"changed_fields": changedFields,
			}); err != nil {
				return err
			}
		}
		return authzAudit.CommitWith(ctx, tx.Service)
	}); err != nil {
		return MeResponse{}, err
	}
	return c.Resolve(ctx)
}

// employeeForAccount resolves both modern account.employee_id links and legacy employee.account_id links.
func (c MeService) employeeForAccount(ctx RequestContext, account Account) (Employee, bool, error) {
	if employeeID := strings.TrimSpace(account.EmployeeID); employeeID != "" {
		employee, ok, err := c.store.GetEmployee(goContext(ctx), ctx.TenantID, employeeID)
		if err != nil || ok {
			return employee, ok, err
		}
	}
	return c.store.GetEmployeeByAccountID(goContext(ctx), ctx.TenantID, account.ID)
}

// applyMeProfilePatch updates only fields explicitly present in the self-service request.
func applyMeProfilePatch(employee *Employee, input UpdateMeProfileInput) []string {
	changedFields := make([]string, 0, 5)
	if input.EnglishName != nil {
		value := strings.TrimSpace(*input.EnglishName)
		if stringFromAny(employee.BasicInfo["name_en"]) != value || stringFromAny(employee.BasicInfo["name_en_source"]) != "self" {
			changedFields = append(changedFields, "english_name")
		}
		employee.BasicInfo["name_en"] = value
		employee.BasicInfo["name_en_source"] = "self"
	}
	if input.MobilePhone != nil {
		value := strings.TrimSpace(*input.MobilePhone)
		if employee.Phone != value || stringFromAny(employee.ContactInfo["mobile_phone"]) != value {
			changedFields = append(changedFields, "mobile_phone")
		}
		employee.Phone = value
		employee.ContactInfo["mobile_phone"] = value
	}
	if input.Extension != nil {
		value := strings.TrimSpace(*input.Extension)
		if stringFromAny(employee.ContactInfo["extension"]) != value {
			changedFields = append(changedFields, "extension")
		}
		employee.ContactInfo["extension"] = value
	}
	if input.Slack != nil {
		value := strings.TrimSpace(*input.Slack)
		if stringFromAny(employee.ContactInfo["slack"]) != value {
			changedFields = append(changedFields, "slack")
		}
		employee.ContactInfo["slack"] = value
	}
	if input.EmergencyContactName != nil {
		value := strings.TrimSpace(*input.EmergencyContactName)
		if stringFromAny(employee.ContactInfo["emergency_contact_name"]) != value {
			changedFields = append(changedFields, "emergency_contact_name")
		}
		employee.ContactInfo["emergency_contact_name"] = value
	}
	return changedFields
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
	effectivePermissionKeys := map[string]struct{}{}
	for _, perm := range perms {
		perm = normalizePermission(perm)
		if permissionCanAuthorizeRequest(perm) {
			effectivePermissionKeys[permissionKey(perm.ApplicationCode, perm.ResourceType, perm.Action)] = struct{}{}
		}
	}
	keys := make([]string, 0, len(perms))
	for _, perm := range perms {
		menuKey := strings.TrimSpace(perm.MenuKey)
		if menuKey == "" && perm.PermissionType == PermissionTypeMenu {
			menuKey = strings.TrimSpace(perm.Resource)
		}
		if menuKey == "" {
			continue
		}
		primaryRead, ok := menuPrimaryReadPermissionKey(menuKey)
		if !ok {
			normalized := normalizePermission(perm)
			if !permissionCanAuthorizeRequest(normalized) || normalized.Action != ActionRead {
				continue
			}
			primaryRead = permissionKey(normalized.ApplicationCode, normalized.ResourceType, normalized.Action)
		}
		if _, effective := effectivePermissionKeys[primaryRead]; !effective {
			continue
		}
		keys = append(keys, menuKey)
		if canonical := canonicalPageMenuKey(menuKey); canonical != menuKey {
			keys = append(keys, canonical)
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
		Key:   "workspace",
		Label: "工作區設定",
		Path:  "/workspace",
		Children: []MenuNode{
			{Key: "workspace.overview", Label: "概覽", Path: "/workspace/overview"},
			{Key: "hr.employees", Label: "員工", Path: "/workspace/employees"},
			{Key: "hr.org_units", Label: "組織單元", Path: "/workspace/org-units"},
			{Key: "hr.positions", Label: "崗位", Path: "/workspace/positions"},
			{Key: "hr.organization", Label: "組織架構", Path: "/workspace/organization"},
			{Key: "hr.turnover", Label: "在職分析", Path: "/workspace/turnover"},
			{Key: "attendance.overview", Label: "工時統計", Path: "/workspace/attendance"},
			{Key: "attendance.clock", Label: "打卡時間", Path: "/workspace/clock"},
			{Key: "attendance.leave_policy", Label: "假勤制度", Path: "/workspace/leave-policy"},
			{Key: "workflow.forms", Label: "表單設計", Path: "/workspace/forms"},
			{Key: "agents.models", Label: "模型設定", Path: "/workspace/agent-models"},
			{Key: "agents.definitions", Label: "Agent 管理", Path: "/workspace/agents"},
			{Key: "agents.knowledge_bases", Label: "知識庫", Path: "/workspace/knowledge-bases"},
			{Key: "agents.tools", Label: "工具與整合", Path: "/workspace/agent-tools"},
			{Key: "iam.members", Label: "成員權限", Path: "/workspace/iam/members"},
			{Key: "iam.user_groups", Label: "使用者群組", Path: "/workspace/iam/user-groups"},
			{Key: "iam.permission_sets", Label: "權限集合", Path: "/workspace/iam/permission-sets"},
			{Key: "iam.assignments", Label: "權限指派", Path: "/workspace/iam/assignments"},
			{Key: "iam.assumable_roles", Label: "可承擔角色", Path: "/workspace/iam/roles"},
			{Key: "iam.policies", Label: "數據策略", Path: "/workspace/iam/policies"},
			{Key: "audit.logs", Label: "操作紀錄", Path: "/workspace/audit-log"},
		},
	},
	{
		Key:   "hr",
		Label: "HR 主資料",
		Path:  "/workspace",
		Children: []MenuNode{
			{Key: "hr.reporting", Label: "匯報關係", Path: "/workspace/organization"},
		},
	},
	{
		Key:   "attendance",
		Label: "假勤自助",
		Path:  "/workspace/attendance",
		Children: []MenuNode{
			{Key: "attendance.corrections", Label: "補卡申請", Path: "/workspace/clock"},
			{Key: "attendance.leave", Label: "請假申請", Path: "/workspace/leave-policy"},
			{Key: "attendance.worksites", Label: "辦公地點", Path: "/workspace/leave-policy"},
			{Key: "attendance.shifts", Label: "班次規則", Path: "/workspace/leave-policy"},
		},
	},
	{
		Key:   "workflow",
		Label: "表單審批",
		Path:  "/forms",
		Children: []MenuNode{
			{Key: "workflow.instances", Label: "流程實例", Path: "/forms"},
		},
	},
	{Key: "iam", Label: "權限中心", Path: "/workspace/iam/permission-sets"},
	{
		Key:   "agents",
		Label: "AI Agent",
		Path:  "/agents",
		Children: []MenuNode{
			{Key: "agents.runs", Label: "Agent Runs", Path: "/agents/runs"},
		},
	},
	{Key: "audit", Label: "審計中心", Path: "/workspace/audit-log"},
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
