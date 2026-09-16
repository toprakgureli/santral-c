// Package contact owns CRM contact persistence and lookup.
package contact

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/toprakgureli/santral-c/backend/internal/domain/dtos/requests"
	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
)

// Repository is the contact data store.
type Repository struct {
	db *gorm.DB
}

// NewRepository builds a contact repository.
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// Create inserts a contact together with its phone numbers.
func (r *Repository) Create(ctx context.Context, c *models.Contact) error {
	if err := r.db.WithContext(ctx).Create(c).Error; err != nil {
		return fmt.Errorf("contact could not be created: %w", err)
	}
	return nil
}

// GetByID loads a contact with its phones, or nil when absent.
func (r *Repository) GetByID(ctx context.Context, id uint) (*models.Contact, error) {
	var c models.Contact
	err := r.db.WithContext(ctx).
		Preload("Phones").
		First(&c, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("contact could not be fetched by id: %w", err)
	}
	return &c, nil
}

// List returns a filtered, paginated page of contacts and the total count.
func (r *Repository) List(ctx context.Context, f requests.ContactFilter) ([]models.Contact, int64, error) {
	q := r.db.WithContext(ctx).Model(&models.Contact{})
	if f.Query != "" {
		like := "%" + f.Query + "%"
		q = q.Where(
			r.db.Where("name ILIKE ?", like).
				Or("company ILIKE ?", like).
				Or("email ILIKE ?", like).
				Or("id IN (?)", r.db.
					Table("contact_phones").
					Select("contact_id").
					Where("number_e164 ILIKE ?", like)),
		)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("contacts could not be counted: %w", err)
	}

	var contacts []models.Contact
	if err := q.
		Preload("Phones").
		Order("name ASC").
		Limit(f.PerPage).
		Offset((f.Page - 1) * f.PerPage).
		Find(&contacts).Error; err != nil {
		return nil, 0, fmt.Errorf("contacts could not be listed: %w", err)
	}
	return contacts, total, nil
}

// UpdateCore updates a contact's core fields.
func (r *Repository) UpdateCore(ctx context.Context, id uint, fields map[string]any) error {
	if err := r.db.WithContext(ctx).
		Model(&models.Contact{}).
		Where("id = ?", id).
		Updates(fields).Error; err != nil {
		return fmt.Errorf("contact could not be updated: %w", err)
	}
	return nil
}

// SoftDelete marks a contact deleted.
func (r *Repository) SoftDelete(ctx context.Context, id uint) error {
	if err := r.db.WithContext(ctx).Delete(&models.Contact{}, id).Error; err != nil {
		return fmt.Errorf("contact could not be deleted: %w", err)
	}
	return nil
}

// PhoneExists reports whether any contact already holds the number.
func (r *Repository) PhoneExists(ctx context.Context, e164 string) (bool, error) {
	var count int64
	if err := r.db.WithContext(ctx).
		Model(&models.ContactPhone{}).
		Where("number_e164 = ?", e164).
		Count(&count).Error; err != nil {
		return false, fmt.Errorf("phone existence could not be checked: %w", err)
	}
	return count > 0, nil
}

// AddPhone inserts a phone, demoting other primaries when the new one is primary.
func (r *Repository) AddPhone(ctx context.Context, phone *models.ContactPhone) error {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if phone.IsPrimary {
			if err := tx.Model(&models.ContactPhone{}).
				Where("contact_id = ?", phone.ContactID).
				Update("is_primary", false).Error; err != nil {
				return fmt.Errorf("existing primary could not be demoted: %w", err)
			}
		}
		if err := tx.Create(phone).Error; err != nil {
			return fmt.Errorf("phone could not be added: %w", err)
		}
		return nil
	})
	return err
}

// RemovePhone deletes a phone belonging to a contact and reports whether a row
// was removed.
func (r *Repository) RemovePhone(ctx context.Context, contactID, phoneID uint) (bool, error) {
	res := r.db.WithContext(ctx).
		Where("id = ? AND contact_id = ?", phoneID, contactID).
		Delete(&models.ContactPhone{})
	if res.Error != nil {
		return false, fmt.Errorf("phone could not be removed: %w", res.Error)
	}
	return res.RowsAffected > 0, nil
}

// NameByNumber returns the name of the contact that owns a number, or "" when
// the number is unknown. Lookup failures read as unknown.
func (r *Repository) NameByNumber(ctx context.Context, e164 string) string {
	c, err := r.ResolveByNumber(ctx, e164)
	if err != nil || c == nil {
		return ""
	}
	return c.Name
}

// ResolveByNumber finds the contact that owns a number, or nil when none does.
func (r *Repository) ResolveByNumber(ctx context.Context, e164 string) (*models.Contact, error) {
	var phone models.ContactPhone
	err := r.db.WithContext(ctx).
		Where("number_e164 = ?", e164).
		First(&phone).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("number could not be resolved: %w", err)
	}
	return r.GetByID(ctx, phone.ContactID)
}
