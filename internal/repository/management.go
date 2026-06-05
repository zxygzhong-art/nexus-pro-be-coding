package repository

import (
	"context"

	"git.corp.ikala.tv/nexus-pro/nexus-pro-be/internal/models"
)

// --- List queries -----------------------------------------------------------

func (r *Repository) ListApplications(ctx context.Context) ([]models.Application, error) {
	db, err := tx(ctx)
	if err != nil {
		return nil, err
	}
	var out []models.Application
	return out, db.Order("application_code").Find(&out).Error
}

func (r *Repository) ListPermissions(ctx context.Context) ([]models.Permission, error) {
	db, err := tx(ctx)
	if err != nil {
		return nil, err
	}
	var out []models.Permission
	return out, db.Order("id").Find(&out).Error
}

func (r *Repository) ListUserGroups(ctx context.Context) ([]models.UserGroup, error) {
	db, err := tx(ctx)
	if err != nil {
		return nil, err
	}
	var out []models.UserGroup
	return out, db.Order("name").Find(&out).Error
}

func (r *Repository) ListPermissionSets(ctx context.Context) ([]models.PermissionSet, error) {
	db, err := tx(ctx)
	if err != nil {
		return nil, err
	}
	var out []models.PermissionSet
	return out, db.Order("name").Find(&out).Error
}

func (r *Repository) PermissionIDsForSet(ctx context.Context, setID string) ([]string, error) {
	db, err := tx(ctx)
	if err != nil {
		return nil, err
	}
	var ids []string
	return ids, db.Model(&models.PermissionSetPermission{}).
		Where("permission_set_id = ?", setID).Pluck("permission_id", &ids).Error
}

func (r *Repository) ListAssignments(ctx context.Context) ([]models.PermissionSetAssignment, error) {
	db, err := tx(ctx)
	if err != nil {
		return nil, err
	}
	var out []models.PermissionSetAssignment
	return out, db.Order("created_at DESC").Find(&out).Error
}

func (r *Repository) ListFieldPolicies(ctx context.Context) ([]models.FieldPolicy, error) {
	db, err := tx(ctx)
	if err != nil {
		return nil, err
	}
	var out []models.FieldPolicy
	return out, db.Order("resource_type, field").Find(&out).Error
}

func (r *Repository) ListDataScopes(ctx context.Context) ([]models.DataScope, error) {
	db, err := tx(ctx)
	if err != nil {
		return nil, err
	}
	var out []models.DataScope
	return out, db.Order("scope_type").Find(&out).Error
}

func (r *Repository) ListAssumableRoles(ctx context.Context) ([]models.AssumableRole, error) {
	db, err := tx(ctx)
	if err != nil {
		return nil, err
	}
	var out []models.AssumableRole
	return out, db.Order("name").Find(&out).Error
}

func (r *Repository) GetAssumableRole(ctx context.Context, id string) (*models.AssumableRole, error) {
	db, err := tx(ctx)
	if err != nil {
		return nil, err
	}
	var role models.AssumableRole
	if err := db.Where("id = ?", id).First(&role).Error; err != nil {
		return nil, err
	}
	return &role, nil
}

func (r *Repository) ListAuditLogs(ctx context.Context, limit int) ([]models.AuditLog, error) {
	db, err := tx(ctx)
	if err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	var out []models.AuditLog
	return out, db.Order("created_at DESC").Limit(limit).Find(&out).Error
}

// --- Writes -----------------------------------------------------------------

func (r *Repository) CreateUserGroup(ctx context.Context, g *models.UserGroup) error {
	db, err := tx(ctx)
	if err != nil {
		return err
	}
	return db.Create(g).Error
}

func (r *Repository) CreatePermissionSet(ctx context.Context, ps *models.PermissionSet) error {
	db, err := tx(ctx)
	if err != nil {
		return err
	}
	return db.Create(ps).Error
}

// AddSetPermissions inserts permission_set -> permission join rows, ignoring dups.
func (r *Repository) AddSetPermissions(ctx context.Context, tenantID, setID string, permissionIDs []string) error {
	db, err := tx(ctx)
	if err != nil {
		return err
	}
	if len(permissionIDs) == 0 {
		return nil
	}
	rows := make([]models.PermissionSetPermission, 0, len(permissionIDs))
	for _, pid := range permissionIDs {
		rows = append(rows, models.PermissionSetPermission{
			TenantID: tenantID, PermissionSetID: setID, PermissionID: pid,
		})
	}
	return db.Create(&rows).Error
}

func (r *Repository) CreateAssignment(ctx context.Context, a *models.PermissionSetAssignment) error {
	db, err := tx(ctx)
	if err != nil {
		return err
	}
	return db.Create(a).Error
}

func (r *Repository) CreateFieldPolicy(ctx context.Context, f *models.FieldPolicy) error {
	db, err := tx(ctx)
	if err != nil {
		return err
	}
	return db.Create(f).Error
}

func (r *Repository) CreateDataScope(ctx context.Context, d *models.DataScope) error {
	db, err := tx(ctx)
	if err != nil {
		return err
	}
	return db.Create(d).Error
}

func (r *Repository) CreateAssumableRole(ctx context.Context, role *models.AssumableRole) error {
	db, err := tx(ctx)
	if err != nil {
		return err
	}
	return db.Create(role).Error
}

func (r *Repository) CreateSession(ctx context.Context, s *models.AssumableRoleSession) error {
	db, err := tx(ctx)
	if err != nil {
		return err
	}
	return db.Create(s).Error
}

func (r *Repository) CreateAuditLog(ctx context.Context, a *models.AuditLog) error {
	db, err := tx(ctx)
	if err != nil {
		return err
	}
	return db.Create(a).Error
}
