package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"sort"
	"strings"
	"time"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/utils"
)

const (
	defaultHRPermissionPackageApplication = "hr"
	defaultHRPermissionPackageVersion     = "1.0.2"
)

var semverPattern = regexp.MustCompile(`^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$`)

// DefaultHRPermissionPackageContent 回傳內置 HR 權限包內容。
func DefaultHRPermissionPackageContent() PermissionPackageContent {
	permissions := normalizePermissions(defaultPermissions())
	resourceTypes := permissionPackageResourceTypesFromPermissions(permissions)
	actions := permissionPackageActionsFromPermissions(permissions)
	return PermissionPackageContent{
		ApplicationCode: defaultHRPermissionPackageApplication,
		Version:         defaultHRPermissionPackageVersion,
		ResourceTypes:   resourceTypes,
		Actions:         actions,
		Permissions:     permissions,
		Menus:           permissionPackageMenusFromNodes(defaultMenuCatalog),
		Buttons:         permissionPackageButtonsFromPermissions(permissions),
		Fields: []PermissionPackageField{
			{ResourceType: "employee", FieldName: "basic_info.national_id", Effect: string(domain.FieldPolicyEffectMask), Description: "Mask sensitive employee identity fields by default."},
		},
		DataScopes: []PermissionPackageDataScope{
			{Code: "hr_all", Name: "HR 全部資料", ScopeType: string(ScopeAll)},
			{Code: "hr_self", Name: "HR 本人資料", ScopeType: string(ScopeSelf)},
		},
		PermissionSetTemplates: []PermissionSetTemplateContent{
			{
				TemplateKey: "hr_employee_base",
				Name:        "員工基礎權限",
				Description: "員工自助入口與本人資料讀取。",
				Permissions: []Permission{
					{Resource: "me", Action: ActionRead, Scope: ScopeSelf, MenuKey: "workbench"},
					{Resource: "me", Action: ActionUpdate, Scope: ScopeSelf, MenuKey: "workbench"},
					{Resource: "hr.employee", Action: ActionRead, Scope: ScopeSelf, MenuKey: "hr.employees"},
					{Resource: "attendance.clock", Action: ActionCreate, Scope: ScopeSelf, MenuKey: "attendance.clock"},
					{Resource: "attendance.leave", Action: ActionRead, Scope: ScopeSelf, MenuKey: "attendance.leave"},
					{Resource: "attendance.leave", Action: ActionCreate, Scope: ScopeSelf, MenuKey: "attendance.leave"},
					{Resource: "workflow.form_template", Action: ActionRead, Scope: ScopeAll, MenuKey: "workflow.forms"},
					{Resource: "workflow.form_instance", Action: ActionRead, Scope: ScopeSelf, MenuKey: "workflow.instances"},
					{Resource: "workflow.form_instance", Action: ActionSubmit, Scope: ScopeSelf, MenuKey: "workflow.instances"},
					{Resource: "workflow.form_instance", Action: ActionUpdate, Scope: ScopeSelf, MenuKey: "workflow.instances"},
					{Resource: "agent.run", Action: ActionRead, Scope: ScopeSelf, MenuKey: "agents.runs"},
					{Resource: "agent.run", Action: ActionCreate, Scope: ScopeSelf, MenuKey: "agents.runs"},
					agentToolPermission("knowledge.search"),
					agentToolPermission("get_my_profile"),
					agentToolPermission("my_leave_balances"),
					agentToolPermission("check_leave_eligibility"),
					agentToolPermission("my_clock_records"),
					agentToolPermission("my_attendance_summary"),
					agentToolPermission("my_form_history"),
					agentToolPermission("my_pending_reviews"),
					agentToolPermission("list_published_form_templates"),
					agentToolPermission("get_published_form_template"),
					agentToolPermission("create_form_draft"),
					agentToolPermission("update_form_draft"),
					agentToolPermission("preview_form_submission"),
					agentToolPermission("prepare_bulk_review"),
				},
			},
			{
				TemplateKey: "hr_management",
				Name:        "HR 管理權限",
				Description: "HR 主資料、組織、假勤與權限管理常用操作。",
				Permissions: []Permission{
					{Resource: "hr.employee", Action: ActionRead, Scope: ScopeAll, MenuKey: "hr.employees"},
					{Resource: "hr.employee", Action: ActionCreate, Scope: ScopeAll, MenuKey: "hr.employees"},
					{Resource: "hr.employee", Action: ActionUpdate, Scope: ScopeAll, MenuKey: "hr.employees"},
					{Resource: "hr.employee", Action: ActionExport, Scope: ScopeAll, MenuKey: "hr.employees"},
					{Resource: "hr.employee", Action: ActionImport, Scope: ScopeAll, MenuKey: "hr.employees"},
					{Resource: "hr.org_unit", Action: ActionRead, Scope: ScopeAll, MenuKey: "hr.org_units"},
					{Resource: "hr.org_unit", Action: ActionCreate, Scope: ScopeAll, MenuKey: "hr.org_units"},
					{Resource: "hr.org_unit", Action: ActionUpdate, Scope: ScopeAll, MenuKey: "hr.org_units"},
					{Resource: "attendance.leave", Action: ActionRead, Scope: ScopeAll, MenuKey: "attendance.leave"},
					{Resource: "attendance.clock", Action: ActionRead, Scope: ScopeAll, MenuKey: "attendance.clock"},
					{Resource: "attendance.correction", Action: ActionApprove, Scope: ScopeAll, MenuKey: "attendance.corrections"},
					{Resource: "workflow.form_template", Action: ActionRead, Scope: ScopeAll, MenuKey: "workflow.forms"},
					{Resource: "workflow.form_instance", Action: ActionRead, Scope: ScopeAll, MenuKey: "workflow.instances"},
					{Resource: "workflow.form_instance", Action: ActionSubmit, Scope: ScopeSelf, MenuKey: "workflow.instances"},
					{Resource: "workflow.form_instance", Action: ActionUpdate, Scope: ScopeAll, MenuKey: "workflow.instances"},
					{Resource: "workflow.form_instance", Action: ActionApprove, Scope: ScopeAll, MenuKey: "workflow.instances"},
					{Resource: "workflow.form_definition_draft", Action: ActionRead, Scope: ScopeAll, MenuKey: "workflow.forms"},
					{Resource: "workflow.form_definition_draft", Action: ActionCreate, Scope: ScopeAll, MenuKey: "workflow.forms"},
					{Resource: "workflow.form_definition_draft", Action: ActionUpdate, Scope: ScopeAll, MenuKey: "workflow.forms"},
					{Resource: "workflow.form_definition_draft", Action: ActionSubmit, Scope: ScopeAll, MenuKey: "workflow.forms"},
					{Resource: "workflow.form_definition_draft", Action: ActionApprove, Scope: ScopeAll, MenuKey: "workflow.forms"},
					{Resource: "iam.permission_set", Action: ActionRead, Scope: ScopeAll, MenuKey: "iam.permission_sets"},
					{Resource: "iam.permission_set_assignment", Action: ActionRead, Scope: ScopeAll, MenuKey: "iam.permission_sets"},
					{Resource: "agent.run", Action: ActionRead, Scope: ScopeAll, MenuKey: "agents.runs"},
					{Resource: "agent.run", Action: ActionCreate, Scope: ScopeAll, MenuKey: "agents.runs"},
					{Resource: "agent.model", Action: ActionRead, Scope: ScopeAll, MenuKey: "agents.runs"},
					{Resource: "agent.model", Action: ActionCreate, Scope: ScopeAll, MenuKey: "agents.runs"},
					{Resource: "agent.model", Action: ActionUpdate, Scope: ScopeAll, MenuKey: "agents.runs"},
					{Resource: "agent.model", Action: ActionDelete, Scope: ScopeAll, MenuKey: "agents.runs"},
					{Resource: "agent.definition", Action: ActionRead, Scope: ScopeAll, MenuKey: "agents.runs"},
					{Resource: "agent.definition", Action: ActionCreate, Scope: ScopeAll, MenuKey: "agents.runs"},
					{Resource: "agent.definition", Action: ActionUpdate, Scope: ScopeAll, MenuKey: "agents.runs"},
					{Resource: "agent.definition", Action: ActionDelete, Scope: ScopeAll, MenuKey: "agents.runs"},
					{Resource: "agent.usage", Action: ActionRead, Scope: ScopeAll, MenuKey: "agents.usage"},
					{Resource: "agent.knowledge_base", Action: ActionRead, Scope: ScopeAll, MenuKey: "agents.knowledge_bases"},
					{Resource: "agent.knowledge_base", Action: ActionCreate, Scope: ScopeAll, MenuKey: "agents.knowledge_bases"},
					{Resource: "agent.knowledge_base", Action: ActionUpdate, Scope: ScopeAll, MenuKey: "agents.knowledge_bases"},
					{Resource: "agent.knowledge_base", Action: ActionDelete, Scope: ScopeAll, MenuKey: "agents.knowledge_bases"},
					{Resource: "agent.tool", Action: ActionRead, Scope: ScopeAll, MenuKey: "agents.tools"},
					agentToolPermission("knowledge.search"),
					agentToolPermission("get_my_profile"),
					agentToolPermission("list_employees"),
					agentToolPermission("get_employee"),
					agentToolPermission("my_leave_balances"),
					agentToolPermission("check_leave_eligibility"),
					agentToolPermission("my_clock_records"),
					agentToolPermission("my_attendance_summary"),
					agentToolPermission("my_form_history"),
					agentToolPermission("my_pending_reviews"),
					agentToolPermission("workspace_insights"),
					agentToolPermission("list_published_form_templates"),
					agentToolPermission("get_published_form_template"),
					agentToolPermission("create_form_draft"),
					agentToolPermission("update_form_draft"),
					agentToolPermission("preview_form_submission"),
					agentToolPermission("prepare_bulk_review"),
					agentToolPermission("form.get_capabilities"),
					agentToolPermission("form.get_data_source_schema"),
					agentToolPermission("form.create_draft"),
					agentToolPermission("form.update_draft"),
					agentToolPermission("form.validate_draft"),
					agentToolPermission("form.preview_draft"),
					agentToolPermission("form.simulate_workflow"),
				},
			},
			{
				TemplateKey: "platform_readonly_troubleshooting",
				Name:        "平臺唯讀排障權限",
				Description: "平臺與審計唯讀排障用權限集合。",
				Permissions: []Permission{
					{Resource: "me", Action: ActionRead, Scope: ScopeAll, MenuKey: "workbench"},
					{Resource: "audit.log", Action: ActionRead, Scope: ScopeAll, MenuKey: "audit"},
					{Resource: "iam.permission", Action: ActionRead, Scope: ScopeAll, MenuKey: "iam.permission_sets"},
					{Resource: "iam.permission_set", Action: ActionRead, Scope: ScopeAll, MenuKey: "iam.permission_sets"},
				},
			},
		},
		UserGroupTemplates: []UserGroupTemplateContent{
			{
				TemplateKey:               "employees",
				Name:                      "員工基礎組",
				Description:               "所有員工的基礎自助權限組。",
				PermissionSetTemplateKeys: []string{"hr_employee_base"},
			},
			{
				TemplateKey:               "hr_managers",
				Name:                      "HR 管理組",
				Description:               "HR 管理人員常用權限組。",
				PermissionSetTemplateKeys: []string{"hr_management"},
			},
		},
		AssumableRoleTemplates: []AssumableRoleTemplateContent{
			{
				TemplateKey:               "platform_readonly_troubleshooter",
				Name:                      "平臺只讀排障",
				Description:               "短時承擔的唯讀排障角色。",
				PermissionSetTemplateKeys: []string{"platform_readonly_troubleshooting"},
				Trusted:                   true,
				TrustPolicy:               map[string]any{"purpose": "troubleshooting"},
				PermissionBoundary:        map[string]any{"effect": "allow_readonly"},
				SessionDurationSeconds:    3600,
			},
		},
		FGAMappings: []PermissionPackageFGAMapping{
			{ResourceType: "employee", OpenFGAType: "employee"},
			{ResourceType: "org_unit", OpenFGAType: "org_unit"},
			{ResourceType: "permission_set_assignment", OpenFGAType: "permission_set_assignment"},
			{ResourceType: "assumable_role", OpenFGAType: "assumable_role"},
		},
	}
}

