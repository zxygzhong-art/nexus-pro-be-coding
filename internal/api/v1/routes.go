package v1

import "github.com/gin-gonic/gin"

type routeBinder interface {
	Handle(resource, action string, next HandlerFunc, options ...RouteOption) gin.HandlerFunc
}

const PathParamID = "id"

type apiRouteBinder struct {
	api *API
}

type routeAuthz struct {
	pathParams            []string
	resourceIDParam       string
	targetEmployeeIDParam string
}

type RouteOption func(*routeAuthz)

func ResourceID(param string) RouteOption {
	return func(cfg *routeAuthz) {
		cfg.resourceIDParam = param
		cfg.pathParams = appendPathParam(cfg.pathParams, param)
	}
}

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

func (r apiRouteBinder) Handle(resource, action string, next HandlerFunc, options ...RouteOption) gin.HandlerFunc {
	return r.api.ginHandle(resource, action, next, routeAuthzFrom(options))
}

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
	WorkflowCtrl{routes: routes, svc: a.workflow}.RegisterRoutes(v1)
	AgentCtrl{routes: routes, svc: a.agent}.RegisterRoutes(v1)
	AuditCtrl{routes: routes, svc: a.audit}.RegisterRoutes(v1)
}
