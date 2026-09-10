package escalation

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
)

// Repository is the escalation data store.
type Repository struct {
	db *gorm.DB
}

// NewRepository builds an escalation repository.
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// Categories lists every category with its reasons, ordered by name.
func (r *Repository) Categories(ctx context.Context) ([]models.EscalationCategory, error) {
	var cats []models.EscalationCategory
	err := r.db.WithContext(ctx).
		Preload("Reasons", func(db *gorm.DB) *gorm.DB { return db.Order("name ASC") }).
		Order("name ASC").
		Find(&cats).Error
	if err != nil {
		return nil, fmt.Errorf("categories could not be listed: %w", err)
	}
	return cats, nil
}

// CreateCategory inserts a category.
func (r *Repository) CreateCategory(ctx context.Context, c *models.EscalationCategory) error {
	if err := r.db.WithContext(ctx).Create(c).Error; err != nil {
		return fmt.Errorf("category could not be created: %w", err)
	}
	return nil
}

// DeleteCategory soft-deletes a category (its reasons cascade on hard delete;
// while soft-deleted they are simply filtered out with the category).
func (r *Repository) DeleteCategory(ctx context.Context, id uint) error {
	if err := r.db.WithContext(ctx).Delete(&models.EscalationCategory{}, id).Error; err != nil {
		return fmt.Errorf("category could not be deleted: %w", err)
	}
	return nil
}

// GetCategory loads a category, or nil when absent.
func (r *Repository) GetCategory(ctx context.Context, id uint) (*models.EscalationCategory, error) {
	var c models.EscalationCategory
	err := r.db.WithContext(ctx).First(&c, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("category could not be fetched: %w", err)
	}
	return &c, nil
}

// CreateReason inserts a reason under a category.
func (r *Repository) CreateReason(ctx context.Context, reason *models.EscalationReason) error {
	if err := r.db.WithContext(ctx).Create(reason).Error; err != nil {
		return fmt.Errorf("reason could not be created: %w", err)
	}
	return nil
}

// DeleteReason soft-deletes a reason.
func (r *Repository) DeleteReason(ctx context.Context, id uint) error {
	if err := r.db.WithContext(ctx).Delete(&models.EscalationReason{}, id).Error; err != nil {
		return fmt.Errorf("reason could not be deleted: %w", err)
	}
	return nil
}

// GetReason loads a reason together with its category, or nil when absent.
func (r *Repository) GetReason(ctx context.Context, id uint) (*models.EscalationReason, *models.EscalationCategory, error) {
	var reason models.EscalationReason
	err := r.db.WithContext(ctx).First(&reason, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("reason could not be fetched: %w", err)
	}
	cat, err := r.GetCategory(ctx, reason.CategoryID)
	if err != nil {
		return nil, nil, err
	}
	return &reason, cat, nil
}

// CreateEscalation stores one logged escalation.
func (r *Repository) CreateEscalation(ctx context.Context, e *models.CallEscalation) error {
	if err := r.db.WithContext(ctx).Create(e).Error; err != nil {
		return fmt.Errorf("escalation could not be created: %w", err)
	}
	return nil
}

// EscalationsByNumberKey lists escalations for a number key, newest first.
func (r *Repository) EscalationsByNumberKey(ctx context.Context, key string, limit int) ([]models.CallEscalation, error) {
	var out []models.CallEscalation
	err := r.db.WithContext(ctx).
		Where("number_key = ?", key).
		Order("created_at DESC").
		Limit(limit).
		Find(&out).Error
	if err != nil {
		return nil, fmt.Errorf("escalations could not be listed: %w", err)
	}
	return out, nil
}

// UpsertCatalog inserts categories and reasons from an import, skipping any that
// already exist (case-insensitively). It returns how many of each were added.
func (r *Repository) UpsertCatalog(ctx context.Context, actorID uint, catalog map[string][]string) (int, int, error) {
	var addedCats, addedReasons int
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for catName, reasons := range catalog {
			var cat models.EscalationCategory
			err := tx.Where("lower(name) = lower(?)", catName).First(&cat).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				cat = models.EscalationCategory{Name: catName, CreatedBy: &actorID}
				if err := tx.Create(&cat).Error; err != nil {
					return fmt.Errorf("category %q could not be created: %w", catName, err)
				}
				addedCats++
			} else if err != nil {
				return fmt.Errorf("category %q could not be checked: %w", catName, err)
			}
			for _, reasonName := range reasons {
				var count int64
				if err := tx.Model(&models.EscalationReason{}).
					Where("category_id = ? AND lower(name) = lower(?)", cat.ID, reasonName).
					Count(&count).Error; err != nil {
					return fmt.Errorf("reason %q could not be checked: %w", reasonName, err)
				}
				if count > 0 {
					continue
				}
				if err := tx.Create(&models.EscalationReason{CategoryID: cat.ID, Name: reasonName}).Error; err != nil {
					return fmt.Errorf("reason %q could not be created: %w", reasonName, err)
				}
				addedReasons++
			}
		}
		return nil
	})
	return addedCats, addedReasons, err
}