func agentToolPermission(toolID string) Permission {
	return Permission{Resource: "agent.tool", Action: ActionCall, Target: toolID, Scope: ScopeAll, MenuKey: "agents.runs"}
}

// DefaultHRPermissionPackage 回傳內置 HR 權限包快照。
func DefaultHRPermissionPackage(now time.Time) (PermissionPackage, error) {
	content := DefaultHRPermissionPackageContent()
	content = normalizePermissionPackageContent(content)
	checksum, err := PermissionPackageChecksum(content)
	if err != nil {
		return PermissionPackage{}, err
	}
	publishedAt := now.UTC()
	return PermissionPackage{
		ID:              stablePermissionPackageID(content.ApplicationCode, content.Version),
		ApplicationCode: content.ApplicationCode,
		Version:         content.Version,
		Status:          PermissionPackageStatusPublished,
		Content:         content,
		Checksum:        checksum,
		CreatedAt:       publishedAt,
		PublishedAt:     &publishedAt,
	}, nil
}

// ValidatePermissionPackageContent 驗證權限包 content。
func ValidatePermissionPackageContent(content PermissionPackageContent) error {
	content = normalizePermissionPackageContent(content)
	fields := make([]domain.FieldError, 0)
	addField := func(field, code, message string) {
		fields = append(fields, domain.FieldError{Field: field, Code: code, Message: message})
	}
	if content.ApplicationCode == "" {
		addField("application_code", "required", "application_code is required")
	}
	if content.Version == "" {
		addField("version", "required", "version is required")
	} else if !semverPattern.MatchString(content.Version) {
		addField("version", "invalid", "version must be semver")
	}
	if len(content.ResourceTypes) == 0 {
		addField("resource_types", "required", "resource_types are required")
	}
	if len(content.Actions) == 0 {
		addField("actions", "required", "actions are required")
	}
	if len(content.Permissions) == 0 {
		addField("permissions", "required", "permissions are required")
	}
	validatePackageResourceTypes(content.ResourceTypes, addField)
	validatePackageActions(content.Actions, addField)
	validatePackagePermissions(content.Permissions, addField)
	validatePackageMenus(content.Menus, addField)
	validatePackageButtons(content.Buttons, addField)
	validatePackageFields(content.Fields, addField)
	validatePackageDataScopes(content.DataScopes, addField)
	validatePackageTemplates(content, addField)
	validatePackageFGAMappings(content.FGAMappings, addField)
	if len(fields) > 0 {
		return domain.ValidationFailed("permission package schema validation failed", fields)
	}
	return nil
}

