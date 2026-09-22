package models

import "time"

// ChatGroup is a room: a named group or a two-person direct message.
type ChatGroup struct {
	ID          uint       `gorm:"column:id;primarykey"`
	Kind        string     `gorm:"column:kind;size:12;not null;default:group"`
	Name        string     `gorm:"column:name;size:120;not null;default:''"`
	Description string     `gorm:"column:description;type:text;not null;default:''"`
	Avatar      string     `gorm:"column:avatar;type:text;not null;default:''"`
	PostPolicy  string     `gorm:"column:post_policy;size:12;not null;default:everyone"`
	DMKey       *string    `gorm:"column:dm_key;size:41"`
	CreatedBy   *uint      `gorm:"column:created_by"`
	CreatedAt   time.Time  `gorm:"column:created_at"`
	UpdatedAt   time.Time  `gorm:"column:updated_at"`
	DeletedAt   *time.Time `gorm:"column:deleted_at"`
}

// TableName pins the table name.
func (ChatGroup) TableName() string { return "chat_groups" }

// ChatMember is one person's seat in a room, with their in-room role.
type ChatMember struct {
	GroupID         uint      `gorm:"column:group_id;primarykey"`
	UserID          uint      `gorm:"column:user_id;primarykey"`
	Role            string    `gorm:"column:role;size:8;not null;default:member"`
	CanPost         bool      `gorm:"column:can_post;not null;default:true"`
	Muted           bool      `gorm:"column:muted;not null;default:false"`
	LastReadID      uint      `gorm:"column:last_read_id;not null;default:0"`
	LastDeliveredID uint      `gorm:"column:last_delivered_id;not null;default:0"`
	InvitedBy       *uint     `gorm:"column:invited_by"`
	JoinedAt        time.Time `gorm:"column:joined_at"`
}

// TableName pins the table name.
func (ChatMember) TableName() string { return "chat_members" }

// ChatInvite is a pending seat the person still has to accept.
type ChatInvite struct {
	ID        uint       `gorm:"column:id;primarykey"`
	GroupID   uint       `gorm:"column:group_id;not null"`
	UserID    uint       `gorm:"column:user_id;not null"`
	InvitedBy *uint      `gorm:"column:invited_by"`
	Status    string     `gorm:"column:status;size:10;not null;default:pending"`
	CreatedAt time.Time  `gorm:"column:created_at"`
	DecidedAt *time.Time `gorm:"column:decided_at"`
}

// TableName pins the table name.
func (ChatInvite) TableName() string { return "chat_invites" }

// ChatMessage is one line in a room. Attachments is reserved for media.
type ChatMessage struct {
	ID          uint       `gorm:"column:id;primarykey"`
	GroupID     uint       `gorm:"column:group_id;not null;index"`
	SenderID    *uint      `gorm:"column:sender_id"`
	Kind        string     `gorm:"column:kind;size:8;not null;default:text"`
	Body        string     `gorm:"column:body;type:text;not null;default:''"`
	ReplyToID   *uint      `gorm:"column:reply_to_id"`
	Attachments *string    `gorm:"column:attachments;type:jsonb"`
	CreatedAt   time.Time  `gorm:"column:created_at"`
	EditedAt    *time.Time `gorm:"column:edited_at"`
	DeletedAt   *time.Time `gorm:"column:deleted_at"`
	DeletedBy   *uint      `gorm:"column:deleted_by"`
}

// TableName pins the table name.
func (ChatMessage) TableName() string { return "chat_messages" }

// ChatReaction is one person's emoji on one message.
type ChatReaction struct {
	MessageID uint      `gorm:"column:message_id;primarykey"`
	UserID    uint      `gorm:"column:user_id;primarykey"`
	Emoji     string    `gorm:"column:emoji;size:16;primarykey"`
	CreatedAt time.Time `gorm:"column:created_at"`
}

// TableName pins the table name.
func (ChatReaction) TableName() string { return "chat_reactions" }

// ChatPresence remembers when a person last had the chat open.
type ChatPresence struct {
	UserID     uint      `gorm:"column:user_id;primarykey"`
	LastSeenAt time.Time `gorm:"column:last_seen_at"`
}

// TableName pins the table name.
func (ChatPresence) TableName() string { return "chat_presence" }
