package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
	"unicode"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/repository"
	"nexus-pro-be/internal/utils"
)

// TenantProvisionInput 定義租戶開通輸入。
type TenantProvisionInput struct {
	TenantID         string `json:"tenant_id"`
	TenantName       string `json:"tenant_name,omitempty"`
	AdminEmail       string `json:"admin_email"`
	AdminName        string `json:"admin_name,omitempty"`
	AdminEmployeeNo  string `json:"admin_employee_no,omitempty"`
	IdentityProvider string `json:"identity_provider,omitempty"`
	IdentitySubject  string `json:"identity_subject"`
}

// TenantProvisionIDs 定義租戶開通會產生的穩定 ID。
type TenantProvisionIDs struct {
	RootOrgUnitID        string `json:"root_org_unit_id"`
	AdminAccountID       string `json:"admin_account_id"`
	AdminEmployeeID      string `json:"admin_employee_id"`
	AdminPermissionSetID string `json:"admin_permission_set_id"`
}

// TenantProvisionResult 定義租戶開通結果。
type TenantProvisionResult struct {
	TenantID             string `json:"tenant_id"`
	TenantName           string `json:"tenant_name"`
	RootOrgUnitID        string `json:"root_org_unit_id"`
	AdminAccountID       string `json:"admin_account_id"`
	AdminEmployeeID      string `json:"admin_employee_id"`
	AdminEmployeeNo      string `json:"admin_employee_no"`
	AdminPermissionSetID string `json:"admin_permission_set_id"`
	AdminEmail           string `json:"admin_email"`
	IdentityProvider     string `json:"identity_provider"`
	IdentitySubject      string `json:"identity_subject"`
	PermissionVersion    int64  `json:"permission_version"`
}

// DefaultTenantProvisionIDs 建立租戶開通用的穩定 ID。
func DefaultTenantProvisionIDs(tenantID string) (TenantProvisionIDs, error) {
	tenantID = strings.TrimSpace(tenantID)
	if tenantID == "" {
		return TenantProvisionIDs{}, BadRequest("tenant_id is required")
	}
	slug := safeTenantProvisionSlug(tenantID)
	hash := shortTenantProvisionHash(tenantID)
	stem := slug + "-" + hash
	return TenantProvisionIDs{
		RootOrgUnitID:        "ou-" + stem + "-root",
		AdminAccountID:       "acct-" + stem + "-admin",
		AdminEmployeeID:      "emp-" + stem + "-admin",
		AdminPermissionSetID: "ps-" + stem + "-platform-admin",
	}, nil
}