func permissionPackageValidationError(err error) error {
	if appErr, ok := domain.AsAppError(err); ok {
		return appErr.WithPublicCode(domain.ErrorCodePermissionPackageInvalid)
	}
	return err
}

// PermissionPackageChecksum 計算權限包 content checksum。
func PermissionPackageChecksum(content PermissionPackageContent) (string, error) {
	content = normalizePermissionPackageContent(content)
	payload, err := json.Marshal(content)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

// ListPermissionPackages 列出權限包。
func (c IAMService) ListPermissionPackages(ctx RequestContext) ([]PermissionPackage, error) {
	if _, _, err := c.requireIAMAuthz(ctx, ResourcePermissionPackage, ActionRead, ""); err != nil {
		return nil, err
	}
	if err := c.ensureBuiltinPermissionPackage(goContext(ctx)); err != nil {
		return nil, err
	}
	return c.store.ListPermissionPackages(goContext(ctx))
}

// ListPermissionPackagePage 列出權限包分頁。
func (c IAMService) ListPermissionPackagePage(ctx RequestContext, page PageRequest) (PageResponse[PermissionPackage], error) {
	items, err := c.ListPermissionPackages(ctx)
	if err != nil {
		return PageResponse[PermissionPackage]{}, err
	}
	return utils.PageResponse(items, page), nil
}

// RegisterPermissionPackage 註冊 draft 權限包。
func (c IAMService) RegisterPermissionPackage(ctx RequestContext, content PermissionPackageContent) (PermissionPackage, error) {
	if _, _, err := c.requireIAMAuthz(ctx, ResourcePermissionPackage, ActionCreate, ""); err != nil {
		return PermissionPackage{}, err
	}
	if err := c.requireGlobalPermissionPackageWrite(ctx); err != nil {
		return PermissionPackage{}, err
	}
	content = normalizePermissionPackageContent(content)
	if err := ValidatePermissionPackageContent(content); err != nil {
		return PermissionPackage{}, permissionPackageValidationError(err)
	}
	if _, ok, err := c.store.GetPermissionPackageByApplicationVersion(goContext(ctx), content.ApplicationCode, content.Version); err != nil {
		return PermissionPackage{}, err
	} else if ok {
		return PermissionPackage{}, Conflict("permission package version already exists").WithPublicCode(domain.ErrorCodePermissionPackageVersionConflict)
	}
	checksum, err := PermissionPackageChecksum(content)
	if err != nil {
		return PermissionPackage{}, err
	}
	pkg := PermissionPackage{
		ID:              stablePermissionPackageID(content.ApplicationCode, content.Version),
		ApplicationCode: content.ApplicationCode,
		Version:         content.Version,
		Status:          PermissionPackageStatusDraft,
		Content:         content,
		Checksum:        checksum,
		CreatedAt:       c.Now(),
	}
	if err := c.withTransaction(ctx, func(tx IAMService) error {
		if err := tx.storePermissionPackageWithTemplates(goContext(ctx), pkg); err != nil {
			return err
		}
		return tx.audit(ctx, "iam.permission_package.register", "permission_package", pkg.ID, "high", map[string]any{
			"application_code": pkg.ApplicationCode,
			"version":          pkg.Version,
			"checksum":         pkg.Checksum,
		})
	}); err != nil {
		return PermissionPackage{}, err
	}
	return pkg, nil
}

// PublishPermissionPackage 發布權限包。
func (c IAMService) PublishPermissionPackage(ctx RequestContext, id string) (PermissionPackage, error) {
	if _, _, err := c.requireIAMAuthz(ctx, ResourcePermissionPackage, Action("publish"), id); err != nil {
		return PermissionPackage{}, err
	}
	if err := c.requireGlobalPermissionPackageWrite(ctx); err != nil {
		return PermissionPackage{}, err
	}
	id = strings.TrimSpace(id)
	pkg, ok, err := c.store.GetPermissionPackage(goContext(ctx), id)
	if err != nil {
		return PermissionPackage{}, err
	}
	if !ok {
		return PermissionPackage{}, NotFound("permission package", id)
	}
	if pkg.Status == PermissionPackageStatusPublished {
		return pkg, nil
	}
	if pkg.Status == PermissionPackageStatusDeprecated {
		return PermissionPackage{}, Conflict("deprecated permission package cannot be published").WithPublicCode(domain.ErrorCodePermissionPackageStateConflict)
	}
	if err := ValidatePermissionPackageContent(pkg.Content); err != nil {
		return PermissionPackage{}, permissionPackageValidationError(err)
	}
	now := c.Now()
	if err := c.withTransaction(ctx, func(tx IAMService) error {
		published, ok, err := tx.store.UpdatePermissionPackageStatus(goContext(ctx), id, PermissionPackageStatusPublished, &now)
		if err != nil {
			return err
		}
		if !ok {
			return NotFound("permission package", id)
		}
		pkg = published
		return tx.audit(ctx, "iam.permission_package.publish", "permission_package", pkg.ID, "high", map[string]any{
			"application_code": pkg.ApplicationCode,
			"version":          pkg.Version,
			"checksum":         pkg.Checksum,
		})
	}); err != nil {
		return PermissionPackage{}, err
	}
	return pkg, nil
}

// requireGlobalPermissionPackageWrite 僅接受由已驗證 token 推導出的平臺管理員身分。
func (c IAMService) requireGlobalPermissionPackageWrite(ctx RequestContext) error {
	if !ctx.PlatformAdmin {
		return Forbidden("global permission package registry write is not authorized")
	}
	return nil
}

// ImportPermissionPackage 將已發布權限包導入當前租戶。
func (c IAMService) ImportPermissionPackage(ctx RequestContext, id string) (PermissionPackageImportResult, error) {
	if _, _, err := c.requireIAMAuthz(ctx, ResourcePermissionPackage, ActionImport, id); err != nil {
		return PermissionPackageImportResult{}, err
	}
	id = strings.TrimSpace(id)
	pkg, ok, err := c.store.GetPermissionPackage(goContext(ctx), id)
	if err != nil {
		return PermissionPackageImportResult{}, err
	}
	if !ok {
		return PermissionPackageImportResult{}, NotFound("permission package", id)
	}
	if pkg.Status != PermissionPackageStatusPublished {
		return PermissionPackageImportResult{}, Conflict("permission package must be published before import").WithPublicCode(domain.ErrorCodePermissionPackageStateConflict)
	}
	if err := ValidatePermissionPackageContent(pkg.Content); err != nil {
		return PermissionPackageImportResult{}, permissionPackageValidationError(err)
	}
	if existing, ok, err := c.store.GetPermissionPackageImport(goContext(ctx), ctx.TenantID, pkg.ID, pkg.Version); err != nil {
		return PermissionPackageImportResult{}, err
	} else if ok {
		return PermissionPackageImportResult{
			Import:        existing,
			Package:       pkg,
			ArtifactIDMap: utils.CopyStringMap(existing.ArtifactIDMap),
			Diff:          c.permissionPackageUpgradeDiff(ctx, pkg),
			Imported:      false,
		}, nil
	}

	now := c.Now()
	result := PermissionPackageImportResult{Package: pkg, Imported: true}
	if err := c.withTransaction(ctx, func(tx IAMService) error {
		if err := syncPermissionCatalogFromPackageForTenant(goContext(ctx), tx.store, ctx.TenantID, pkg.Content, now); err != nil {
			return err
		}
		artifactMap, err := tx.instantiatePermissionPackageTemplates(ctx, pkg)
		if err != nil {
			return err
		}
		record := PermissionPackageImport{
			ID:            stablePermissionPackageImportID(ctx.TenantID, pkg.ID, pkg.Version),
			TenantID:      ctx.TenantID,
			PackageID:     pkg.ID,
			Version:       pkg.Version,
			ImportedAt:    now,
			ImportedBy:    ctx.AccountID,
			ArtifactIDMap: artifactMap,
		}
		if err := tx.store.UpsertPermissionPackageImport(goContext(ctx), record); err != nil {
			return err
		}
		if err := tx.touchAuthzConfig(ctx, "iam.permission_package.import", map[string]any{
			"permission_package_id": pkg.ID,
			"application_code":      pkg.ApplicationCode,
			"version":               pkg.Version,
		}); err != nil {
			return err
		}
		if err := tx.audit(ctx, "iam.permission_package.import", "permission_package", pkg.ID, "high", map[string]any{
			"application_code": pkg.ApplicationCode,
			"version":          pkg.Version,
			"artifact_id_map":  artifactMap,
		}); err != nil {
			return err
		}
		result.Import = record
		result.ArtifactIDMap = artifactMap
		result.Diff = tx.permissionPackageUpgradeDiff(ctx, pkg)
		return nil
	}); err != nil {
		return PermissionPackageImportResult{}, err
	}
	return result, nil
}

func (c IAMService) storePermissionPackageWithTemplates(ctx context.Context, pkg PermissionPackage) error {
	if err := c.store.UpsertPermissionPackage(ctx, pkg); err != nil {
		return err
	}
	for _, tpl := range permissionSetTemplatesFromPackage(pkg) {
		if err := c.store.UpsertPermissionSetTemplate(ctx, tpl); err != nil {
			return err
		}
	}
	for _, tpl := range userGroupTemplatesFromPackage(pkg) {
		if err := c.store.UpsertUserGroupTemplate(ctx, tpl); err != nil {
			return err
		}
	}
	for _, tpl := range assumableRoleTemplatesFromPackage(pkg) {
		if err := c.store.UpsertAssumableRoleTemplate(ctx, tpl); err != nil {
			return err
		}
	}
	return nil
}

func (c *Service) ensureBuiltinPermissionPackage(ctx context.Context) error {
	pkg, err := DefaultHRPermissionPackage(c.Now())
	if err != nil {
		return err
	}
	if _, ok, err := c.store.GetPermissionPackage(ctx, pkg.ID); err != nil {
		return err
	} else if ok {
		return nil
	}
	iam := c.IAM()
	return iam.storePermissionPackageWithTemplates(ctx, pkg)
}

// permissionPackageArtifactMaps 以 typed 欄位收集模板實例化產生的 ID 對映，避免對內層 map 裸斷言。
type permissionPackageArtifactMaps struct {
	permissionSetTemplates map[string]any
	userGroupTemplates     map[string]any
	assumableRoleTemplates map[string]any
	dataScopes             map[string]any
}

func newPermissionPackageArtifactMaps() permissionPackageArtifactMaps {
	return permissionPackageArtifactMaps{
		permissionSetTemplates: map[string]any{},
		userGroupTemplates:     map[string]any{},
		assumableRoleTemplates: map[string]any{},
		dataScopes:             map[string]any{},
	}
}

// toArtifactMap 轉為既有的儲存/線上格式。
func (m permissionPackageArtifactMaps) toArtifactMap() map[string]any {
	return map[string]any{
		"permission_set_templates": m.permissionSetTemplates,
		"user_group_templates":     m.userGroupTemplates,
		"assumable_role_templates": m.assumableRoleTemplates,
		"data_scopes":              m.dataScopes,
	}
}

func (c IAMService) instantiatePermissionPackageTemplates(ctx RequestContext, pkg PermissionPackage) (map[string]any, error) {
	artifacts := newPermissionPackageArtifactMaps()
	permissionSetIDs := map[string]string{}
	for _, scope := range pkg.Content.DataScopes {
		id, err := c.ensurePackageDataScope(ctx, pkg, scope)
		if err != nil {
			return nil, err
		}
		if id != "" {
			artifacts.dataScopes[scope.Code] = id
		}
	}
	for _, tpl := range pkg.Content.PermissionSetTemplates {
		set, err := c.ensurePackagePermissionSet(ctx, pkg, tpl)
		if err != nil {
			return nil, err
		}
		permissionSetIDs[tpl.TemplateKey] = set.ID
		artifacts.permissionSetTemplates[tpl.TemplateKey] = set.ID
	}
	for _, tpl := range pkg.Content.UserGroupTemplates {
		group, err := c.ensurePackageUserGroup(ctx, pkg, tpl, permissionSetIDs)
		if err != nil {
			return nil, err
		}
		artifacts.userGroupTemplates[tpl.TemplateKey] = group.ID
	}
	for _, tpl := range pkg.Content.AssumableRoleTemplates {
		role, err := c.ensurePackageAssumableRole(ctx, pkg, tpl, permissionSetIDs)
		if err != nil {
			return nil, err
		}
		artifacts.assumableRoleTemplates[tpl.TemplateKey] = role.ID
	}
	return artifacts.toArtifactMap(), nil
}

func (c IAMService) ensurePackageDataScope(ctx RequestContext, pkg PermissionPackage, tpl PermissionPackageDataScope) (string, error) {
	code := strings.TrimSpace(tpl.Code)
	if code == "" {
		return "", nil
	}
	existing, err := c.store.ListDataScopes(goContext(ctx), ctx.TenantID)
	if err != nil {
		return "", err
	}
	expectedID := stablePermissionTemplateID("ds", ctx.TenantID, pkg.ApplicationCode, tpl.Code)
	for _, item := range existing {
		if item.Code == code {
			if item.ID != expectedID {
				return item.ID, nil
			}
			next := item
			next.Name = strings.TrimSpace(tpl.Name)
			next.ScopeType = strings.TrimSpace(tpl.ScopeType)
			next.Params = utils.CopyStringMap(tpl.Params)
			if next.Name == "" {
				next.Name = code
			}
			if next.ScopeType == "" {
				next.ScopeType = code
			}
			if err := c.store.UpsertDataScope(goContext(ctx), next); err != nil {
				return "", err
			}
			return item.ID, nil
		}
	}
	scope := DataScope{
		ID:        expectedID,
		TenantID:  ctx.TenantID,
		Code:      code,
		Name:      strings.TrimSpace(tpl.Name),
		ScopeType: strings.TrimSpace(tpl.ScopeType),
		Params:    utils.CopyStringMap(tpl.Params),
		CreatedAt: c.Now(),
	}
	if scope.Name == "" {
		scope.Name = scope.Code
	}
	if scope.ScopeType == "" {
		scope.ScopeType = scope.Code
	}
	if err := c.store.UpsertDataScope(goContext(ctx), scope); err != nil {
		return "", err
	}
	return scope.ID, nil
}

func (c IAMService) ensurePackagePermissionSet(ctx RequestContext, pkg PermissionPackage, tpl PermissionSetTemplateContent) (PermissionSet, error) {
	existing, err := c.findPermissionSetByTemplate(ctx, pkg, tpl.TemplateKey)
	if err != nil {
		return existing, err
	}
	if existing.ID != "" {
		if existing.SourceTemplateKey != tpl.TemplateKey {
			return existing, nil
		}
		existing.Name = strings.TrimSpace(tpl.Name)
		existing.Description = strings.TrimSpace(tpl.Description)
		existing.Permissions = normalizePermissions(tpl.Permissions)
		existing.SourcePackageVersion = pkg.Version
		if existing.Name == "" {
			existing.Name = tpl.TemplateKey
		}
		if _, err := c.upsertPermissionSetWithItems(ctx, existing); err != nil {
			return PermissionSet{}, err
		}
		return existing, nil
	}
	set := PermissionSet{
		ID:                   stablePermissionTemplateID("ps", ctx.TenantID, pkg.ApplicationCode, tpl.TemplateKey),
		TenantID:             ctx.TenantID,
		Name:                 strings.TrimSpace(tpl.Name),
		Description:          strings.TrimSpace(tpl.Description),
		Permissions:          normalizePermissions(tpl.Permissions),
		SourceTemplateKey:    tpl.TemplateKey,
		SourcePackageVersion: pkg.Version,
		CreatedAt:            c.Now(),
	}
	if set.Name == "" {
		set.Name = tpl.TemplateKey
	}
	if _, err := c.upsertPermissionSetWithItems(ctx, set); err != nil {
		return PermissionSet{}, err
	}
	return set, nil
}

func (c IAMService) findPermissionSetByTemplate(ctx RequestContext, pkg PermissionPackage, templateKey string) (PermissionSet, error) {
	id := stablePermissionTemplateID("ps", ctx.TenantID, pkg.ApplicationCode, templateKey)
	if existing, ok, err := c.store.GetPermissionSet(goContext(ctx), ctx.TenantID, id); err != nil {
		return PermissionSet{}, err
	} else if ok {
		return existing, nil
	}
	sets, err := c.store.ListPermissionSets(goContext(ctx), ctx.TenantID)
	if err != nil {
		return PermissionSet{}, err
	}
	for _, item := range sets {
		if item.SourceTemplateKey == templateKey {
			return item, nil
		}
	}
	return PermissionSet{}, nil
}

func (c IAMService) ensurePackageUserGroup(ctx RequestContext, pkg PermissionPackage, tpl UserGroupTemplateContent, permissionSetIDs map[string]string) (UserGroup, error) {
	existing, err := c.findUserGroupByTemplate(ctx, pkg, tpl.TemplateKey)
	if err != nil {
		return existing, err
	}
	setIDs, err := permissionSetIDsForTemplateKeys(tpl.PermissionSetTemplateKeys, permissionSetIDs)
	if err != nil {
		return UserGroup{}, err
	}
	if existing.ID != "" {
		if existing.SourceTemplateKey != tpl.TemplateKey {
			return existing, nil
		}
		before := existing
		existing.Name = strings.TrimSpace(tpl.Name)
		existing.Description = strings.TrimSpace(tpl.Description)
		existing.PermissionSetIDs = setIDs
		existing.SourcePackageVersion = pkg.Version
		if existing.Name == "" {
			existing.Name = tpl.TemplateKey
		}
		if err := c.store.UpsertUserGroup(goContext(ctx), existing); err != nil {
			return UserGroup{}, err
		}
		if err := c.Service.syncUserGroupRelationshipTuples(ctx, before, existing); err != nil {
			return UserGroup{}, err
		}
		return existing, nil
	}
	group := UserGroup{
		ID:                   stablePermissionTemplateID("ug", ctx.TenantID, pkg.ApplicationCode, tpl.TemplateKey),
		TenantID:             ctx.TenantID,
		Name:                 strings.TrimSpace(tpl.Name),
		Description:          strings.TrimSpace(tpl.Description),
		PermissionSetIDs:     setIDs,
		SourceTemplateKey:    tpl.TemplateKey,
		SourcePackageVersion: pkg.Version,
		CreatedAt:            c.Now(),
	}
	if group.Name == "" {
		group.Name = tpl.TemplateKey
	}
	if err := c.store.UpsertUserGroup(goContext(ctx), group); err != nil {
		return UserGroup{}, err
	}
	if err := c.Service.syncUserGroupRelationshipTuples(ctx, UserGroup{}, group); err != nil {
		return UserGroup{}, err
	}
	return group, nil
}

func (c IAMService) findUserGroupByTemplate(ctx RequestContext, pkg PermissionPackage, templateKey string) (UserGroup, error) {
	id := stablePermissionTemplateID("ug", ctx.TenantID, pkg.ApplicationCode, templateKey)
	if existing, ok, err := c.store.GetUserGroup(goContext(ctx), ctx.TenantID, id); err != nil {
		return UserGroup{}, err
	} else if ok {
		return existing, nil
	}
	groups, err := c.store.ListUserGroups(goContext(ctx), ctx.TenantID)
	if err != nil {
		return UserGroup{}, err
	}
	for _, item := range groups {
		if item.SourceTemplateKey == templateKey {
			return item, nil
		}
	}
	return UserGroup{}, nil
}

func (c IAMService) ensurePackageAssumableRole(ctx RequestContext, pkg PermissionPackage, tpl AssumableRoleTemplateContent, permissionSetIDs map[string]string) (AssumableRole, error) {
	existing, err := c.findAssumableRoleByTemplate(ctx, pkg, tpl.TemplateKey)
	if err != nil {
		return existing, err
	}
	setIDs, err := permissionSetIDsForTemplateKeys(tpl.PermissionSetTemplateKeys, permissionSetIDs)
	if err != nil {
		return AssumableRole{}, err
	}
	duration := tpl.SessionDurationSeconds
	if duration <= 0 {
		duration = defaultAssumableRoleSessionSeconds
	}
	if existing.ID != "" {
		if existing.SourceTemplateKey != tpl.TemplateKey {
			return existing, nil
		}
		before := existing
		existing.Name = strings.TrimSpace(tpl.Name)
		existing.Description = strings.TrimSpace(tpl.Description)
		existing.PermissionSetIDs = setIDs
		existing.Trusted = tpl.Trusted
		existing.TrustPolicy = utils.CopyStringMap(tpl.TrustPolicy)
		existing.PermissionBoundary = utils.CopyStringMap(tpl.PermissionBoundary)
		existing.SessionDurationSeconds = duration
		existing.SourcePackageVersion = pkg.Version
		if existing.Name == "" {
			existing.Name = tpl.TemplateKey
		}
		if err := c.store.UpsertAssumableRole(goContext(ctx), existing); err != nil {
			return AssumableRole{}, err
		}
		if err := c.Service.syncAssumableRoleRelationshipTuples(ctx, before, existing); err != nil {
			return AssumableRole{}, err
		}
		if err := c.revokePackageRoleSessions(ctx, existing.ID); err != nil {
			return AssumableRole{}, err
		}
		return existing, nil
	}
	role := AssumableRole{
		ID:                     stablePermissionTemplateID("ar", ctx.TenantID, pkg.ApplicationCode, tpl.TemplateKey),
		TenantID:               ctx.TenantID,
		Name:                   strings.TrimSpace(tpl.Name),
		Description:            strings.TrimSpace(tpl.Description),
		PermissionSetIDs:       setIDs,
		Trusted:                tpl.Trusted,
		TrustPolicy:            utils.CopyStringMap(tpl.TrustPolicy),
		PermissionBoundary:     utils.CopyStringMap(tpl.PermissionBoundary),
		SessionDurationSeconds: duration,
		SourceTemplateKey:      tpl.TemplateKey,
		SourcePackageVersion:   pkg.Version,
		CreatedAt:              c.Now(),
	}
	if role.Name == "" {
		role.Name = tpl.TemplateKey
	}
	if err := c.store.UpsertAssumableRole(goContext(ctx), role); err != nil {
		return AssumableRole{}, err
	}
	return role, nil
}

// revokePackageRoleSessions 讓權限包角色的收斂變更立即終止舊 session。
func (c IAMService) revokePackageRoleSessions(ctx RequestContext, roleID string) error {
	sessions, err := c.store.ListActiveAssumableRoleSessionsForRole(goContext(ctx), ctx.TenantID, roleID)
	if err != nil {
		return err
	}
	now := c.Now()
	for _, session := range sessions {
		session.RevokedAt = &now
		if err := c.store.UpsertAssumableRoleSession(goContext(ctx), session); err != nil {
			return err
		}
	}
	return nil
}

func (c IAMService) findAssumableRoleByTemplate(ctx RequestContext, pkg PermissionPackage, templateKey string) (AssumableRole, error) {
	id := stablePermissionTemplateID("ar", ctx.TenantID, pkg.ApplicationCode, templateKey)
	if existing, ok, err := c.store.GetAssumableRole(goContext(ctx), ctx.TenantID, id); err != nil {
		return AssumableRole{}, err
	} else if ok {
		return existing, nil
	}
	roles, err := c.store.ListAssumableRoles(goContext(ctx), ctx.TenantID)
	if err != nil {
		return AssumableRole{}, err
	}
	for _, item := range roles {
		if item.SourceTemplateKey == templateKey {
			return item, nil
		}
	}
	return AssumableRole{}, nil
}

func permissionSetIDsForTemplateKeys(keys []string, idByTemplate map[string]string) ([]string, error) {
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		key = strings.TrimSpace(key)
		id := strings.TrimSpace(idByTemplate[key])
		if id == "" {
			return nil, BadRequest("permission set template not instantiated: " + key)
		}
		out = append(out, id)
	}
	return uniqueStrings(out), nil
}

