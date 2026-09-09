package models

import (
	"time"

	"gorm.io/gorm"
)

// Role is a named bundle of permissions.
type Role struct {
	ID          uint         `gorm:"primarykey"`
	Name        string       `gorm:"size:60;not null;uniqueIndex"`
	DisplayName string       `gorm:"size:120;not null"`
	Description string       `gorm:"size:255"`
	Permissions []Permission `gorm:"many2many:role_permissions"`
	System      bool         `gorm:"not null;default:false"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
	DeletedAt   gorm.DeletedAt `gorm:"index"`
}
