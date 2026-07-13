package service

import (
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// startServiceSpan 啟動服務 span。
func startServiceSpan(ctx RequestContext, name string, attrs ...attribute.KeyValue) (RequestContext, trace.Span) {
	base := goContext(ctx)
	baseAttrs := make([]attribute.KeyValue, 0, len(attrs)+3)
	if ctx.TenantID != "" {
		baseAttrs = append(baseAttrs, attribute.String("tenant.id", ctx.TenantID))
	}
	if ctx.AccountID != "" {
		baseAttrs = append(baseAttrs, attribute.String("account.id", ctx.AccountID))
	}
	if ctx.RequestID != "" {
		baseAttrs = append(baseAttrs, attribute.String("request.id", ctx.RequestID))
	}
	baseAttrs = append(baseAttrs, attrs...)

	next, span := otel.Tracer("nexus-pro-be/internal/service").Start(base, name,
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(baseAttrs...),
	)
	ctx.Context = next
	return ctx, span
}

// finishServiceSpan 處理 finish 服務 span。
func finishServiceSpan(span trace.Span, err error) {
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	span.End()
}

// authzSpanAttributes 處理授權 span attributes。
func authzSpanAttributes(req CheckRequest) []attribute.KeyValue {
	attrs := []attribute.KeyValue{
		attribute.String("authz.action", string(req.Action)),
	}
	if req.Resource != "" {
		attrs = append(attrs, attribute.String("authz.resource", req.Resource))
	}
	if req.ApplicationCode != "" {
		attrs = append(attrs, attribute.String("authz.application_code", string(req.ApplicationCode)))
	}
	if req.ResourceType != "" {
		attrs = append(attrs, attribute.String("authz.resource_type", string(req.ResourceType)))
	}
	if req.ResourceID != "" {
		attrs = append(attrs, attribute.String("authz.resource_id", req.ResourceID))
	}
	if req.TargetEmployeeID != "" {
		attrs = append(attrs, attribute.String("authz.target_employee_id", req.TargetEmployeeID))
	}
	if req.RouteMethod != "" {
		attrs = append(attrs, attribute.String("authz.route_method", req.RouteMethod))
	}
	if req.RoutePath != "" {
		attrs = append(attrs, attribute.String("authz.route_path", req.RoutePath))
	}
	return attrs
}

// setAuthzSpanResult 處理集合授權 span 結果。
func setAuthzSpanResult(span trace.Span, result CheckResult) {
	if result.Action == "" && result.Resource == "" && result.ApplicationCode == "" && result.ResourceType == "" {
		return
	}
	attrs := []attribute.KeyValue{
		attribute.Bool("authz.allowed", result.Allowed),
		attribute.String("authz.reason", result.Reason),
	}
	if result.EffectiveScope != "" {
		attrs = append(attrs, attribute.String("authz.effective_scope", string(result.EffectiveScope)))
	}
	if result.RiskLevel != "" {
		attrs = append(attrs, attribute.String("authz.risk_level", result.RiskLevel))
	}
	span.SetAttributes(attrs...)
}
