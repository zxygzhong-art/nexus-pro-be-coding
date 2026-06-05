// Package handler implements the HTTP handlers for runtime authz endpoints and
// IAM management endpoints, kept contract-compatible with the existing frontend.
package handler

import (
	"net/http"

	"git.corp.ikala.tv/nexus-pro/nexus-pro-be/internal/adapters/authorizer"
	"git.corp.ikala.tv/nexus-pro/nexus-pro-be/internal/iam/service"
	"git.corp.ikala.tv/nexus-pro/nexus-pro-be/internal/platform/reqctx"
	"git.corp.ikala.tv/nexus-pro/nexus-pro-be/internal/platform/response"
	"git.corp.ikala.tv/nexus-pro/nexus-pro-be/internal/repository"
	"github.com/gin-gonic/gin"
)

// Handler bundles the dependencies shared by runtime and IAM handlers.
type Handler struct {
	repo     *repository.Repository
	az       authorizer.Authorizer
	identity *service.IdentityService
	assume   *service.AssumableRoleService
}

// New builds a Handler.
func New(repo *repository.Repository, az authorizer.Authorizer, identity *service.IdentityService, assume *service.AssumableRoleService) *Handler {
	return &Handler{repo: repo, az: az, identity: identity, assume: assume}
}

// defaultApplication is the application served by the runtime menu/me endpoints.
const defaultApplication = "hr"

// Healthz is an unauthenticated liveness probe.
func (h *Handler) Healthz(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// Me returns the current principal plus a capability summary.
func (h *Handler) Me(c *gin.Context) {
	ctx := c.Request.Context()
	p, ok := reqctx.Principal(ctx)
	if !ok {
		response.Error(c, nil)
		return
	}
	caps, err := h.identity.Capabilities(ctx)
	if err != nil {
		response.Error(c, err)
		return
	}
	roleIDs := p.GroupIDs
	if roleIDs == nil {
		roleIDs = []string{}
	}
	response.OK(c, MeDTO{
		TenantID:     p.TenantID,
		AccountID:    p.AccountID,
		Email:        p.Email,
		Name:         p.Name,
		RoleIDs:      roleIDs,
		Capabilities: caps,
	})
}

// Menus returns the permission-pruned menu tree.
func (h *Handler) Menus(c *gin.Context) {
	ctx := c.Request.Context()
	menus, err := h.az.PruneMenus(ctx, defaultApplication)
	if err != nil {
		response.Error(c, err)
		return
	}
	items := make([]MenuDTO, 0, len(menus))
	for _, m := range menus {
		items = append(items, MenuDTO{
			ID:                   m.ID,
			ParentID:             m.ParentID,
			Label:                m.Label,
			Route:                m.Route,
			Icon:                 m.Icon,
			RequiredPermissionID: m.RequiredPermissionID,
			SortOrder:            m.SortOrder,
		})
	}
	response.Items(c, items)
}

// AuthzCheck evaluates a single authorization request.
func (h *Handler) AuthzCheck(c *gin.Context) {
	var req CheckRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad_request", "message": "invalid body"})
		return
	}
	dec, err := h.az.Check(c.Request.Context(), req.toAuthz())
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, dec)
}

// AuthzBatchCheck evaluates many requests, preserving order.
func (h *Handler) AuthzBatchCheck(c *gin.Context) {
	var body BatchCheckRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad_request", "message": "invalid body"})
		return
	}
	decisions := make([]any, 0, len(body.Checks))
	for _, ch := range body.Checks {
		dec, err := h.az.Check(c.Request.Context(), ch.toAuthz())
		if err != nil {
			response.Error(c, err)
			return
		}
		decisions = append(decisions, dec)
	}
	c.JSON(http.StatusOK, gin.H{"decisions": decisions})
}

// Explain returns the decision with matched/missing detail (same engine output).
func (h *Handler) Explain(c *gin.Context) {
	var req CheckRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad_request", "message": "invalid body"})
		return
	}
	dec, err := h.az.Check(c.Request.Context(), req.toAuthz())
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, dec)
}

// Simulate is a scaffold that currently evaluates against the live state.
func (h *Handler) Simulate(c *gin.Context) {
	h.Explain(c)
}
