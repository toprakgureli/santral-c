package models

import "time"

// Permission is a fine-grained capability row.
type Permission struct {
	ID          uint   `gorm:"primarykey"`
	Key         string `gorm:"size:100;not null;uniqueIndex"`
	Module      string `gorm:"size:60;not null;index"`
	Description string `gorm:"size:255"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}
