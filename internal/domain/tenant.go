package domain

import "time"

// Tenant is the top-level isolation boundary for business data.
type Tenant struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}
