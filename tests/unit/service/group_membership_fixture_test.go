package service_test

import (
	"context"
	"testing"
	"time"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/repository/memory"
)

// seedActiveGroupMembership records the authoritative relation used by authorization tests.
func seedActiveGroupMembership(t *testing.T, store *memory.Store, tenantID, groupID, accountID string, at time.Time) {
	t.Helper()
	if err := store.UpsertGroupMembership(context.Background(), domain.GroupMembership{
		ID: "ugm-" + groupID + "-" + accountID, TenantID: tenantID, UserGroupID: groupID,
		AccountID: accountID, ValidFrom: at, Source: "manual", CreatedAt: at,
	}); err != nil {
		t.Fatal(err)
	}
}
