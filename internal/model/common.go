package model

import "time"

// CreatedOnly is embedded by append-only records that need a creation stamp.
type CreatedOnly struct {
	CreatedAt time.Time `gorm:"autoCreateTime" column:"created_at" json:"created_at"`
}

// Common is embedded by mutable records that need creation and update stamps.
type Common struct {
	CreatedAt time.Time `gorm:"autoCreateTime" column:"created_at" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" column:"updated_at" json:"updated_at"`
}
