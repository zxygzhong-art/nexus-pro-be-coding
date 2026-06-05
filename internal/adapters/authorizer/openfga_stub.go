package authorizer

import (
	"context"

	"git.corp.ikala.tv/nexus-pro/nexus-pro-be/internal/authz"
	"git.corp.ikala.tv/nexus-pro/nexus-pro-be/internal/models"
)

// OpenFGAAuthorizer is a placeholder for a future ReBAC backend. The OpenFGA
// model lives at deploy/openfga/model.fga. Until wired up, it returns
// ErrNotImplemented so misconfiguration fails loudly rather than silently
// allowing. Selected via AUTHZ_BACKEND=openfga.
type OpenFGAAuthorizer struct {
	APIURL  string
	StoreID string
	ModelID string
}

// NewOpenFGAAuthorizer constructs the stub.
func NewOpenFGAAuthorizer(apiURL, storeID, modelID string) *OpenFGAAuthorizer {
	return &OpenFGAAuthorizer{APIURL: apiURL, StoreID: storeID, ModelID: modelID}
}

func (*OpenFGAAuthorizer) Check(context.Context, authz.Request) (authz.Decision, error) {
	return authz.Decision{}, ErrNotImplemented
}

func (*OpenFGAAuthorizer) BatchCheck(context.Context, []authz.Request) ([]authz.Decision, error) {
	return nil, ErrNotImplemented
}

func (*OpenFGAAuthorizer) PruneMenus(context.Context, string) ([]models.MenuItem, error) {
	return nil, ErrNotImplemented
}
