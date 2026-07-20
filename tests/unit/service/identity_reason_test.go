package service_test

import (
	"context"
	"testing"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/repository/memory"
	"nexus-pro-api/internal/service"
)

// TestUnlinkedIdentityErrorsExposeStableReasonCode keeps every identity-resolution entrypoint message-independent.
func TestUnlinkedIdentityErrorsExposeStableReasonCode(t *testing.T) {
	identity := service.New(memory.NewStore()).Identity()
	tests := []struct {
		name    string
		resolve func() error
	}{
		{
			name: "tenant-scoped principal",
			resolve: func() error {
				_, err := identity.ResolveAuthenticatedPrincipal(context.Background(), domain.AuthenticatedPrincipal{
					Provider: "keycloak",
					Subject:  "unlinked-subject",
					TenantID: "tenant-1",
				})
				return err
			},
		},
		{
			name: "bound principal",
			resolve: func() error {
				_, err := identity.ResolveBoundAuthenticatedPrincipal(context.Background(), domain.AuthenticatedPrincipal{
					Provider: "keycloak",
					Subject:  "unlinked-subject",
					TenantID: "tenant-1",
				})
				return err
			},
		},
		{
			name: "subject lookup without tenant claim",
			resolve: func() error {
				_, err := identity.ResolveAuthenticatedPrincipal(context.Background(), domain.AuthenticatedPrincipal{
					Provider: "keycloak",
					Subject:  "unlinked-subject",
				})
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			appErr, ok := domain.AsAppError(tt.resolve())
			if !ok {
				t.Fatal("expected an AppError")
			}
			if appErr.Status != 401 || appErr.NumericCode() != domain.ErrorCodeUnauthorized || appErr.ReasonCode != "identity_not_linked" {
				t.Fatalf("expected stable unlinked identity error, got %+v", appErr)
			}
		})
	}
}
