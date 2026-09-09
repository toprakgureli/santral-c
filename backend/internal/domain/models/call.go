package models

import "time"

// Call is one inbound, outbound or internal call, correlated to Asterisk by
// its linkedid.
type Call struct {
	ID              uint       `gorm:"column:id;primarykey"`
	Linkedid        string     `gorm:"column:linkedid;size:80;not null;uniqueIndex"`
	Direction       string     `gorm:"column:direction;size:10;not null;index"`
	Disposition     string     `gorm:"column:disposition;size:16;not null;default:in_progress;index"`
	FromNumber      string     `gorm:"column:from_number;size:32;not null;index"`
	ToNumber        string     `gorm:"column:to_number;size:32;not null;index"`
	FromUserID      *uint      `gorm:"column:from_user_id;index"`
	ToUserID        *uint      `gorm:"column:to_user_id;index"`
	ContactID       *uint      `gorm:"column:contact_id;index"`
	Trunk           string     `gorm:"column:trunk;size:60"`
	StartedAt       time.Time  `gorm:"column:started_at;index"`
	AnsweredAt      *time.Time `gorm:"column:answered_at"`
	EndedAt         *time.Time `gorm:"column:ended_at"`
	RingSeconds     int        `gorm:"column:ring_seconds;not null;default:0"`
	TalkSeconds     int        `gorm:"column:talk_seconds;not null;default:0"`
	HangupCauseCode *int       `gorm:"column:hangup_cause_code"`
	HangupCauseText string     `gorm:"column:hangup_cause_text;size:80"`
	HangupBy        *string    `gorm:"column:hangup_by;size:10"`
	RecordingPath   string     `gorm:"column:recording_path;size:255"`
	FromUser        *User      `gorm:"foreignKey:FromUserID"`
	ToUser          *User      `gorm:"foreignKey:ToUserID"`
	Contact         *Contact   `gorm:"foreignKey:ContactID"`
	CreatedAt       time.Time  `gorm:"column:created_at"`
	UpdatedAt       time.Time  `gorm:"column:updated_at"`
}
