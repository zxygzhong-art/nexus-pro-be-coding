package tenantctx

import (
	"context"
	"reflect"
	"strconv"
)

type tenantIDContextKey struct{}
type companyIDContextKey struct{}

// WithTenantID returns a context carrying the tenant ID used by repository adapters.
func WithTenantID(ctx context.Context, tenantID string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if tenantID == "" {
		return ctx
	}
	return context.WithValue(ctx, tenantIDContextKey{}, tenantID)
}

// TenantIDFromContext returns the tenant ID stored in ctx, if any.
func TenantIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	tenantID, _ := ctx.Value(tenantIDContextKey{}).(string)
	return tenantID
}

// TenantIDFromArgs extracts the first tenant_id string from sqlc argument structs.
func TenantIDFromArgs(args []interface{}) string {
	for _, arg := range args {
		if tenantID := tenantIDFromArg(arg); tenantID != "" {
			return tenantID
		}
	}
	return ""
}

// WithCompanyID returns a context carrying the company ID used by RLS policies.
func WithCompanyID(ctx context.Context, companyID string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if companyID == "" {
		return ctx
	}
	return context.WithValue(ctx, companyIDContextKey{}, companyID)
}

// CompanyIDFromContext returns the company ID stored in ctx, if any.
func CompanyIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	companyID, _ := ctx.Value(companyIDContextKey{}).(string)
	return companyID
}

// CompanyIDFromArgs extracts the first company_id value from sqlc argument structs.
func CompanyIDFromArgs(args []interface{}) string {
	for _, arg := range args {
		if companyID := companyIDFromArg(arg); companyID != "" {
			return companyID
		}
	}
	return ""
}

func tenantIDFromArg(arg interface{}) string {
	return stringFieldFromArg(arg, "TenantID")
}

func companyIDFromArg(arg interface{}) string {
	return stringFieldFromArg(arg, "CompanyID")
}

func stringFieldFromArg(arg interface{}, fieldName string) string {
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
	field := v.FieldByName(fieldName)
	if !field.IsValid() {
		return ""
	}
	switch field.Kind() {
	case reflect.String:
		return field.String()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		value := field.Int()
		if value == 0 {
			return ""
		}
		return strconv.FormatInt(value, 10)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		value := field.Uint()
		if value == 0 {
			return ""
		}
		return strconv.FormatUint(value, 10)
	default:
		return ""
	}
}
