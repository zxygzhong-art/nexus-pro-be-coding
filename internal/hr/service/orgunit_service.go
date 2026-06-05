package service

import (
	"context"

	"git.corp.ikala.tv/nexus-pro/nexus-pro-be/internal/models"
	"git.corp.ikala.tv/nexus-pro/nexus-pro-be/internal/platform/apperror"
)

// OrgUnitService will own organization-tree read/write operations.
type OrgUnitService struct{}

// NewOrgUnitService builds the service.
func NewOrgUnitService() *OrgUnitService { return &OrgUnitService{} }

// List returns the organization tree in the caller's data scope.
// TODO: query hr_org_units within tenant tx and build the tree.
func (s *OrgUnitService) List(_ context.Context) ([]models.OrgUnit, error) {
	return nil, apperror.NotImplemented("hr org unit listing not implemented in the foundation milestone")
}
