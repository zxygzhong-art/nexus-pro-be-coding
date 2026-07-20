package postgres_test

import (
	"context"
	"testing"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/utils/jsoncodec"
	"nexus-pro-api/internal/utils/tenantctx"
)

// TestTenantIDFromArgs 驗證租戶 ID 來源 args。
func TestTenantIDFromArgs(t *testing.T) {
	type params struct {
		TenantID string
		ID       string
	}
	tests := []struct {
		name string
		args []interface{}
		want string
	}{
		{name: "bare string ignored", args: []interface{}{"tenant-1"}, want: ""},
		{name: "params struct", args: []interface{}{params{TenantID: "tenant-2", ID: "x"}}, want: "tenant-2"},
		{name: "params pointer", args: []interface{}{&params{TenantID: "tenant-3", ID: "x"}}, want: "tenant-3"},
		{name: "empty", args: []interface{}{params{ID: "x"}}, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tenantctx.TenantIDFromArgs(tt.args); got != tt.want {
				t.Fatalf("TenantIDFromArgs() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestTenantIDFromContext 驗證租戶 ID 來源 context。
func TestTenantIDFromContext(t *testing.T) {
	ctx := tenantctx.WithTenantID(context.Background(), "tenant-from-context")

	if got := tenantctx.TenantIDFromContext(ctx); got != "tenant-from-context" {
		t.Fatalf("TenantIDFromContext() = %q, want tenant-from-context", got)
	}
}

// TestCompanyIDFromArgs 驗證公司 ID 來源 args。
func TestCompanyIDFromArgs(t *testing.T) {
	type intParams struct {
		CompanyID int
		ID        string
	}
	type stringParams struct {
		CompanyID string
		ID        string
	}
	tests := []struct {
		name string
		args []interface{}
		want string
	}{
		{name: "int company id", args: []interface{}{intParams{CompanyID: 42, ID: "x"}}, want: "42"},
		{name: "string company id", args: []interface{}{stringParams{CompanyID: "company-7", ID: "x"}}, want: "company-7"},
		{name: "params pointer", args: []interface{}{&intParams{CompanyID: 99, ID: "x"}}, want: "99"},
		{name: "zero int omitted", args: []interface{}{intParams{ID: "x"}}, want: ""},
		{name: "nil pointer ignored", args: []interface{}{(*intParams)(nil), intParams{CompanyID: 8}}, want: "8"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tenantctx.CompanyIDFromArgs(tt.args); got != tt.want {
				t.Fatalf("CompanyIDFromArgs() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestSystemTaskFromContext 驗證 system 任務 來源 context。
func TestSystemTaskFromContext(t *testing.T) {
	if tenantctx.SystemTaskFromContext(context.Background()) {
		t.Fatal("expected plain context not to be a system task")
	}
	ctx := tenantctx.WithSystemTask(context.Background())
	if !tenantctx.SystemTaskFromContext(ctx) {
		t.Fatal("expected system task context to be detected")
	}
	if tenantID := tenantctx.TenantIDFromContext(ctx); tenantID != "" {
		t.Fatalf("expected system task context to carry no tenant id, got %q", tenantID)
	}
}

// TestCompanyIDFromContext 驗證公司 ID 來源 context。
func TestCompanyIDFromContext(t *testing.T) {
	ctx := tenantctx.WithCompanyID(context.Background(), "42")

	if got := tenantctx.CompanyIDFromContext(ctx); got != "42" {
		t.Fatalf("CompanyIDFromContext() = %q, want 42", got)
	}
}

// TestJSONCodecsDoNotPanicOnInvalidPayload 驗證 JSON codecs do not panic on 無效 payload。
func TestJSONCodecsDoNotPanicOnInvalidPayload(t *testing.T) {
	invalid := []byte("{")
	if got := jsoncodec.Map(invalid); got != nil {
		t.Fatalf("expected invalid map payload to decode to nil, got %+v", got)
	}
	if got := jsoncodec.Permissions(invalid); got != nil {
		t.Fatalf("expected invalid permissions payload to decode to nil, got %+v", got)
	}
	if string(jsoncodec.Must([]domain.Permission(nil))) != "[]" {
		t.Fatalf("expected nil permission slice to marshal as []")
	}
}
