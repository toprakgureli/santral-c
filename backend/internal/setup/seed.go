// Package setup seeds baseline permissions, roles and the owner account.
package setup

import (
	"fmt"
	"log/slog"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/toprakgureli/santral-c/backend/configs"
	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
	"github.com/toprakgureli/santral-c/backend/pkg/enums"
	"github.com/toprakgureli/santral-c/backend/pkg/hash"
)

// Seed applies the permission, role, owner and default-settings seed
// idempotently.
func Seed(db *gorm.DB) error {
	if err := seedPermissions(db); err != nil {
		return err
	}
	if err := seedRoles(db); err != nil {
		return err
	}
	if err := seedSettings(db); err != nil {
		return err
	}
	return seedOwner(db)
}

// seedSettings writes default runtime flags when they are not yet present, so a
// fresh deployment can force MFA enrollment out of the box (security.requireMFA)
// without overriding a value an admin later changed.
func seedSettings(db *gorm.DB) error {
	var count int64
	if err := db.Model(&models.SystemSetting{}).Where("key = ?", "mfa_required").Count(&count).Error; err != nil {
		return fmt.Errorf("mfa_required setting could not be checked: %w", err)
	}
	if count > 0 {
		return nil
	}
	value := "false"
	if configs.Cnf.Security.RequireMFA {
		value = "true"
	}
	if err := db.Create(&models.SystemSetting{Key: "mfa_required", Value: value}).Error; err != nil {
		return fmt.Errorf("mfa_required setting could not be seeded: %w", err)
	}
	slog.Info("mfa_required setting seeded", "value", value)
	return nil
}

func seedPermissions(db *gorm.DB) error {
	list := enums.Permissions()
	rows := make([]models.Permission, 0, len(list))
	for _, p := range list {
		rows = append(rows, models.Permission{
			Key:         string(p.Key),
			Module:      string(p.Key.Module()),
			Description: p.Description,
		})
	}
	if err := db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"module", "description", "updated_at"}),
	}).Create(&rows).Error; err != nil {
		return fmt.Errorf("permissions could not be seeded: %w", err)
	}
	return nil
}

func seedRoles(db *gorm.DB) error {
	var existing []string
	if err := db.Model(&models.Role{}).Pluck("name", &existing).Error; err != nil {
		return fmt.Errorf("roles could not be listed: %w", err)
	}
	found := make(map[string]bool, len(existing))
	for _, name := range existing {
		found[name] = true
	}

	var perms []models.Permission
	if err := db.Find(&perms).Error; err != nil {
		return fmt.Errorf("permissions could not be listed: %w", err)
	}
	byKey := make(map[string]models.Permission, len(perms))
	for _, p := range perms {
		byKey[p.Key] = p
	}

	for _, info := range enums.Roles() {
		if found[string(info.Name)] {
			continue
		}
		role := models.Role{
			Name:        string(info.Name),
			DisplayName: info.DisplayName,
			Description: info.Description,
			System:      true,
		}
		for _, key := range enums.RolePermissions(info.Name) {
			if p, ok := byKey[string(key)]; ok {
				role.Permissions = append(role.Permissions, p)
			}
		}
		if err := db.Omit("Permissions.*").Create(&role).Error; err != nil {
			return fmt.Errorf("role could not be created: %w", err)
		}
		slog.Info("role seeded", "role", string(info.Name), "permissions", len(role.Permissions))
	}

	if err := syncInvisibleAdminPermissions(db, perms); err != nil {
		return err
	}
	return syncSystemRolePermissions(db, byKey)
}

func syncInvisibleAdminPermissions(db *gorm.DB, perms []models.Permission) error {
	var role models.Role
	if err := db.Preload("Permissions").
		Where("name = ?", string(enums.RoleInvisibleAdmin)).
		First(&role).Error; err != nil {
		return fmt.Errorf("invisible-admin role could not be fetched: %w", err)
	}
	if len(role.Permissions) == len(perms) {
		return nil
	}
	if err := db.Model(&role).Omit("Permissions.*").Association("Permissions").Replace(perms); err != nil {
		return fmt.Errorf("invisible-admin permissions could not be synced: %w", err)
	}
	slog.Info("invisible-admin permissions synced", "permissions", len(perms))
	return nil
}

func syncSystemRolePermissions(db *gorm.DB, byKey map[string]models.Permission) error {
	for _, info := range enums.Roles() {
		if info.Name == enums.RoleInvisibleAdmin {
			continue
		}
		var role models.Role
		if err := db.Preload("Permissions").
			Where("name = ?", string(info.Name)).
			First(&role).Error; err != nil {
			return fmt.Errorf("role could not be fetched: %w", err)
		}
		current := make(map[string]bool, len(role.Permissions))
		for _, p := range role.Permissions {
			current[p.Key] = true
		}
		missing := make([]models.Permission, 0)
		for _, key := range enums.RolePermissions(info.Name) {
			if current[string(key)] {
				continue
			}
			if p, ok := byKey[string(key)]; ok {
				missing = append(missing, p)
				current[p.Key] = true
			}
		}
		if len(missing) == 0 {
			continue
		}
		if err := db.Model(&role).Omit("Permissions.*").Association("Permissions").Append(missing); err != nil {
			return fmt.Errorf("role permissions could not be synced: %w", err)
		}
		slog.Info("role permissions synced", "role", string(info.Name), "added", len(missing))
	}
	return nil
}

func seedOwner(db *gorm.DB) error {
	var count int64
	if err := db.Model(&models.User{}).Count(&count).Error; err != nil {
		return fmt.Errorf("users could not be counted: %w", err)
	}
	if count > 0 {
		return nil
	}

	c := configs.Cnf.Owner
	if c.Email == "" || c.Password == "" {
		return fmt.Errorf("owner credentials are missing")
	}

	pw, err := hash.Password(c.Password)
	if err != nil {
		return err
	}

	var role models.Role
	if err := db.Where("name = ?", string(enums.RoleInvisibleAdmin)).First(&role).Error; err != nil {
		return fmt.Errorf("invisible-admin role could not be found: %w", err)
	}

	owner := models.User{
		Name:               c.Name,
		Email:              c.Email,
		Password:           pw,
		Active:             true,
		MustChangePassword: true,
		Roles:              []models.Role{role},
	}
	if err := db.Omit("Roles.*").Create(&owner).Error; err != nil {
		return fmt.Errorf("owner user could not be created: %w", err)
	}
	slog.Info("owner user seeded", "email", c.Email)
	return nil
}
