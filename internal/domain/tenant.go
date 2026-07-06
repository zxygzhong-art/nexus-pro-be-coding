package domain

import "time"

// Tenant 定義租戶的資料結構。
type Tenant struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}
