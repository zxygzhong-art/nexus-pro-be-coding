package tenantctx

import (
	"context"
	"reflect"
	"strconv"
)

type tenantIDContextKey struct{}
type companyIDContextKey struct{}
type systemTaskContextKey struct{}

// WithTenantID 附加租戶 ID。
func WithTenantID(ctx context.Context, tenantID string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if tenantID == "" {
		return ctx
	}
	return context.WithValue(ctx, tenantIDContextKey{}, tenantID)
}

// TenantIDFromContext 處理租戶 ID 來源 context。
func TenantIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	tenantID, _ := ctx.Value(tenantIDContextKey{}).(string)
	return tenantID
}

// TenantIDFromArgs 處理租戶 ID 來源 args。
func TenantIDFromArgs(args []interface{}) string {
	for _, arg := range args {
		if tenantID := tenantIDFromArg(arg); tenantID != "" {
			return tenantID
		}
	}
	return ""
}

// WithCompanyID 附加公司 ID。
func WithCompanyID(ctx context.Context, companyID string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if companyID == "" {
		return ctx
	}
	return context.WithValue(ctx, companyIDContextKey{}, companyID)
}

// CompanyIDFromContext 處理公司 ID 來源 context。
func CompanyIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	companyID, _ := ctx.Value(companyIDContextKey{}).(string)
	return companyID
}

// WithSystemTask 附加 system 任務。
func WithSystemTask(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, systemTaskContextKey{}, true)
}

// SystemTaskFromContext 處理 system 任務 來源 context。
func SystemTaskFromContext(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	systemTask, _ := ctx.Value(systemTaskContextKey{}).(bool)
	return systemTask
}

// CompanyIDFromArgs 處理公司 ID 來源 args。
func CompanyIDFromArgs(args []interface{}) string {
	for _, arg := range args {
		if companyID := companyIDFromArg(arg); companyID != "" {
			return companyID
		}
	}
	return ""
}

// tenantIDFromArg 處理租戶 ID 來源 arg。
func tenantIDFromArg(arg interface{}) string {
	return stringFieldFromArg(arg, "TenantID")
}

// companyIDFromArg 處理公司 ID 來源 arg。
func companyIDFromArg(arg interface{}) string {
	return stringFieldFromArg(arg, "CompanyID")
}

// stringFieldFromArg 處理字串欄位 來源 arg。
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
