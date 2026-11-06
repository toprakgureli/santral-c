package models

import "time"

// CustomMenu is an admin-defined sidebar entry pointing to an external web
// application opened inside the panel's iframe workspace (or in a new tab).
type CustomMenu struct {
	ID        uint      `gorm:"column:id;primarykey"`
	Label     string    `gorm:"column:label;size:120;not null"`
	Icon      string    `gorm:"column:icon;size:64;not null;default:globe"`
	TargetURL string    `gorm:"column:target_url;type:text;not null"`
	OpenMode  string    `gorm:"column:open_mode;size:20;not null;default:iframe"`
	Active    bool      `gorm:"column:active;not null;default:true"`
	SortOrder int       `gorm:"column:sort_order;not null;default:0"`
	CreatedBy *uint     `gorm:"column:created_by"`
	Roles     []Role    `gorm:"many2many:custom_menu_roles;joinForeignKey:menu_id;joinReferences:role_id"`
	CreatedAt time.Time `gorm:"column:created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

// TableName pins the table name.
func (CustomMenu) TableName() string { return "custom_menus" }
