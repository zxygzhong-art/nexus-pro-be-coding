package models

import "gorm.io/datatypes"

// Tenant is the global isolation boundary. Not tenant-scoped itself.
type Tenant struct {
	BaseModel
	Name              string `json:"name"`
	Status            string `json:"status"`
	PermissionVersion int64  `json:"permission_version"`
}

func (Tenant) TableName() string { return "iam_tenants" }

// Application is a registered application/domain (hr, iam, ...). Global registry.
type Application struct {
	BaseModel
	ApplicationCode string         `gorm:"column:application_code" json:"application_code"`
	Name            string         `json:"name"`
	Status          string         `json:"status"`
	ResourceTypes   datatypes.JSON `gorm:"column:resource_types" json:"resource_types"`
	Actions         datatypes.JSON `json:"actions"`
}

func (Application) TableName() string { return "iam_applications" }
