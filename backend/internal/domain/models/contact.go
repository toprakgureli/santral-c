package models

import (
	"time"

	"gorm.io/gorm"
)

// Contact is an external party the contact center communicates with.
type Contact struct {
	ID        uint           `gorm:"column:id;primarykey"`
	Name      string         `gorm:"column:name;size:160;not null"`
	Company   string         `gorm:"column:company;size:160"`
	Email     string         `gorm:"column:email;size:255"`
	Notes     string         `gorm:"column:notes;type:text"`
	CreatedBy *uint          `gorm:"column:created_by;index"`
	Phones    []ContactPhone `gorm:"foreignKey:ContactID"`
	CreatedAt time.Time      `gorm:"column:created_at"`
	UpdatedAt time.Time      `gorm:"column:updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"column:deleted_at;index"`
}

// ContactPhone is one dialable number belonging to a contact.
type ContactPhone struct {
	ID         uint      `gorm:"column:id;primarykey"`
	ContactID  uint      `gorm:"column:contact_id;not null;index"`
	Label      string    `gorm:"column:label;size:20;not null;default:other"`
	NumberE164 string    `gorm:"column:number_e164;size:20;not null;uniqueIndex"`
	IsPrimary  bool      `gorm:"column:is_primary;not null;default:false"`
	CreatedAt  time.Time `gorm:"column:created_at"`
	UpdatedAt  time.Time `gorm:"column:updated_at"`
}
