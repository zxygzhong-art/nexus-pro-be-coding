// Package models holds the GORM structs mapping to the iam_* tables. IDs are
// application-assigned (prefixed) strings, so auto-increment is disabled and the
// service layer assigns ids. Each model declares an explicit TableName.
package models

import (
	"time"

	"gorm.io/gorm"
)

// BaseModel is embedded by every model: a text PK plus timestamps.
type BaseModel struct {
	ID        string    `gorm:"primaryKey;type:text" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// SoftDelete adds GORM soft-delete to configuration tables.
type SoftDelete struct {
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

// TenantModel is embedded by tenant-scoped models. RLS enforces isolation at the
// database layer; the TenantID column is also used for explicit WHERE filters.
type TenantModel struct {
	BaseModel
	TenantID string `gorm:"type:text;index;not null" json:"tenant_id"`
}
