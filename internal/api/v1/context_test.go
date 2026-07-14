package v1

import (
	"testing"

	"nexus-pro-be/internal/domain"
)

// TestAuthenticatedPlatformAdminAcceptsDedicatedClaim 驗證專用布林 claim 可建立平台管理員身分。
func TestAuthenticatedPlatformAdminAcceptsDedicatedClaim(t *testing.T) {
	principal := domain.AuthenticatedPrincipal{Claims: map[string]any{"platform_admin": true}}
	if !authenticatedPlatformAdmin(principal) {
		t.Fatal("expected platform_admin claim to authorize platform registry writes")
	}
}

// TestAuthenticatedPlatformAdminRequiresDedicatedRealmRole 驗證租戶管理角色不會被提升為平台管理員。
func TestAuthenticatedPlatformAdminRequiresDedicatedRealmRole(t *testing.T) {
	admin := domain.AuthenticatedPrincipal{Claims: map[string]any{
		"realm_access": map[string]any{"roles": []any{"nexus-platform-admin"}},
	}}
	if !authenticatedPlatformAdmin(admin) {
		t.Fatal("expected dedicated platform realm role to authorize platform registry writes")
	}

	tenantAdmin := domain.AuthenticatedPrincipal{Claims: map[string]any{
		"realm_access": map[string]any{"roles": []any{"admin", "tenant-admin"}},
	}}
	if authenticatedPlatformAdmin(tenantAdmin) {
		t.Fatal("expected tenant-local admin roles to remain insufficient")
	}
}
