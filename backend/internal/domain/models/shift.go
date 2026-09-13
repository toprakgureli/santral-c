package models

import "time"

// Shift is one work stretch of a user: opened by "mesai başlat", closed by
// "mesai bitir" or by the server at the evening cutoff.
type Shift struct {
	ID        uint       `gorm:"column:id;primarykey"`
	UserID    uint       `gorm:"column:user_id;not null;index"`
	StartedAt time.Time  `gorm:"column:started_at"`
	EndedAt   *time.Time `gorm:"column:ended_at"`
	EndedBy   *string    `gorm:"column:ended_by;size:8"`
	CreatedAt time.Time  `gorm:"column:created_at"`
}

// TableName pins the table name.
func (Shift) TableName() string { return "shifts" }
