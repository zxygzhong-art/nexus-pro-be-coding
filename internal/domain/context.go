package domain

import "context"

// RequestContext 定義請求 context 的資料結構。
type RequestContext struct {
	Context              context.Context
	TenantID             string
	AccountID            string
	PlatformAdmin        bool
	AssumedRoleID        string
	AssumedRoleSessionID string
	RouteApplicationCode string
	RouteResourceType    string
	RouteAction          string
	RoutePath            string
	RequestID            string
	IdempotencyKey       string
	TraceID              string
	SpanID               string
}