// ProvisionTenant 建立租戶、首管理員、權限集合與 OIDC 身分綁定。
func (c *Service) ProvisionTenant(ctx context.Context, input TenantProvisionInput) (TenantProvisionResult, error) {
	normalized, ids, err := normalizeTenantProvisionInput(input)
	if err != nil {
		return TenantProvisionResult{}, err
	}
	now := c.Now()
	result := TenantProvisionResult{
		TenantID:             normalized.TenantID,
		TenantName:           normalized.TenantName,
		RootOrgUnitID:        ids.RootOrgUnitID,
		AdminAccountID:       ids.AdminAccountID,
		AdminEmployeeID:      ids.AdminEmployeeID,
		AdminEmployeeNo:      normalized.AdminEmployeeNo,
		AdminPermissionSetID: ids.AdminPermissionSetID,
		AdminEmail:           normalized.AdminEmail,
		IdentityProvider:     normalized.IdentityProvider,
		IdentitySubject:      normalized.IdentitySubject,
	}
	if ctx == nil {
		ctx = context.Background()
	}
	err = repository.WithinTenantTransaction(ctx, c.store, normalized.TenantID, func(tx repository.Store) error {
		txService := *c
		txService.store = tx
		txCtx := RequestContext{Context: ctx, TenantID: normalized.TenantID}
		if err := tx.UpsertTenant(ctx, domain.Tenant{ID: normalized.TenantID, Name: normalized.TenantName, CreatedAt: now}); err != nil {
			return err
		}
		if _, err := ensureTenantDefaultFormTemplates(ctx, tx, normalized.TenantID, now); err != nil {
			return err
		}
		if err := syncPermissionCatalogForTenant(ctx, tx, normalized.TenantID, now); err != nil {
			return err
		}
		rootOrg := tenantProvisionRootOrgUnit(normalized, ids, now)
		existingRoot, rootExists, err := tx.GetOrgUnit(ctx, normalized.TenantID, rootOrg.ID)
		if err != nil {
			return err
		}
		if err := tx.UpsertOrgUnit(ctx, rootOrg); err != nil {
			return err
		}
		if !rootExists {
			existingRoot = domain.OrgUnit{}
		}
		if err := txService.syncOrgUnitRelationshipTuples(txCtx, existingRoot, rootOrg); err != nil {
			return err
		}
		adminPermissionSet := tenantProvisionAdminPermissionSet(normalized, ids, now)
		if err := tx.UpsertPermissionSet(ctx, adminPermissionSet); err != nil {
			return err
		}
		if _, err := syncPermissionSetItems(ctx, tx, adminPermissionSet, now); err != nil {
			return err
		}
		adminAccount := tenantProvisionAdminAccount(normalized, ids, now)
		existingAccount, accountExists, err := tx.GetAccount(ctx, normalized.TenantID, adminAccount.ID)
		if err != nil {
			return err
		}
		if err := tx.UpsertAccount(ctx, adminAccount); err != nil {
			return err
		}
		if !accountExists {
			existingAccount = domain.Account{}
		}
		if err := txService.syncAccountTenantMembershipTuple(txCtx, existingAccount, adminAccount); err != nil {
			return err
		}
		adminEmployee := tenantProvisionAdminEmployee(normalized, ids, now)
		existingEmployee, employeeExists, err := tx.GetEmployee(ctx, normalized.TenantID, adminEmployee.ID)
		if err != nil {
			return err
		}
		if err := tx.UpsertEmployee(ctx, adminEmployee); err != nil {
			return err
		}
		if !employeeExists {
			existingEmployee = domain.Employee{}
		}
		if existingEmployee.OrgUnitID != adminEmployee.OrgUnitID || existingEmployee.AccountID != adminEmployee.AccountID || existingEmployee.ManagerEmployeeID != adminEmployee.ManagerEmployeeID {
			if err := txService.HR().syncEmployeeRelationshipTuples(txCtx, existingEmployee, adminEmployee); err != nil {
				return err
			}
		}
		if err := assertTenantProvisionIdentityAvailable(ctx, tx, normalized, ids); err != nil {
			return err
		}
		if err := tx.UpsertUserIdentity(ctx, tenantProvisionAdminIdentity(normalized, ids, now)); err != nil {
			return err
		}
		version, err := tx.IncrementPermissionVersion(ctx, normalized.TenantID)
		if err != nil {
			return err
		}
		result.PermissionVersion = version
		return tx.AppendOutboxEvent(ctx, domain.OutboxEvent{
			ID:            utils.NewID("outbox"),
			TenantID:      normalized.TenantID,
			EventType:     "tenant.provisioned",
			AggregateType: domain.OutboxAggregateAuthz,
			AggregateID:   normalized.TenantID,
			Payload: map[string]any{
				"tenant_id":               normalized.TenantID,
				"admin_account_id":        ids.AdminAccountID,
				"admin_employee_id":       ids.AdminEmployeeID,
				"admin_permission_set_id": ids.AdminPermissionSetID,
				"permission_version":      version,
			},
			Status:    "pending",
			CreatedAt: now,
		})
	})
	if err != nil {
		return TenantProvisionResult{}, err
	}
	c.invalidateAuthzSnapshots(ctx, normalized.TenantID)
	return result, nil
}