func (c IAMService) permissionPackageUpgradeDiff(ctx RequestContext, pkg PermissionPackage) PermissionPackageImportDiff {
	imports, err := c.store.ListPermissionPackageImports(goContext(ctx), ctx.TenantID)
	if err != nil {
		return PermissionPackageImportDiff{}
	}
	var previous *PermissionPackage
	for _, item := range imports {
		if item.PackageID == pkg.ID && item.Version == pkg.Version {
			continue
		}
		prev, ok, err := c.store.GetPermissionPackage(goContext(ctx), item.PackageID)
		if err != nil || !ok || prev.ApplicationCode != pkg.ApplicationCode {
			continue
		}
		if previous == nil || item.ImportedAt.After(previousImportTime(imports, previous.ID, previous.Version)) {
			copyPrev := prev
			previous = &copyPrev
		}
	}
	if previous == nil {
		return PermissionPackageImportDiff{AddedTemplates: permissionPackageTemplateKeys(pkg.Content)}
	}
	return diffPermissionPackageTemplates(previous.Content, pkg.Content)
}

func previousImportTime(imports []PermissionPackageImport, packageID, version string) time.Time {
	for _, item := range imports {
		if item.PackageID == packageID && item.Version == version {
			return item.ImportedAt
		}
	}
	return time.Time{}
}

