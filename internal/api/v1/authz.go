package v1

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/service"
)

type AuthzCtrl struct {
	routes routeBinder
	svc    service.AuthzFacade
}

func (c AuthzCtrl) RegisterRoutes(router *gin.RouterGroup) {
	authz := router.Group("/authz")
	authz.POST("/check", c.routes.Handle("iam.authz", "check", c.checkAuthz))
	authz.POST("/batch-check", c.routes.Handle("iam.authz", "check", c.batchCheckAuthz))
	authz.POST("/explain", c.routes.Handle("iam.authz", "explain", c.explainAuthz))
	authz.POST("/simulate", c.routes.Handle("iam.authz", "simulate", c.simulateAuthz))
}

func (a *API) authorize(ctx domain.RequestContext, r *http.Request, resource, action string, authz routeAuthz) error {
	req := domain.CheckRequest{Resource: resource, Action: domain.Action(action)}
	if authz.resourceIDParam != "" {
		req.ResourceID = r.PathValue(authz.resourceIDParam)
	}
	if authz.targetEmployeeIDParam != "" {
		req.TargetEmployeeID = r.PathValue(authz.targetEmployeeIDParam)
		if req.ResourceID == "" {
			req.ResourceID = req.TargetEmployeeID
		}
	}
	result, err := a.authz.Check(ctx, req)
	if err != nil {
		return err
	}
	if !result.Allowed {
		return domain.ForbiddenReason(apiAuthzReasonCode(result), result.Reason)
	}
	if result.RequiresApproval && ctx.ApprovalInstanceID != "" {
		if err := a.authz.ValidateApprovalInstance(ctx, req); err != nil {
			return err
		}
	}
	if result.RequiresApproval && ctx.ApprovalInstanceID == "" && !ctx.ApprovalConfirmed {
		return domain.ForbiddenReason("approval_required", "high-risk action requires approval confirmation")
	}
	return nil
}

func apiAuthzReasonCode(result domain.CheckResult) string {
	switch result.Reason {
	case "missing permission":
		switch result.Action {
		case domain.ActionRead:
			return "menu_denied"
		case domain.ActionCreate, domain.ActionUpdate, domain.ActionDelete, domain.ActionExport, domain.ActionImport, domain.ActionInvite, domain.ActionUpdateStatus, domain.ActionStatusTransition:
			return "button_denied"
		default:
			return "permission_missing"
		}
	default:
		return "permission_missing"
	}
}

func (c AuthzCtrl) checkAuthz(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	var input domain.CheckRequest
	if err := readJSON(w, r, &input); err != nil {
		return err
	}
	result, err := c.svc.Check(ctx, input)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, result)
	return nil
}

func (c AuthzCtrl) explainAuthz(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	var input domain.CheckRequest
	if err := readJSON(w, r, &input); err != nil {
		return err
	}
	result, err := c.svc.Check(ctx, input)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, domain.AuthzExplainResponse{Decision: result, Explain: result.Reason})
	return nil
}

func (c AuthzCtrl) simulateAuthz(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	var input domain.CheckRequest
	if err := readJSON(w, r, &input); err != nil {
		return err
	}
	result, err := c.svc.Check(ctx, input)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, domain.AuthzSimulationResponse{Decision: result, Simulated: true})
	return nil
}

func (c AuthzCtrl) batchCheckAuthz(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	var input domain.BatchCheckRequest
	if err := readJSON(w, r, &input); err != nil {
		return err
	}
	result, err := c.svc.BatchCheck(ctx, input)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, result)
	return nil
}
