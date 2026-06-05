package handler

import (
	"net/http"
	"strings"

	"git.corp.ikala.tv/nexus-pro/nexus-pro-be/internal/models"
	"git.corp.ikala.tv/nexus-pro/nexus-pro-be/internal/platform/idgen"
	"git.corp.ikala.tv/nexus-pro/nexus-pro-be/internal/platform/reqctx"
	"git.corp.ikala.tv/nexus-pro/nexus-pro-be/internal/platform/response"
	"github.com/gin-gonic/gin"
)

// --- Dictionaries -----------------------------------------------------------

func (h *Handler) ListApplications(c *gin.Context) {
	apps, err := h.repo.ListApplications(c.Request.Context())
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Items(c, apps)
}

func (h *Handler) ListResourceTypes(c *gin.Context) {
	apps, err := h.repo.ListApplications(c.Request.Context())
	if err != nil {
		response.Error(c, err)
		return
	}
	type rt struct {
		ApplicationCode string `json:"application_code"`
		ResourceTypes   any    `json:"resource_types"`
	}
	out := make([]rt, 0, len(apps))
	for _, a := range apps {
		out = append(out, rt{ApplicationCode: a.ApplicationCode, ResourceTypes: a.ResourceTypes})
	}
	response.Items(c, out)
}

func (h *Handler) ListPermissions(c *gin.Context) {
	perms, err := h.repo.ListPermissions(c.Request.Context())
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Items(c, perms)
}

// --- User groups ------------------------------------------------------------

func (h *Handler) ListUserGroups(c *gin.Context) {
	groups, err := h.repo.ListUserGroups(c.Request.Context())
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Items(c, groups)
}

func (h *Handler) CreateUserGroup(c *gin.Context) {
	var req CreateUserGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad_request", "message": "name required"})
		return
	}
	p, _ := reqctx.Principal(c.Request.Context())
	g := &models.UserGroup{Code: req.Code, Name: req.Name, Description: req.Description, Source: "manual"}
	g.ID = idgen.New("group")
	g.TenantID = p.TenantID
	if err := h.repo.CreateUserGroup(c.Request.Context(), g); err != nil {
		response.Error(c, err)
		return
	}
	c.JSON(http.StatusCreated, g)
}

// --- Permission sets --------------------------------------------------------

func (h *Handler) ListPermissionSets(c *gin.Context) {
	ctx := c.Request.Context()
	sets, err := h.repo.ListPermissionSets(ctx)
	if err != nil {
		response.Error(c, err)
		return
	}
	type psDTO struct {
		models.PermissionSet
		PermissionIDs []string `json:"permission_ids"`
	}
	out := make([]psDTO, 0, len(sets))
	for _, s := range sets {
		ids, err := h.repo.PermissionIDsForSet(ctx, s.ID)
		if err != nil {
			response.Error(c, err)
			return
		}
		if ids == nil {
			ids = []string{}
		}
		out = append(out, psDTO{PermissionSet: s, PermissionIDs: ids})
	}
	response.Items(c, out)
}

func (h *Handler) CreatePermissionSet(c *gin.Context) {
	var req struct {
		Name          string   `json:"name"`
		Description   string   `json:"description"`
		PermissionIDs []string `json:"permission_ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad_request", "message": "name required"})
		return
	}
	ctx := c.Request.Context()
	p, _ := reqctx.Principal(ctx)
	ps := &models.PermissionSet{Name: req.Name, Description: req.Description, Source: "manual", Version: 1}
	ps.ID = idgen.New("ps")
	ps.TenantID = p.TenantID
	if err := h.repo.CreatePermissionSet(ctx, ps); err != nil {
		response.Error(c, err)
		return
	}
	if err := h.repo.AddSetPermissions(ctx, p.TenantID, ps.ID, req.PermissionIDs); err != nil {
		response.Error(c, err)
		return
	}
	c.JSON(http.StatusCreated, ps)
}

// --- Assignments ------------------------------------------------------------

func (h *Handler) ListAssignments(c *gin.Context) {
	a, err := h.repo.ListAssignments(c.Request.Context())
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Items(c, a)
}

func (h *Handler) CreateAssignment(c *gin.Context) {
	var req struct {
		PermissionSetID string `json:"permission_set_id"`
		SubjectType     string `json:"subject_type"`
		SubjectID       string `json:"subject_id"`
		Effect          string `json:"effect"`
		DataScopeID     string `json:"data_scope_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.PermissionSetID == "" || req.SubjectType == "" || req.SubjectID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad_request", "message": "permission_set_id, subject_type, subject_id required"})
		return
	}
	if req.Effect == "" {
		req.Effect = "allow"
	}
	ctx := c.Request.Context()
	p, _ := reqctx.Principal(ctx)
	a := &models.PermissionSetAssignment{
		PermissionSetID: req.PermissionSetID,
		SubjectType:     req.SubjectType,
		SubjectID:       req.SubjectID,
		Effect:          req.Effect,
		DataScopeID:     req.DataScopeID,
	}
	a.ID = idgen.New("psa")
	a.TenantID = p.TenantID
	if err := h.repo.CreateAssignment(ctx, a); err != nil {
		response.Error(c, err)
		return
	}
	c.JSON(http.StatusCreated, a)
}

// --- Field policies ---------------------------------------------------------

func (h *Handler) ListFieldPolicies(c *gin.Context) {
	fps, err := h.repo.ListFieldPolicies(c.Request.Context())
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Items(c, fps)
}

