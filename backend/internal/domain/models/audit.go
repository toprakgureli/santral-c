package models

import "time"

// AuditLog records a privileged action, including invisible-admin actions.
type AuditLog struct {
	ID         uint      `gorm:"primarykey"`
	ActorID    *uint     `gorm:"index"`
	Action     string    `gorm:"size:80;not null;index"`
	TargetType string    `gorm:"size:60"`
	TargetID   string    `gorm:"size:64"`
	IP         string    `gorm:"size:45"`
	Detail     string    `gorm:"type:jsonb;not null;default:'{}'"`
	CreatedAt  time.Time `gorm:"index"`
}
