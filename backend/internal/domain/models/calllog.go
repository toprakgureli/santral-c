package models

import "time"

// CallLog is one call the panel's softphone handled, created when the call
// starts and finalized when it ends.
type CallLog struct {
	ID              uint       `gorm:"column:id;primarykey"`
	CallID          string     `gorm:"column:call_id;size:80;not null;uniqueIndex"`
	UserID          *uint      `gorm:"column:user_id;index"`
	Direction       string     `gorm:"column:direction;size:10;not null"`
	PeerNumber      string     `gorm:"column:peer_number;size:40;not null"`
	PeerKey         string     `gorm:"column:peer_key;size:20;not null;index"`
	Disposition     string     `gorm:"column:disposition;size:16;not null;default:in_progress"`
	StartedAt       time.Time  `gorm:"column:started_at"`
	AnsweredAt      *time.Time `gorm:"column:answered_at"`
	EndedAt         *time.Time `gorm:"column:ended_at"`
	DurationSeconds int        `gorm:"column:duration_seconds;not null;default:0"`
	CreatedAt       time.Time  `gorm:"column:created_at"`
	UpdatedAt       time.Time  `gorm:"column:updated_at"`
}

// TableName pins the table name.
func (CallLog) TableName() string { return "call_logs" }

// AgentPresence is a persisted per-agent presence state.
type AgentPresence struct {
	UserID    uint      `gorm:"column:user_id;primarykey"`
	State     string    `gorm:"column:state;size:16;not null;default:available"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

// TableName pins the table name.
func (AgentPresence) TableName() string { return "agent_presence" }
