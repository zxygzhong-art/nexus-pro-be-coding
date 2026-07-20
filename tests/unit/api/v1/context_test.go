package v1_test

import (
	"testing"

	v1 "nexus-pro-api/internal/api/v1"
	"nexus-pro-api/internal/domain"
)

// TestAuthenticatedPlatformAdminAcceptsDedicatedClaim 驗證專用布林 claim 可建立平臺管理員身分。
func TestAuthenticatedPlatformAdminAcceptsDedicatedClaim(t *testing.T) {
	principal := domain.AuthenticatedPrincipal{Claims: map[string]any{"platform_admin": true}}
	if !v1.AuthenticatedPlatformAdmin(principal) {
		t.Fatal("expected platform_admin claim to authorize platform registry writes")
	}
}

// TestAuthenticatedPlatformAdminRequiresDedicatedRealmRole 驗證租戶管理角色不會被提升為平臺管理員。
func TestAuthenticatedPlatformAdminRequiresDedicatedRealmRole(t *testing.T) {
	admin := domain.AuthenticatedPrincipal{Claims: map[string]any{
		"realm_access": map[string]any{"roles": []any{"nexus-platform-admin"}},
	}}
	if !v1.AuthenticatedPlatformAdmin(admin) {
		t.Fatal("expected dedicated platform realm role to authorize platform registry writes")
	}

	tenantAdmin := domain.AuthenticatedPrincipal{Claims: map[string]any{
		"realm_access": map[string]any{"roles": []any{"admin", "tenant-admin"}},
	}}
	if v1.AuthenticatedPlatformAdmin(tenantAdmin) {
		t.Fatal("expected tenant-local admin roles to remain insufficient")
	}
}
