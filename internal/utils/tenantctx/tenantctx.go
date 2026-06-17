package tenantctx

import (
	"context"
	"reflect"
)

type tenantIDContextKey struct{}

func WithTenantID(ctx context.Context, tenantID string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if tenantID == "" {
		return ctx
	}
	return context.WithValue(ctx, tenantIDContextKey{}, tenantID)
}

func TenantIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	tenantID, _ := ctx.Value(tenantIDContextKey{}).(string)
	return tenantID
}

func TenantIDFromArgs(args []interface{}) string {
	for _, arg := range args {
		if tenantID := tenantIDFromArg(arg); tenantID != "" {
			return tenantID
		}
	}
	return ""
}

func tenantIDFromArg(arg interface{}) string {
	if arg == nil {
		return ""
	}
	v := reflect.ValueOf(arg)
	for v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return ""
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return ""
	}
	field := v.FieldByName("TenantID")
	if !field.IsValid() || field.Kind() != reflect.String {
		return ""
	}
	return field.String()
}
