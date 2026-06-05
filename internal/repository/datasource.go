package repository

import (
	"context"
	"time"

	"git.corp.ikala.tv/nexus-pro/nexus-pro-be/internal/authz"
	"git.corp.ikala.tv/nexus-pro/nexus-pro-be/internal/models"
)

// Assignments returns active allow/deny assignments for the given subjects,
// resolving each assignment's data scope type. Implements authz.DataSource.
func (r *Repository) Assignments(ctx context.Context, subjects []authz.SubjectRef) ([]authz.AssignmentInfo, error) {
	db, err := tx(ctx)
	if err != nil {
		return nil, err
	}
	if len(subjects) == 0 {
		return nil, nil
	}

	ids := make([]string, 0, len(subjects))
	want := make(map[string]string, len(subjects)) // subject_id -> subject_type
	for _, s := range subjects {
		ids = append(ids, s.ID)
		want[s.ID] = s.Type
	}

	type row struct {
		PermissionSetID string
		SubjectType     string
		SubjectID       string
		Effect          string
		ScopeType       string
	}
	var rows []row
	err = db.Table("iam_permission_set_assignments AS a").
		Select("a.permission_set_id, a.subject_type, a.subject_id, a.effect, ds.scope_type").
		Joins("LEFT JOIN iam_data_scopes ds ON ds.id = a.data_scope_id").
		Where("a.deleted_at IS NULL").
		Where("a.subject_id IN ?", ids).
		Where("(a.valid_from IS NULL OR a.valid_from <= now())").
		Where("(a.valid_until IS NULL OR a.valid_until >= now())").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	out := make([]authz.AssignmentInfo, 0, len(rows))
	for _, r := range rows {
		// Filter to the exact (type,id) pairs requested.
		if want[r.SubjectID] != r.SubjectType {
			continue
		}
		out = append(out, authz.AssignmentInfo{
			PermissionSetID: r.PermissionSetID,
			SubjectType:     r.SubjectType,
			Effect:          r.Effect,
			DataScopeType:   r.ScopeType,
			SourceLabel:     r.SubjectType + "." + r.SubjectID,
		})
	}
	return out, nil
}

// PermissionsForSets returns permissions grouped by permission set id.
func (r *Repository) PermissionsForSets(ctx context.Context, setIDs []string) (map[string][]models.Permission, error) {
	db, err := tx(ctx)
	if err != nil {
		return nil, err
	}
	out := map[string][]models.Permission{}
	if len(setIDs) == 0 {
		return out, nil
	}

	type row struct {
		PermissionSetID string
		models.Permission
	}
	var rows []row
	err = db.Table("iam_permission_set_permissions AS j").
		Select("j.permission_set_id, p.*").
		Joins("JOIN iam_permissions p ON p.id = j.permission_id").
		Where("j.permission_set_id IN ?", setIDs).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[r.PermissionSetID] = append(out[r.PermissionSetID], r.Permission)
	}
	return out, nil
}

// FieldPolicies returns field policies for a resource type.
func (r *Repository) FieldPolicies(ctx context.Context, applicationCode, resourceType string) ([]models.FieldPolicy, error) {
	db, err := tx(ctx)
	if err != nil {
		return nil, err
	}
	var out []models.FieldPolicy
	q := db.Where("resource_type = ?", resourceType)
	if applicationCode != "" {
		q = q.Where("application_code = ?", applicationCode)
	}
	err = q.Find(&out).Error
	return out, err
}

// Menus returns menu items for an application, ordered by sort_order.
func (r *Repository) Menus(ctx context.Context, applicationCode string) ([]models.MenuItem, error) {
	db, err := tx(ctx)
	if err != nil {
		return nil, err
	}
	var out []models.MenuItem
	q := db.Order("sort_order ASC")
	if applicationCode != "" {
		q = q.Where("application_code = ?", applicationCode)
	}
	err = q.Find(&out).Error
	return out, err
}

// PermissionsByIDs returns permissions keyed by id.
func (r *Repository) PermissionsByIDs(ctx context.Context, ids []string) (map[string]models.Permission, error) {
	db, err := tx(ctx)
	if err != nil {
		return nil, err
	}
	out := map[string]models.Permission{}
	if len(ids) == 0 {
		return out, nil
	}
	var perms []models.Permission
	if err := db.Where("id IN ?", ids).Find(&perms).Error; err != nil {
		return nil, err
	}
	for _, p := range perms {
		out[p.ID] = p
	}
	return out, nil
}

// ActiveSession returns an active, unexpired assume session.
func (r *Repository) ActiveSession(ctx context.Context, sessionID string) (*models.AssumableRoleSession, error) {
	db, err := tx(ctx)
	if err != nil {
		return nil, err
	}
	var s models.AssumableRoleSession
	err = db.Where("id = ? AND status = ? AND expires_at > ?", sessionID, "active", time.Now()).First(&s).Error
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// Boundary returns a permission boundary by id.
func (r *Repository) Boundary(ctx context.Context, id string) (*models.PermissionBoundary, error) {
	db, err := tx(ctx)
	if err != nil {
		return nil, err
	}
	var b models.PermissionBoundary
	if err := db.Where("id = ?", id).First(&b).Error; err != nil {
		return nil, err
	}
	return &b, nil
}
