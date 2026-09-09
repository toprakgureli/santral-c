package models

import "time"

// Session is a refresh-token session; only the token hash is stored.
type Session struct {
	ID         uint      `gorm:"primarykey"`
	UserID     uint      `gorm:"not null;index"`
	User       *User     `gorm:"foreignKey:UserID"`
	TokenHash  string    `gorm:"size:64;not null;uniqueIndex"`
	IP         string    `gorm:"size:45"`
	UserAgent  string    `gorm:"size:255"`
	ExpiresAt  time.Time `gorm:"not null;index"`
	RevokedAt  *time.Time
	LastUsedAt *time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// LoginAttempt records one authentication attempt.
type LoginAttempt struct {
	ID        uint      `gorm:"primarykey"`
	Email     string    `gorm:"size:255;not null;index"`
	UserID    *uint     `gorm:"index"`
	IP        string    `gorm:"size:45;not null;index"`
	UserAgent string    `gorm:"size:255"`
	Success   bool      `gorm:"not null;index"`
	Reason    string    `gorm:"size:60"`
	CreatedAt time.Time `gorm:"index"`
}

// IPBan is an active ban on a source IP.
type IPBan struct {
	ID        uint      `gorm:"primarykey"`
	IP        string    `gorm:"size:45;not null;uniqueIndex"`
	Reason    string    `gorm:"size:160"`
	Attempts  int       `gorm:"not null;default:0"`
	Until     time.Time `gorm:"not null;index"`
	CreatedAt time.Time
	UpdatedAt time.Time
}
