package handler

import "git.corp.ikala.tv/nexus-pro/nexus-pro-be/internal/authz"

// MeDTO is the GET /v1/me payload. role_ids carries the caller's group ids for
// backward compatibility with the prototype's role-driven UI.
type MeDTO struct {
	TenantID     string                    `json:"tenant_id"`
	AccountID    string                    `json:"account_id"`
	Email        string                    `json:"email"`
	Name         string                    `json:"name"`
	RoleIDs      []string                  `json:"role_ids"`
	Capabilities map[string]authz.Decision `json:"capabilities"`
}

// MenuDTO mirrors the frontend MenuItem shape.
type MenuDTO struct {
	ID                   string `json:"id"`
	ParentID             string `json:"parent_id,omitempty"`
	Label                string `json:"label"`
	Route                string `json:"route"`
	Icon                 string `json:"icon"`
	RequiredPermissionID string `json:"required_permission_id"`
	SortOrder            int    `json:"sort_order"`
}

// CheckRequest is the POST /v1/authz/check body (frontend-compatible).
type CheckRequest struct {
	ApplicationCode string `json:"application_code"`
	Resource        string `json:"resource"`
	Action          string `json:"action"`
	ResourceID      string `json:"resource_id"`
}

func (c CheckRequest) toAuthz() authz.Request {
	return authz.Request{
		ApplicationCode: c.ApplicationCode,
		ResourceType:    c.Resource,
		Action:          c.Action,
		ResourceID:      c.ResourceID,
	}
}

// BatchCheckRequest is the POST /v1/authz/batch-check body.
type BatchCheckRequest struct {
	Checks []CheckRequest `json:"checks"`
}

// RoleDTO is the legacy /v1/iam/roles shape (compat shim over user groups).
type RoleDTO struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	Description      string   `json:"description"`
	RiskLevel        string   `json:"risk_level"`
	PermissionSetIDs []string `json:"permission_set_ids"`
}

// CreateUserGroupRequest is the POST /v1/iam/user-groups body.
type CreateUserGroupRequest struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// AssumeRequest is the POST /v1/iam/assumable-roles/:id/assume body (§9).
type AssumeRequest struct {
	Reason          string         `json:"reason"`
	DurationMinutes int            `json:"duration_minutes"`
	SessionPolicy   map[string]any `json:"session_policy"`
}

// AssumeResponse mirrors the §9 assume response.
type AssumeResponse struct {
	SessionID          string `json:"session_id"`
	AssumedRole        string `json:"assumed_role"`
	ExpiresAt          string `json:"expires_at"`
	PermissionBoundary string `json:"permission_boundary,omitempty"`
	RequiresApproval   bool   `json:"requires_approval"`
}