func (h *Handler) CreateFieldPolicy(c *gin.Context) {
	var req struct {
		ApplicationCode string `json:"application_code"`
		ResourceType    string `json:"resource_type"`
		Field           string `json:"field"`
		Effect          string `json:"effect"`
		Sensitivity     string `json:"sensitivity"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.ResourceType == "" || req.Field == "" || req.Effect == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad_request", "message": "resource_type, field, effect required"})
		return
	}
	ctx := c.Request.Context()
	p, _ := reqctx.Principal(ctx)
	f := &models.FieldPolicy{
		ApplicationCode: req.ApplicationCode,
		ResourceType:    req.ResourceType,
		Field:           req.Field,
		Effect:          req.Effect,
		Sensitivity:     req.Sensitivity,
	}
	f.ID = idgen.New("fp")
	f.TenantID = p.TenantID
	if err := h.repo.CreateFieldPolicy(ctx, f); err != nil {
		response.Error(c, err)
		return
	}
	c.JSON(http.StatusCreated, f)
}

// --- Data scopes ------------------------------------------------------------

func (h *Handler) ListDataScopes(c *gin.Context) {
	ds, err := h.repo.ListDataScopes(c.Request.Context())
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Items(c, ds)
}

func (h *Handler) CreateDataScope(c *gin.Context) {
	var req struct {
		Name      string `json:"name"`
		ScopeType string `json:"scope_type"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.ScopeType == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad_request", "message": "scope_type required"})
		return
	}
	ctx := c.Request.Context()
	p, _ := reqctx.Principal(ctx)
	d := &models.DataScope{Name: req.Name, ScopeType: req.ScopeType}
	d.ID = idgen.New("ds")
	d.TenantID = p.TenantID
	if err := h.repo.CreateDataScope(ctx, d); err != nil {
		response.Error(c, err)
		return
	}
	c.JSON(http.StatusCreated, d)
}

// --- Assumable roles --------------------------------------------------------

func (h *Handler) ListAssumableRoles(c *gin.Context) {
	roles, err := h.repo.ListAssumableRoles(c.Request.Context())
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Items(c, roles)
}

func (h *Handler) CreateAssumableRole(c *gin.Context) {
	var req struct {
		Name                 string `json:"name"`
		Description          string `json:"description"`
		PermissionBoundaryID string `json:"permission_boundary_id"`
		MaxSessionMinutes    int    `json:"max_session_minutes"`
		RequiresApproval     bool   `json:"requires_approval"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad_request", "message": "name required"})
		return
	}
	if req.MaxSessionMinutes <= 0 {
		req.MaxSessionMinutes = 60
	}
	ctx := c.Request.Context()
	p, _ := reqctx.Principal(ctx)
	role := &models.AssumableRole{
		Name:                 req.Name,
		Description:          req.Description,
		PermissionBoundaryID: req.PermissionBoundaryID,
		MaxSessionMinutes:    req.MaxSessionMinutes,
		RequiresApproval:     req.RequiresApproval,
		AuditLevel:           "full",
	}
	role.ID = idgen.New("assumable")
	role.TenantID = p.TenantID
	if err := h.repo.CreateAssumableRole(ctx, role); err != nil {
		response.Error(c, err)
		return
	}
	c.JSON(http.StatusCreated, role)
}

// Assume creates a controlled session for the given assumable role (§9).
func (h *Handler) Assume(c *gin.Context) {
	roleID := c.Param("id")
	var req AssumeRequest
	_ = c.ShouldBindJSON(&req)
	ctx := c.Request.Context()
	p, _ := reqctx.Principal(ctx)
	res, err := h.assume.Assume(ctx, p, roleID, req.Reason, req.DurationMinutes, req.SessionPolicy)
	if err != nil {
		response.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, AssumeResponse{
		SessionID:          res.SessionID,
		AssumedRole:        res.AssumedRole,
		ExpiresAt:          res.ExpiresAt.Format("2006-01-02T15:04:05Z07:00"),
		PermissionBoundary: res.PermissionBoundary,
		RequiresApproval:   res.RequiresApproval,
	})
}

// --- Audit ------------------------------------------------------------------

func (h *Handler) ListAuditLogs(c *gin.Context) {
	logs, err := h.repo.ListAuditLogs(c.Request.Context(), 100)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Items(c, logs)
}

// --- Compatibility shims (legacy role API over user groups) -----------------

func (h *Handler) ListRoles(c *gin.Context) {
	ctx := c.Request.Context()
	groups, err := h.repo.ListUserGroups(ctx)
	if err != nil {
		response.Error(c, err)
		return
	}
	assignments, err := h.repo.ListAssignments(ctx)
	if err != nil {
		response.Error(c, err)
		return
	}
	setsBySubject := map[string][]string{}
	for _, a := range assignments {
		if a.SubjectType == "group" && a.Effect == "allow" {
			setsBySubject[a.SubjectID] = append(setsBySubject[a.SubjectID], a.PermissionSetID)
		}
	}
	out := make([]RoleDTO, 0, len(groups))
	for _, g := range groups {
		sets := setsBySubject[g.ID]
		if sets == nil {
			sets = []string{}
		}
		out = append(out, RoleDTO{
			ID:               g.ID,
			Name:             g.Name,
			Description:      g.Description,
			RiskLevel:        riskForGroup(g.Code),
			PermissionSetIDs: sets,
		})
	}
	response.Items(c, out)
}

func (h *Handler) ListRoleBindings(c *gin.Context) {
	a, err := h.repo.ListAssignments(c.Request.Context())
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Items(c, a)
}

func riskForGroup(code string) string {
	switch {
	case strings.Contains(code, "tenant-admin"):
		return "critical"
	case strings.Contains(code, "admin"):
		return "high"
	default:
		return "normal"
	}
}
