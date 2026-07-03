package v1

import "github.com/gin-gonic/gin"

type routeBinder interface {
	Handle(resource, action string, next HandlerFunc, options ...RouteOption) gin.HandlerFunc
}

// PathParamID is the conventional path parameter name for resource identifiers.
const PathParamID = "id"

type apiRouteBinder struct {
	api *API
}

type routeAuthz struct {
	pathParams            []string
	resourceIDParam       string
	targetEmployeeIDParam string
}

// RouteOption adjusts how route parameters are passed into authorization checks.
type RouteOption func(*routeAuthz)

// ResourceID marks a path parameter as the protected resource identifier.
func ResourceID(param string) RouteOption {
	return func(cfg *routeAuthz) {
		cfg.resourceIDParam = param
		cfg.pathParams = appendPathParam(cfg.pathParams, param)
	}
}

// PathParam exposes a route parameter to handlers without using it as an authz target.
func PathParam(param string) RouteOption {
	return func(cfg *routeAuthz) {
		cfg.pathParams = appendPathParam(cfg.pathParams, param)
	}
}

// TargetEmployeeID marks a path parameter as the employee target for scoped HR checks.
func TargetEmployeeID(param string) RouteOption {
	return func(cfg *routeAuthz) {
		cfg.targetEmployeeIDParam = param
		cfg.pathParams = appendPathParam(cfg.pathParams, param)
	}
}

func appendPathParam(params []string, param string) []string {
	for _, existing := range params {
		if existing == param {
			return params
		}
	}
	return append(params, param)
}

func routeAuthzFrom(options []RouteOption) routeAuthz {
	cfg := routeAuthz{}
	for _, option := range options {
		if option != nil {
			option(&cfg)
		}
	}
	return cfg
}

// Handle wraps a handler with route-level authorization metadata.
func (r apiRouteBinder) Handle(resource, action string, next HandlerFunc, options ...RouteOption) gin.HandlerFunc {
	return r.api.ginHandle(resource, action, next, routeAuthzFrom(options))
}

// RegisterRoutes attaches health, Swagger, and all v1 route groups.
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
	AuditCtrl{routes: routes, svc: a.audit}.RegisterRoutes(v1)
}
