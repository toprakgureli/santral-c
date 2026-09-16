package models

import (
	"time"

	"gorm.io/gorm"
)

// EscalationCategory is an admin-managed grouping of escalation reasons, e.g.
// "Memnuniyet".
type EscalationCategory struct {
	ID        uint               `gorm:"column:id;primarykey"`
	Name      string             `gorm:"column:name;size:160;not null"`
	SortOrder int                `gorm:"column:sort_order;not null;default:0"`
	CreatedBy *uint              `gorm:"column:created_by"`
	Reasons   []EscalationReason `gorm:"foreignKey:CategoryID"`
	CreatedAt time.Time          `gorm:"column:created_at"`
	UpdatedAt time.Time          `gorm:"column:updated_at"`
	DeletedAt gorm.DeletedAt     `gorm:"column:deleted_at;index"`
}

// TableName pins the table name.
func (EscalationCategory) TableName() string { return "escalation_categories" }

// EscalationReason is one concrete situation under a category.
type EscalationReason struct {
	ID         uint           `gorm:"column:id;primarykey"`
	CategoryID uint           `gorm:"column:category_id;not null;index"`
	Name       string         `gorm:"column:name;size:240;not null"`
	SortOrder  int            `gorm:"column:sort_order;not null;default:0"`
	CreatedAt  time.Time      `gorm:"column:created_at"`
	UpdatedAt  time.Time      `gorm:"column:updated_at"`
	DeletedAt  gorm.DeletedAt `gorm:"column:deleted_at;index"`
}

// TableName pins the table name.
func (EscalationReason) TableName() string { return "escalation_reasons" }

// CallEscalation is one escalation an agent logged against a customer number.
type CallEscalation struct {
	ID           uint      `gorm:"column:id;primarykey"`
	NumberKey    string    `gorm:"column:number_key;size:20;not null;index"`
	Number       string    `gorm:"column:number;size:32;not null"`
	CategoryID   *uint     `gorm:"column:category_id"`
	ReasonID     *uint     `gorm:"column:reason_id"`
	CategoryName string    `gorm:"column:category_name;size:160;not null"`
	ReasonName   string    `gorm:"column:reason_name;size:240;not null"`
	Note         string    `gorm:"column:note;type:text"`
	AgentID      *uint     `gorm:"column:agent_id"`
	AgentName    string    `gorm:"column:agent_name;size:160;not null"`
	CallUUID     *string   `gorm:"column:call_uuid;size:80"`
	CreatedAt    time.Time `gorm:"column:created_at"`
}

// TableName pins the table name.
func (CallEscalation) TableName() string { return "call_escalations" }
