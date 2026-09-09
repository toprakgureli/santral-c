package models

import "time"

// SystemSetting is a key/value runtime flag.
type SystemSetting struct {
	Key       string `gorm:"primaryKey;size:60"`
	Value     string `gorm:"size:200;not null;default:''"`
	UpdatedAt time.Time
}

// TableName pins the table name.
func (SystemSetting) TableName() string { return "system_settings" }