func diffPermissionPackageTemplates(previous, current PermissionPackageContent) PermissionPackageImportDiff {
	prev := permissionPackageTemplateDigests(previous)
	next := permissionPackageTemplateDigests(current)
	diff := PermissionPackageImportDiff{}
	for key, digest := range next {
		if old, ok := prev[key]; !ok {
			diff.AddedTemplates = append(diff.AddedTemplates, key)
		} else if old != digest {
			diff.ChangedTemplates = append(diff.ChangedTemplates, key)
		}
	}
	for key := range prev {
		if _, ok := next[key]; !ok {
			diff.OrphanedTemplates = append(diff.OrphanedTemplates, key)
		}
	}
	sort.Strings(diff.AddedTemplates)
	sort.Strings(diff.ChangedTemplates)
	sort.Strings(diff.OrphanedTemplates)
	return diff
}

func permissionPackageTemplateKeys(content PermissionPackageContent) []string {
	digests := permissionPackageTemplateDigests(content)
	out := make([]string, 0, len(digests))
	for key := range digests {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

func permissionPackageTemplateDigests(content PermissionPackageContent) map[string]string {
	out := map[string]string{}
	for _, item := range content.PermissionSetTemplates {
		out["permission_set:"+item.TemplateKey] = digestAny(item)
	}
	for _, item := range content.UserGroupTemplates {
		out["user_group:"+item.TemplateKey] = digestAny(item)
	}
	for _, item := range content.AssumableRoleTemplates {
		out["assumable_role:"+item.TemplateKey] = digestAny(item)
	}
	return out
}

func digestAny(value any) string {
	payload, _ := json.Marshal(value)
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

func normalizePermissionPackageContent(content PermissionPackageContent) PermissionPackageContent {
	content.ApplicationCode = strings.TrimSpace(content.ApplicationCode)
	content.Version = strings.TrimSpace(content.Version)
	content.Permissions = normalizePermissions(content.Permissions)
	for i := range content.ResourceTypes {
		content.ResourceTypes[i].ApplicationCode = strings.TrimSpace(content.ResourceTypes[i].ApplicationCode)
		content.ResourceTypes[i].ResourceType = strings.TrimSpace(content.ResourceTypes[i].ResourceType)
		content.ResourceTypes[i].Actions = uniqueStrings(content.ResourceTypes[i].Actions)
		sort.Strings(content.ResourceTypes[i].Actions)
	}
	for i := range content.Actions {
		content.Actions[i].Action = strings.TrimSpace(content.Actions[i].Action)
	}
	for i := range content.PermissionSetTemplates {
		content.PermissionSetTemplates[i].TemplateKey = strings.TrimSpace(content.PermissionSetTemplates[i].TemplateKey)
		content.PermissionSetTemplates[i].Name = strings.TrimSpace(content.PermissionSetTemplates[i].Name)
		content.PermissionSetTemplates[i].Permissions = normalizePermissions(content.PermissionSetTemplates[i].Permissions)
	}
	for i := range content.UserGroupTemplates {
		content.UserGroupTemplates[i].TemplateKey = strings.TrimSpace(content.UserGroupTemplates[i].TemplateKey)
		content.UserGroupTemplates[i].Name = strings.TrimSpace(content.UserGroupTemplates[i].Name)
		content.UserGroupTemplates[i].PermissionSetTemplateKeys = uniqueStrings(content.UserGroupTemplates[i].PermissionSetTemplateKeys)
	}
	for i := range content.AssumableRoleTemplates {
		content.AssumableRoleTemplates[i].TemplateKey = strings.TrimSpace(content.AssumableRoleTemplates[i].TemplateKey)
		content.AssumableRoleTemplates[i].Name = strings.TrimSpace(content.AssumableRoleTemplates[i].Name)
		content.AssumableRoleTemplates[i].PermissionSetTemplateKeys = uniqueStrings(content.AssumableRoleTemplates[i].PermissionSetTemplateKeys)
	}
	return content
}

func validatePackageResourceTypes(items []PermissionPackageResourceType, addField func(string, string, string)) {
	seen := map[string]struct{}{}
	for i, item := range items {
		field := "resource_types"
		if item.ApplicationCode == "" || item.ResourceType == "" {
			addField(field, "invalid", "resource type application_code and resource_type are required")
			continue
		}
		key := item.ApplicationCode + "." + item.ResourceType
		if _, ok := seen[key]; ok {
			addField(field, "invalid", "duplicate resource type: "+key)
		}
		seen[key] = struct{}{}
		if len(item.Actions) == 0 {
			addField(field, "invalid", "resource type actions are required")
		}
		_ = i
	}
}

func validatePackageActions(items []PermissionPackageAction, addField func(string, string, string)) {
	seen := map[string]struct{}{}
	for _, item := range items {
		if item.Action == "" {
			addField("actions", "invalid", "action is required")
			continue
		}
		if _, ok := seen[item.Action]; ok {
			addField("actions", "invalid", "duplicate action: "+item.Action)
		}
		seen[item.Action] = struct{}{}
	}
}

func validatePackagePermissions(items []Permission, addField func(string, string, string)) {
	seen := map[string]struct{}{}
	for _, item := range items {
		if item.Conditions != nil {
			addField("permissions", "invalid", "permission conditions are read-only")
			continue
		}
		item = normalizePermission(item)
		if item.Resource == "" || item.Action == "" {
			addField("permissions", "invalid", "permission resource and action are required")
			continue
		}
		key := string(item.ApplicationCode) + ":" + item.Resource + ":" + string(item.Action)
		if _, ok := seen[key]; ok {
			addField("permissions", "invalid", "duplicate permission: "+key)
			continue
		}
		seen[key] = struct{}{}
	}
}

func validatePackageMenus(items []PermissionPackageMenu, addField func(string, string, string)) {
	seen := map[string]struct{}{}
	var walk func([]PermissionPackageMenu)
	walk = func(nodes []PermissionPackageMenu) {
		for _, item := range nodes {
			if strings.TrimSpace(item.Key) == "" || strings.TrimSpace(item.Label) == "" {
				addField("menus", "invalid", "menu key and label are required")
			}
			if _, ok := seen[item.Key]; ok {
				addField("menus", "invalid", "duplicate menu key: "+item.Key)
			}
			seen[item.Key] = struct{}{}
			walk(item.Children)
		}
	}
	walk(items)
}

func validatePackageButtons(items []PermissionPackageButton, addField func(string, string, string)) {
	seen := map[string]struct{}{}
	for _, item := range items {
		if item.Key == "" || item.Resource == "" || item.Action == "" {
			addField("buttons", "invalid", "button key, resource and action are required")
			continue
		}
		if _, ok := seen[item.Key]; ok {
			addField("buttons", "invalid", "duplicate button key: "+item.Key)
		}
		seen[item.Key] = struct{}{}
	}
}

func validatePackageFields(items []PermissionPackageField, addField func(string, string, string)) {
	for _, item := range items {
		if item.ResourceType == "" || item.FieldName == "" || item.Effect == "" {
			addField("fields", "invalid", "field resource_type, field_name and effect are required")
		}
	}
}

func validatePackageDataScopes(items []PermissionPackageDataScope, addField func(string, string, string)) {
	seen := map[string]struct{}{}
	for _, item := range items {
		if item.Code == "" || item.Name == "" || item.ScopeType == "" {
			addField("data_scopes", "invalid", "data scope code, name and scope_type are required")
			continue
		}
		if _, ok := seen[item.Code]; ok {
			addField("data_scopes", "invalid", "duplicate data scope: "+item.Code)
		}
		seen[item.Code] = struct{}{}
	}
}

func validatePackageTemplates(content PermissionPackageContent, addField func(string, string, string)) {
	permissionSetKeys := map[string]struct{}{}
	for _, item := range content.PermissionSetTemplates {
		if item.TemplateKey == "" || item.Name == "" {
			addField("permission_set_templates", "invalid", "permission set template_key and name are required")
			continue
		}
		if _, ok := permissionSetKeys[item.TemplateKey]; ok {
			addField("permission_set_templates", "invalid", "duplicate permission set template: "+item.TemplateKey)
		}
		permissionSetKeys[item.TemplateKey] = struct{}{}
		if len(item.Permissions) == 0 {
			addField("permission_set_templates", "invalid", "permission set template permissions are required")
		}
		for _, permission := range item.Permissions {
			if permission.Conditions != nil {
				addField("permission_set_templates", "invalid", "permission conditions are read-only")
			}
		}
	}
	validateTemplateRefs := func(field string, templateKey string, refs []string) {
		if templateKey == "" {
			addField(field, "invalid", "template_key is required")
		}
		if len(refs) == 0 {
			addField(field, "invalid", "permission_set_template_keys are required")
		}
		for _, ref := range refs {
			if _, ok := permissionSetKeys[ref]; !ok {
				addField(field, "not_found", "permission set template not found: "+ref)
			}
		}
	}
	seenGroups := map[string]struct{}{}
	for _, item := range content.UserGroupTemplates {
		validateTemplateRefs("user_group_templates", item.TemplateKey, item.PermissionSetTemplateKeys)
		if _, ok := seenGroups[item.TemplateKey]; ok {
			addField("user_group_templates", "invalid", "duplicate user group template: "+item.TemplateKey)
		}
		seenGroups[item.TemplateKey] = struct{}{}
	}
	seenRoles := map[string]struct{}{}
	for _, item := range content.AssumableRoleTemplates {
		validateTemplateRefs("assumable_role_templates", item.TemplateKey, item.PermissionSetTemplateKeys)
		if _, ok := seenRoles[item.TemplateKey]; ok {
			addField("assumable_role_templates", "invalid", "duplicate assumable role template: "+item.TemplateKey)
		}
		seenRoles[item.TemplateKey] = struct{}{}
	}
}

func validatePackageFGAMappings(items []PermissionPackageFGAMapping, addField func(string, string, string)) {
	seen := map[string]struct{}{}
	for _, item := range items {
		if item.ResourceType == "" || item.OpenFGAType == "" {
			addField("fga_mappings", "invalid", "fga mapping resource_type and openfga_type are required")
			continue
		}
		if _, ok := seen[item.ResourceType]; ok {
			addField("fga_mappings", "invalid", "duplicate fga mapping: "+item.ResourceType)
		}
		seen[item.ResourceType] = struct{}{}
	}
}

func permissionPackageResourceTypesFromPermissions(permissions []Permission) []PermissionPackageResourceType {
	grouped := map[string]map[string]struct{}{}
	for _, perm := range permissions {
		perm = normalizePermission(perm)
		key := string(perm.ApplicationCode) + "\x00" + string(perm.ResourceType)
		if grouped[key] == nil {
			grouped[key] = map[string]struct{}{}
		}
		grouped[key][string(perm.Action)] = struct{}{}
	}
	out := make([]PermissionPackageResourceType, 0, len(grouped))
	for key, actions := range grouped {
		parts := strings.SplitN(key, "\x00", 2)
		actionList := make([]string, 0, len(actions))
		for action := range actions {
			actionList = append(actionList, action)
		}
		sort.Strings(actionList)
		out = append(out, PermissionPackageResourceType{ApplicationCode: parts[0], ResourceType: parts[1], Actions: actionList})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ApplicationCode == out[j].ApplicationCode {
			return out[i].ResourceType < out[j].ResourceType
		}
		return out[i].ApplicationCode < out[j].ApplicationCode
	})
	return out
}

func permissionPackageActionsFromPermissions(permissions []Permission) []PermissionPackageAction {
	seen := map[string]PermissionPackageAction{}
	for _, perm := range permissions {
		action := strings.TrimSpace(string(perm.Action))
		if action == "" {
			continue
		}
		item := seen[action]
		item.Action = action
		item.HighRisk = item.HighRisk || perm.HighRisk || perm.RiskLevel == string(domain.RiskHigh) || perm.RiskLevel == string(domain.RiskCritical)
		seen[action] = item
	}
	out := make([]PermissionPackageAction, 0, len(seen))
	for _, item := range seen {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Action < out[j].Action })
	return out
}

func permissionPackageButtonsFromPermissions(permissions []Permission) []PermissionPackageButton {
	out := make([]PermissionPackageButton, 0)
	seen := map[string]struct{}{}
	for _, perm := range permissions {
		perm = normalizePermission(perm)
		if perm.MenuKey == "" {
			continue
		}
		key := string(perm.ApplicationCode) + "." + string(perm.ResourceType) + "." + string(perm.Action)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, PermissionPackageButton{
			Key:      key,
			Resource: perm.Resource,
			Action:   string(perm.Action),
			Name:     perm.Name,
			HighRisk: perm.HighRisk || perm.RiskLevel == string(domain.RiskHigh) || perm.RiskLevel == string(domain.RiskCritical),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

func permissionPackageMenusFromNodes(nodes []MenuNode) []PermissionPackageMenu {
	out := make([]PermissionPackageMenu, 0, len(nodes))
	for index, node := range nodes {
		out = append(out, PermissionPackageMenu{
			Key:       node.Key,
			Label:     node.Label,
			Path:      node.Path,
			Icon:      node.Icon,
			SortOrder: index,
			Children:  permissionPackageMenusFromNodes(node.Children),
		})
	}
	return out
}

func permissionSetTemplatesFromPackage(pkg PermissionPackage) []PermissionSetTemplate {
	out := make([]PermissionSetTemplate, 0, len(pkg.Content.PermissionSetTemplates))
	for _, item := range pkg.Content.PermissionSetTemplates {
		out = append(out, PermissionSetTemplate{
			ID:          stablePermissionTemplateID("pst", pkg.ID, item.TemplateKey),
			PackageID:   pkg.ID,
			TemplateKey: item.TemplateKey,
			Name:        item.Name,
			Content:     item,
			Version:     pkg.Version,
		})
	}
	return out
}

func userGroupTemplatesFromPackage(pkg PermissionPackage) []UserGroupTemplate {
	out := make([]UserGroupTemplate, 0, len(pkg.Content.UserGroupTemplates))
	for _, item := range pkg.Content.UserGroupTemplates {
		out = append(out, UserGroupTemplate{
			ID:          stablePermissionTemplateID("ugt", pkg.ID, item.TemplateKey),
			PackageID:   pkg.ID,
			TemplateKey: item.TemplateKey,
			Name:        item.Name,
			Content:     item,
			Version:     pkg.Version,
		})
	}
	return out
}

func assumableRoleTemplatesFromPackage(pkg PermissionPackage) []AssumableRoleTemplate {
	out := make([]AssumableRoleTemplate, 0, len(pkg.Content.AssumableRoleTemplates))
	for _, item := range pkg.Content.AssumableRoleTemplates {
		out = append(out, AssumableRoleTemplate{
			ID:          stablePermissionTemplateID("art", pkg.ID, item.TemplateKey),
			PackageID:   pkg.ID,
			TemplateKey: item.TemplateKey,
			Name:        item.Name,
			Content:     item,
			Version:     pkg.Version,
		})
	}
	return out
}

func stablePermissionPackageID(applicationCode, version string) string {
	return stableCatalogID("pkg", strings.TrimSpace(applicationCode), strings.TrimSpace(version))
}

func stablePermissionPackageImportID(tenantID, packageID, version string) string {
	return stableCatalogID("pki", tenantID, packageID, version)
}

func stablePermissionTemplateID(prefix string, parts ...string) string {
	if len(parts) == 0 {
		return stableCatalogID(prefix, "")
	}
	return stableCatalogID(prefix, parts[0], parts[1:]...)
}