// EnsureTenantDefaultFormTemplates 幂等补齐既有租户缺少的内建表单，不覆盖管理员已配置的同 key 模板。
func (c *Service) EnsureTenantDefaultFormTemplates(ctx context.Context, tenantID string) (int, error) {
	tenantID = strings.TrimSpace(tenantID)
	if tenantID == "" {
		return 0, BadRequest("tenant_id is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	created := 0
	err := repository.WithinTenantTransaction(ctx, c.store, tenantID, func(tx repository.Store) error {
		if _, ok, err := tx.GetTenant(ctx, tenantID); err != nil {
			return err
		} else if !ok {
			return NotFound("tenant", tenantID)
		}
		var err error
		created, err = ensureTenantDefaultFormTemplates(ctx, tx, tenantID, c.Now())
		return err
	})
	return created, err
}

// ensureTenantDefaultFormTemplates 只建立缺失的默认模板，保留租户对现有模板的停用、删除与自定义配置。
func ensureTenantDefaultFormTemplates(ctx context.Context, store repository.Store, tenantID string, now time.Time) (int, error) {
	if _, exists, err := store.GetFormTemplateByKey(ctx, tenantID, "leave-request"); err != nil {
		return 0, err
	} else if exists {
		return 0, nil
	}
	if err := store.UpsertFormTemplate(ctx, tenantDefaultLeaveFormTemplate(tenantID, now)); err != nil {
		return 0, err
	}
	return 1, nil
}

// tenantDefaultLeaveFormTemplate 建立 Agent、表单中心与工作流共用的可提交请假模板。
func tenantDefaultLeaveFormTemplate(tenantID string, now time.Time) domain.FormTemplate {
	return domain.FormTemplate{
		ID:             fmt.Sprintf("ft-%s-%s-leave-request", safeTenantProvisionSlug(tenantID), shortTenantProvisionHash(tenantID)),
		TenantID:       tenantID,
		Key:            "leave-request",
		Name:           "請假申請單",
		Description:    "特休、事假、病假與其他假別申請",
		Status:         "published",
		CurrentVersion: 1,
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"leave_type": map[string]any{"type": "string"},
				"start_at":   map[string]any{"type": "string", "format": "date-time"},
				"end_at":     map[string]any{"type": "string", "format": "date-time"},
				"hours":      map[string]any{"type": "number"},
				"proxy":      map[string]any{"type": "string"},
				"reason":     map[string]any{"type": "string"},
			},
			"required": []string{"leave_type", "start_at", "end_at", "hours", "reason"},
			platformFormDesignSchemaKey: map[string]any{
				"enabled":   true,
				"form_kind": "hybrid",
				"category":  "人事考勤類",
				"icon":      "🗓️",
				"desc":      "特休 / 事假 / 病假 / 公假",
				"fields":    platformLeaveRequestBuilderFields(),
				"stages": []domain.PlatformFormBuilderStage{{
					ID: "stage-manager", Type: "approver", Label: "直屬主管",
					Detail: "依員工主管關係自動帶入", Config: map[string]any{"role": "manager"},
				}},
			},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// normalizeTenantProvisionInput 正規化租戶開通輸入。
func normalizeTenantProvisionInput(input TenantProvisionInput) (TenantProvisionInput, TenantProvisionIDs, error) {
	input.TenantID = strings.TrimSpace(input.TenantID)
	if input.TenantID == "" {
		return TenantProvisionInput{}, TenantProvisionIDs{}, BadRequest("tenant_id is required")
	}
	input.TenantName = strings.TrimSpace(input.TenantName)
	if input.TenantName == "" {
		input.TenantName = input.TenantID
	}
	input.AdminEmail = strings.ToLower(strings.TrimSpace(input.AdminEmail))
	if input.AdminEmail == "" || !strings.Contains(input.AdminEmail, "@") {
		return TenantProvisionInput{}, TenantProvisionIDs{}, BadRequest("admin_email is required")
	}
	input.AdminName = strings.TrimSpace(input.AdminName)
	if input.AdminName == "" {
		input.AdminName = defaultTenantProvisionAdminName(input.AdminEmail)
	}
	input.AdminEmployeeNo = strings.TrimSpace(input.AdminEmployeeNo)
	if input.AdminEmployeeNo == "" {
		input.AdminEmployeeNo = "ADMIN001"
	}
	input.IdentityProvider = strings.TrimSpace(input.IdentityProvider)
	if input.IdentityProvider == "" {
		input.IdentityProvider = domain.IdentityProviderKeycloak
	}
	input.IdentitySubject = strings.TrimSpace(input.IdentitySubject)
	if input.IdentitySubject == "" {
		return TenantProvisionInput{}, TenantProvisionIDs{}, BadRequest("identity_subject is required")
	}
	ids, err := DefaultTenantProvisionIDs(input.TenantID)
	if err != nil {
		return TenantProvisionInput{}, TenantProvisionIDs{}, err
	}
	return input, ids, nil
}

// tenantProvisionRootOrgUnit 建立租戶根組織。
func tenantProvisionRootOrgUnit(input TenantProvisionInput, ids TenantProvisionIDs, now time.Time) domain.OrgUnit {
	return domain.OrgUnit{
		ID:        ids.RootOrgUnitID,
		TenantID:  input.TenantID,
		Code:      "ROOT",
		Name:      input.TenantName,
		Path:      []string{ids.RootOrgUnitID},
		CreatedAt: now,
	}
}

// tenantProvisionAdminPermissionSet 建立首管理員權限集合。
func tenantProvisionAdminPermissionSet(input TenantProvisionInput, ids TenantProvisionIDs, now time.Time) domain.PermissionSet {
	return domain.PermissionSet{
		ID:          ids.AdminPermissionSetID,
		TenantID:    input.TenantID,
		Name:        "Platform Admin",
		Description: "Initial tenant administrator permission set.",
		Permissions: tenantProvisionAdminPermissions(),
		CreatedAt:   now,
	}
}

// tenantProvisionAdminAccount 建立首管理員帳號。
func tenantProvisionAdminAccount(input TenantProvisionInput, ids TenantProvisionIDs, now time.Time) domain.Account {
	return domain.Account{
		ID:                     ids.AdminAccountID,
		TenantID:               input.TenantID,
		DisplayName:            input.AdminName,
		Email:                  input.AdminEmail,
		EmployeeID:             ids.AdminEmployeeID,
		Status:                 string(domain.AccountStatusActive),
		DirectPermissionSetIDs: []string{ids.AdminPermissionSetID},
		CreatedAt:              now,
	}
}

// tenantProvisionAdminEmployee 建立首管理員員工檔。
func tenantProvisionAdminEmployee(input TenantProvisionInput, ids TenantProvisionIDs, now time.Time) domain.Employee {
	return domain.Employee{
		ID:               ids.AdminEmployeeID,
		TenantID:         input.TenantID,
		EmployeeNo:       input.AdminEmployeeNo,
		Name:             input.AdminName,
		CompanyEmail:     input.AdminEmail,
		OrgUnitID:        ids.RootOrgUnitID,
		AccountID:        ids.AdminAccountID,
		Position:         "Tenant Administrator",
		Category:         string(domain.EmployeeCategoryFullTime),
		Status:           string(domain.EmployeeStatusActive),
		EmploymentStatus: string(domain.EmployeeStatusActive),
		BasicInfo:        map[string]any{"name": input.AdminName, "company_email": input.AdminEmail},
		EmploymentInfo:   map[string]any{"position": "Tenant Administrator"},
		CreatedAt:        now,
		UpdatedAt:        now,
	}
}

// tenantProvisionAdminIdentity 建立首管理員外部身分映射。
func tenantProvisionAdminIdentity(input TenantProvisionInput, ids TenantProvisionIDs, now time.Time) domain.UserIdentity {
	providerSlug := safeTenantProvisionSlug(input.IdentityProvider)
	return domain.UserIdentity{
		ID:        fmt.Sprintf("uid-%s-%s-admin-%s", safeTenantProvisionSlug(input.TenantID), shortTenantProvisionHash(input.TenantID), providerSlug),
		TenantID:  input.TenantID,
		AccountID: ids.AdminAccountID,
		Provider:  input.IdentityProvider,
		Subject:   input.IdentitySubject,
		Email:     input.AdminEmail,
		CreatedAt: now,
	}
}

// assertTenantProvisionIdentityAvailable 防止外部 subject 被綁到其他帳號。
func assertTenantProvisionIdentityAvailable(ctx context.Context, store repository.Store, input TenantProvisionInput, ids TenantProvisionIDs) error {
	identity, ok, err := store.GetUserIdentity(ctx, input.TenantID, input.IdentityProvider, input.IdentitySubject)
	if err != nil {
		return err
	}
	if ok && identity.AccountID != ids.AdminAccountID {
		return Conflict("identity_subject is already bound to another account")
	}
	return nil
}

// tenantProvisionAdminPermissions 回傳首管理員可見選單與全域操作權限。
func tenantProvisionAdminPermissions() []domain.Permission {
	permissions := []domain.Permission{
		{Resource: "*", Action: "*", Scope: domain.ScopeAll, MenuKey: "workbench"},
		{Resource: "me", Action: domain.ActionRead, Scope: domain.ScopeAll, MenuKey: "workbench"},
		{Resource: "me", Action: domain.ActionCreate, Scope: domain.ScopeAll, MenuKey: "workbench"},
		{Resource: "me", Action: domain.ActionUpdate, Scope: domain.ScopeAll, MenuKey: "workbench"},
		{Resource: "me", Action: domain.ActionDelete, Scope: domain.ScopeAll, MenuKey: "workbench"},
		{Resource: "hr.employee", Action: domain.ActionRead, Scope: domain.ScopeAll, MenuKey: "hr.employees"},
		{Resource: "hr.employee", Action: domain.ActionCreate, Scope: domain.ScopeAll, MenuKey: "hr.employees"},
		{Resource: "hr.employee", Action: domain.ActionUpdate, Scope: domain.ScopeAll, MenuKey: "hr.employees"},
		{Resource: "hr.employee", Action: domain.ActionDelete, Scope: domain.ScopeAll, MenuKey: "hr.employees"},
		{Resource: "hr.employee", Action: domain.ActionExport, Scope: domain.ScopeAll, MenuKey: "hr.employees"},
		{Resource: "hr.employee", Action: domain.ActionImport, Scope: domain.ScopeAll, MenuKey: "hr.employees"},
		{Resource: "hr.employee", Action: domain.ActionInvite, Scope: domain.ScopeAll, MenuKey: "hr.employees"},
		{Resource: "hr.employee", Action: domain.ActionUpdateStatus, Scope: domain.ScopeAll, MenuKey: "hr.employees"},
		{Resource: "hr.employee", Action: domain.ActionStatusTransition, Scope: domain.ScopeAll, MenuKey: "hr.employees"},
		{Resource: "hr.org_unit", Action: domain.ActionRead, Scope: domain.ScopeAll, MenuKey: "hr.org_units"},
		{Resource: "hr.org_unit", Action: domain.ActionCreate, Scope: domain.ScopeAll, MenuKey: "hr.org_units"},
		{Resource: "hr.org_unit", Action: domain.ActionUpdate, Scope: domain.ScopeAll, MenuKey: "hr.org_units"},
		{Resource: "attendance.leave", Action: domain.ActionRead, Scope: domain.ScopeAll, MenuKey: "attendance.leave"},
		{Resource: "attendance.leave", Action: domain.ActionCreate, Scope: domain.ScopeAll, MenuKey: "attendance.leave"},
		{Resource: "attendance.leave", Action: domain.ActionUpdate, Scope: domain.ScopeAll, MenuKey: "attendance.leave"},
		{Resource: "attendance.worksite", Action: domain.ActionRead, Scope: domain.ScopeAll, MenuKey: "attendance.worksites"},
		{Resource: "attendance.worksite", Action: domain.ActionCreate, Scope: domain.ScopeAll, MenuKey: "attendance.worksites"},
		{Resource: "attendance.worksite", Action: domain.ActionUpdate, Scope: domain.ScopeAll, MenuKey: "attendance.worksites"},
		{Resource: "attendance.shift", Action: domain.ActionRead, Scope: domain.ScopeAll, MenuKey: "attendance.shifts"},
		{Resource: "attendance.shift", Action: domain.ActionCreate, Scope: domain.ScopeAll, MenuKey: "attendance.shifts"},
		{Resource: "attendance.shift", Action: domain.ActionUpdate, Scope: domain.ScopeAll, MenuKey: "attendance.shifts"},
		{Resource: "attendance.shift_assignment", Action: domain.ActionRead, Scope: domain.ScopeAll},
		{Resource: "attendance.shift_assignment", Action: domain.ActionCreate, Scope: domain.ScopeAll},
		{Resource: "attendance.shift_assignment", Action: domain.ActionUpdate, Scope: domain.ScopeAll},
		{Resource: "attendance.clock", Action: domain.ActionRead, Scope: domain.ScopeAll, MenuKey: "attendance.clock"},
		{Resource: "attendance.clock", Action: domain.ActionCreate, Scope: domain.ScopeAll, MenuKey: "attendance.clock"},
		{Resource: "attendance.correction", Action: domain.ActionRead, Scope: domain.ScopeAll, MenuKey: "attendance.corrections"},
		{Resource: "attendance.correction", Action: domain.ActionCreate, Scope: domain.ScopeAll, MenuKey: "attendance.corrections"},
		{Resource: "attendance.correction", Action: domain.ActionApprove, Scope: domain.ScopeAll, MenuKey: "attendance.corrections"},
		{Resource: "attendance.correction", Action: domain.ActionUpdate, Scope: domain.ScopeAll, MenuKey: "attendance.corrections"},
		{Resource: "workflow.form_template", Action: domain.ActionRead, Scope: domain.ScopeAll, MenuKey: "workflow.forms"},
		{Resource: "workflow.form_template", Action: domain.ActionCreate, Scope: domain.ScopeAll, MenuKey: "workflow.forms"},
		{Resource: "workflow.form_template", Action: domain.ActionUpdate, Scope: domain.ScopeAll, MenuKey: "workflow.forms"},
		{Resource: "workflow.form_template", Action: domain.ActionDelete, Scope: domain.ScopeAll, MenuKey: "workflow.forms"},
		{Resource: "workflow.form_instance", Action: domain.ActionRead, Scope: domain.ScopeAll, MenuKey: "workflow.instances"},
		{Resource: "workflow.form_instance", Action: domain.ActionCreate, Scope: domain.ScopeAll, MenuKey: "workflow.instances"},
		{Resource: "workflow.form_instance", Action: domain.ActionSubmit, Scope: domain.ScopeAll, MenuKey: "workflow.instances"},
		{Resource: "workflow.form_instance", Action: domain.ActionApprove, Scope: domain.ScopeAll, MenuKey: "workflow.instances"},
		{Resource: "workflow.form_instance", Action: domain.ActionUpdate, Scope: domain.ScopeAll, MenuKey: "workflow.instances"},
		{Resource: "workflow.form_instance", Action: domain.ActionDelete, Scope: domain.ScopeAll, MenuKey: "workflow.instances"},
		{Resource: "workflow.form_definition_draft", Action: domain.ActionRead, Scope: domain.ScopeAll, MenuKey: "workflow.forms"},
		{Resource: "workflow.form_definition_draft", Action: domain.ActionCreate, Scope: domain.ScopeAll, MenuKey: "workflow.forms"},
		{Resource: "workflow.form_definition_draft", Action: domain.ActionUpdate, Scope: domain.ScopeAll, MenuKey: "workflow.forms"},
		{Resource: "workflow.form_definition_draft", Action: domain.ActionSubmit, Scope: domain.ScopeAll, MenuKey: "workflow.forms"},
		{Resource: "workflow.form_definition_draft", Action: domain.ActionApprove, Scope: domain.ScopeAll, MenuKey: "workflow.forms"},
		{Resource: "iam.permission", Action: domain.ActionRead, Scope: domain.ScopeAll, MenuKey: "iam.permission_sets"},
		{Resource: "iam.user_group", Action: domain.ActionRead, Scope: domain.ScopeAll, MenuKey: "iam.user_groups"},
		{Resource: "iam.user_group", Action: domain.ActionCreate, Scope: domain.ScopeAll, MenuKey: "iam.user_groups"},
		{Resource: "iam.user_group", Action: domain.ActionUpdate, Scope: domain.ScopeAll, MenuKey: "iam.user_groups"},
		{Resource: "iam.permission_set", Action: domain.ActionRead, Scope: domain.ScopeAll, MenuKey: "iam.permission_sets"},
		{Resource: "iam.permission_set", Action: domain.ActionCreate, Scope: domain.ScopeAll, MenuKey: "iam.permission_sets"},
		{Resource: "iam.permission_set", Action: domain.ActionUpdate, Scope: domain.ScopeAll, MenuKey: "iam.permission_sets"},
		{Resource: "iam.permission_set_assignment", Action: domain.ActionRead, Scope: domain.ScopeAll, MenuKey: "iam.permission_sets"},
		{Resource: "iam.permission_set_assignment", Action: domain.ActionCreate, Scope: domain.ScopeAll, MenuKey: "iam.permission_sets"},
		{Resource: "iam.permission_set_assignment", Action: domain.ActionUpdate, Scope: domain.ScopeAll, MenuKey: "iam.permission_sets"},
		{Resource: "iam.permission_set_assignment", Action: domain.ActionDelete, Scope: domain.ScopeAll, MenuKey: "iam.permission_sets"},
		{Resource: "iam.data_scope", Action: domain.ActionRead, Scope: domain.ScopeAll, MenuKey: "iam.permission_sets"},
		{Resource: "iam.data_scope", Action: domain.ActionCreate, Scope: domain.ScopeAll, MenuKey: "iam.permission_sets"},
		{Resource: "iam.field_policy", Action: domain.ActionRead, Scope: domain.ScopeAll, MenuKey: "iam.permission_sets"},
		{Resource: "iam.field_policy", Action: domain.ActionCreate, Scope: domain.ScopeAll, MenuKey: "iam.permission_sets"},
		{Resource: "iam.assumable_role", Action: domain.ActionRead, Scope: domain.ScopeAll, MenuKey: "iam.assumable_roles"},
		{Resource: "iam.assumable_role", Action: domain.ActionCreate, Scope: domain.ScopeAll, MenuKey: "iam.assumable_roles"},
		{Resource: "iam.assumable_role", Action: domain.ActionAssume, Target: "*", Scope: domain.ScopeAll, MenuKey: "iam.assumable_roles"},
		{Resource: "agent.run", Action: domain.ActionRead, Scope: domain.ScopeAll, MenuKey: "agents.runs"},
		{Resource: "agent.run", Action: domain.ActionCreate, Scope: domain.ScopeAll, MenuKey: "agents.runs"},
		{Resource: "agent.model", Action: domain.ActionRead, Scope: domain.ScopeAll, MenuKey: "agents.runs"},
		{Resource: "agent.model", Action: domain.ActionCreate, Scope: domain.ScopeAll, MenuKey: "agents.runs"},
		{Resource: "agent.model", Action: domain.ActionUpdate, Scope: domain.ScopeAll, MenuKey: "agents.runs"},
		{Resource: "agent.model", Action: domain.ActionDelete, Scope: domain.ScopeAll, MenuKey: "agents.runs"},
		{Resource: "agent.definition", Action: domain.ActionRead, Scope: domain.ScopeAll, MenuKey: "agents.runs"},
		{Resource: "agent.definition", Action: domain.ActionCreate, Scope: domain.ScopeAll, MenuKey: "agents.runs"},
		{Resource: "agent.definition", Action: domain.ActionUpdate, Scope: domain.ScopeAll, MenuKey: "agents.runs"},
		{Resource: "agent.definition", Action: domain.ActionDelete, Scope: domain.ScopeAll, MenuKey: "agents.runs"},
		{Resource: "agent.knowledge_base", Action: domain.ActionRead, Scope: domain.ScopeAll, MenuKey: "agents.knowledge_bases"},
		{Resource: "agent.knowledge_base", Action: domain.ActionCreate, Scope: domain.ScopeAll, MenuKey: "agents.knowledge_bases"},
		{Resource: "agent.knowledge_base", Action: domain.ActionUpdate, Scope: domain.ScopeAll, MenuKey: "agents.knowledge_bases"},
		{Resource: "agent.knowledge_base", Action: domain.ActionDelete, Scope: domain.ScopeAll, MenuKey: "agents.knowledge_bases"},
		{Resource: "agent.tool", Action: domain.ActionRead, Scope: domain.ScopeAll, MenuKey: "agents.tools"},
		{Resource: "audit.log", Action: domain.ActionRead, Scope: domain.ScopeAll, MenuKey: "audit"},
		{Resource: "audit.audit_log", Action: domain.ActionRead, Scope: domain.ScopeAll, MenuKey: "audit"},
	}
	for _, toolID := range defaultAgentToolIDs() {
		permissions = append(permissions, domain.Permission{Resource: "agent.tool", Action: domain.ActionCall, Target: toolID, Scope: domain.ScopeAll, MenuKey: "agents.runs"})
	}
	return permissions
}

// defaultTenantProvisionAdminName 從 email 推導預設管理員名稱。
func defaultTenantProvisionAdminName(email string) string {
	local, _, ok := strings.Cut(email, "@")
	if ok && strings.TrimSpace(local) != "" {
		return strings.TrimSpace(local)
	}
	return "Tenant Admin"
}

// safeTenantProvisionSlug 將租戶識別碼轉成穩定 ID 片段。
func safeTenantProvisionSlug(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if r > 127 {
				continue
			}
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	slug := strings.Trim(b.String(), "-")
	if slug == "" {
		slug = "tenant"
	}
	if len(slug) > 32 {
		slug = strings.Trim(slug[:32], "-")
	}
	if slug == "" {
		return "tenant"
	}
	return slug
}

// shortTenantProvisionHash 建立租戶識別碼短 hash。
func shortTenantProvisionHash(value string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(value)))
	return hex.EncodeToString(sum[:])[:10]
}
