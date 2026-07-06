package v1

import "github.com/gin-gonic/gin"

type routeBinder interface {
	Handle(resource, action string, next HandlerFunc, options ...RouteOption) gin.HandlerFunc
}

// PathParamID 定義 path param ID 的固定值。
const PathParamID = "id"

type apiRouteBinder struct {
	api *API
}

type routeAuthz struct {
	pathParams            []string
	resourceIDParam       string
	targetEmployeeIDParam string
}

// RouteOption 表示路由選項。
type RouteOption func(*routeAuthz)

// ResourceID 處理 resource ID。
func ResourceID(param string) RouteOption {
	return func(cfg *routeAuthz) {
		cfg.resourceIDParam = param
		cfg.pathParams = appendPathParam(cfg.pathParams, param)
	}
}

// PathParam 處理 path param。
func PathParam(param string) RouteOption {
	return func(cfg *routeAuthz) {
		cfg.pathParams = appendPathParam(cfg.pathParams, param)
	}
}

// TargetEmployeeID 處理 target 員工 ID。
func TargetEmployeeID(param string) RouteOption {
	return func(cfg *routeAuthz) {
		cfg.targetEmployeeIDParam = param
		cfg.pathParams = appendPathParam(cfg.pathParams, param)
	}
}

// appendPathParam 附加 path param。
func appendPathParam(params []string, param string) []string {
	for _, existing := range params {
		if existing == param {
			return params
		}
	}
	return append(params, param)
}

// routeAuthzFrom 處理路由授權 from。
func routeAuthzFrom(options []RouteOption) routeAuthz {
	cfg := routeAuthz{}
	for _, option := range options {
		if option != nil {
			option(&cfg)
		}
	}
	return cfg
}

// Handle 處理 handle。
func (r apiRouteBinder) Handle(resource, action string, next HandlerFunc, options ...RouteOption) gin.HandlerFunc {
	return r.api.ginHandle(resource, action, next, routeAuthzFrom(options))
}

// RegisterRoutes 註冊路由。
func (a *API) RegisterRoutes(router *gin.Engine) {
	routes := apiRouteBinder{api: a}
	SwaggerCtrl{}.RegisterRoutes(router)
	HealthCtrl{readinessChecks: a.readinessChecks}.RegisterRoutes(router)

	v1 := router.Group("/v1")
	MeCtrl{routes: routes, svc: a.me}.RegisterRoutes(v1)
	AuthzCtrl{routes: routes, svc: a.authz}.RegisterRoutes(v1)
	IAMCtrl{routes: routes, svc: a.iam}.RegisterRoutes(v1)
	HRCtrl{routes: routes, svc: a.hr}.RegisterRoutes(v1)
	AttendanceCtrl{routes: routes, svc: a.attendance}.RegisterRoutes(v1)
	PlatformCtrl{routes: routes, svc: a.platform}.RegisterRoutes(v1)
	WorkspaceCtrl{routes: routes, svc: a.workspace}.RegisterRoutes(v1)
	WorkflowCtrl{routes: routes, svc: a.workflow}.RegisterRoutes(v1)
	AgentCtrl{routes: routes, svc: a.agent}.RegisterRoutes(v1)
	NotificationCtrl{routes: routes, svc: a.notification}.RegisterRoutes(v1)
	AuditCtrl{routes: routes, svc: a.audit}.RegisterRoutes(v1)
}
