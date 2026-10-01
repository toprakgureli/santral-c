package models

import "time"

// BackupSettings is the one row of database backup settings.
type BackupSettings struct {
	ID             uint   `gorm:"column:id;primarykey"`
	Enabled        bool   `gorm:"column:enabled"`
	FolderID       string `gorm:"column:folder_id"`
	CredentialsEnc string `gorm:"column:credentials_enc"`
	Account        string `gorm:"column:account"`
	UpdatedAt      time.Time
}

// TableName pins the table name.
func (BackupSettings) TableName() string { return "backup_settings" }

// BackupRun is one database backup.
type BackupRun struct {
	ID         uint       `gorm:"column:id;primarykey"`
	StartedAt  time.Time  `gorm:"column:started_at"`
	FinishedAt *time.Time `gorm:"column:finished_at"`
	OK         bool       `gorm:"column:ok"`
	File       string     `gorm:"column:file"`
	Size       int64      `gorm:"column:size"`
	Error      string     `gorm:"column:error"`
	StartedBy  *uint      `gorm:"column:started_by"`
}

// TableName pins the table name.
func (BackupRun) TableName() string { return "backup_runs" }
