package postgres_test

import (
	"context"
	"testing"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/utils/jsoncodec"
	"nexus-pro-be/internal/utils/tenantctx"
)

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

func TestTenantIDFromContext(t *testing.T) {
	ctx := tenantctx.WithTenantID(context.Background(), "tenant-from-context")

	if got := tenantctx.TenantIDFromContext(ctx); got != "tenant-from-context" {
		t.Fatalf("TenantIDFromContext() = %q, want tenant-from-context", got)
	}
}

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
