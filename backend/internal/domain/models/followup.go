package models

import "time"

// UserNotice is a notice for one person, shown on any page until read.
type UserNotice struct {
	ID        uint       `gorm:"column:id;primarykey"`
	UserID    uint       `gorm:"column:user_id"`
	Kind      string     `gorm:"column:kind"`
	Text      string     `gorm:"column:text"`
	Link      string     `gorm:"column:link"`
	CreatedAt time.Time  `gorm:"column:created_at"`
	ReadAt    *time.Time `gorm:"column:read_at"`
}

// TableName pins the table name.
func (UserNotice) TableName() string { return "user_notices" }

// CallUnreached is a number someone called and could not reach, open until
// anyone has a real conversation with it or nobody needs to call back.
type CallUnreached struct {
	ID               uint       `gorm:"column:id;primarykey"`
	PeerKey          string     `gorm:"column:peer_key"`
	PeerNumber       string     `gorm:"column:peer_number"`
	UserID           uint       `gorm:"column:user_id"`
	Attempts         int        `gorm:"column:attempts"`
	FirstAt          time.Time  `gorm:"column:first_at"`
	LastAt           time.Time  `gorm:"column:last_at"`
	LastCallID       string     `gorm:"column:last_call_id"`
	LastReason       string     `gorm:"column:last_reason"`
	Status           string     `gorm:"column:status"`
	ReachedAt        *time.Time `gorm:"column:reached_at"`
	ReachedBy        *uint      `gorm:"column:reached_by"`
	ReachedDirection *string    `gorm:"column:reached_direction"`
	ReachedSeconds   *int       `gorm:"column:reached_seconds"`
	ReachedCallID    *string    `gorm:"column:reached_call_id"`
	ClaimedBy        *uint      `gorm:"column:claimed_by"`
	ClaimedUntil     *time.Time `gorm:"column:claimed_until"`
	DroppedBy        *uint      `gorm:"column:dropped_by"`
	DroppedAt        *time.Time `gorm:"column:dropped_at"`
	CreatedAt        time.Time  `gorm:"column:created_at"`
	UpdatedAt        time.Time  `gorm:"column:updated_at"`
}

// TableName pins the table name.
func (CallUnreached) TableName() string { return "call_unreached" }

// CallReminder is a call back someone planned for a time.
type CallReminder struct {
	ID         uint       `gorm:"column:id;primarykey"`
	UserID     uint       `gorm:"column:user_id"`
	PeerNumber string     `gorm:"column:peer_number"`
	PeerKey    string     `gorm:"column:peer_key"`
	Note       string     `gorm:"column:note"`
	DueAt      time.Time  `gorm:"column:due_at"`
	Snoozes    int        `gorm:"column:snoozes"`
	CreatedAt  time.Time  `gorm:"column:created_at"`
	DoneAt     *time.Time `gorm:"column:done_at"`
	DoneReason *string    `gorm:"column:done_reason"`
	DoneBy     *uint      `gorm:"column:done_by"`
}

// TableName pins the table name.
func (CallReminder) TableName() string { return "call_reminders" }
