package v1

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/service"
)

// AuthzCtrl 定義授權 ctrl 的資料結構。
type AuthzCtrl struct {
	routes routeBinder
	svc    service.AuthzFacade
}

// RegisterRoutes 註冊此 controller 的 HTTP 路由。
func (c AuthzCtrl) RegisterRoutes(router *gin.RouterGroup) {
	authz := router.Group("/authz")
	authz.POST("/check", c.routes.Handle("iam.authz", "check", c.checkAuthz))
	authz.POST("/batch-check", c.routes.Handle("iam.authz", "check", c.batchCheckAuthz))
	authz.POST("/explain", c.routes.Handle("iam.authz", "explain", c.explainAuthz))
	authz.POST("/simulate", c.routes.Handle("iam.authz", "simulate", c.simulateAuthz))
}

// authorize 授權目前流程。
func (a *API) authorize(ctx domain.RequestContext, r *http.Request, routePath, resource, action string, authz routeAuthz) error {
	req := domain.CheckRequest{Resource: resource, Action: domain.Action(action), RouteMethod: r.Method, RoutePath: routePath}
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
	return nil
}

// apiAuthzReasonCode 處理 API 授權 reason 碼。
func apiAuthzReasonCode(result domain.CheckResult) string {
	switch result.Reason {
	case "missing permission":
		switch result.Action {
		case domain.ActionRead:
			return "menu_denied"
		case domain.ActionCreate, domain.ActionUpdate, domain.ActionDelete, domain.ActionExport, domain.ActionImport, domain.ActionInvite, domain.ActionApprove, domain.ActionUpdateStatus, domain.ActionStatusTransition:
			return "button_denied"
		default:
			return "permission_missing"
		}
	default:
		return "permission_missing"
	}
}

// checkAuthz 處理授權的 HTTP 請求。
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

// explainAuthz 處理授權的 HTTP 請求。
func (c AuthzCtrl) explainAuthz(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	var input domain.CheckRequest
	if err := readJSON(w, r, &input); err != nil {
		return err
	}
	result, err := c.svc.Explain(ctx, input)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, result)
	return nil
}

// simulateAuthz 處理授權的 HTTP 請求。
func (c AuthzCtrl) simulateAuthz(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	var input domain.AuthzSimulationRequest
	if err := readJSON(w, r, &input); err != nil {
		return err
	}
	result, err := c.svc.Simulate(ctx, input)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, result)
	return nil
}

// batchCheckAuthz 處理批次 check 授權的 HTTP 請求。
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
