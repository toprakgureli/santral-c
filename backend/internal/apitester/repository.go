package apitester

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
)

const keyEnabled = "api_tester_enabled"

// Repository persists API test collections, requests, run logs and feature toggle.
type Repository struct {
	db *gorm.DB
}

// NewRepository builds an apitester repository backed by db.
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// IsEnabled reports whether the optional API tester module is turned on.
// When the setting row has not been written yet, it defaults to enabled.
func (r *Repository) IsEnabled(ctx context.Context) (bool, error) {
	var vals []string
	if err := r.db.WithContext(ctx).
		Model(&models.SystemSetting{}).
		Where("key = ?", keyEnabled).
		Limit(1).
		Pluck("value", &vals).Error; err != nil {
		return false, err
	}
	if len(vals) == 0 {
		return true, nil
	}
	v := strings.TrimSpace(vals[0])
	return v == "1" || strings.EqualFold(v, "true"), nil
}

// SetEnabled saves the global on/off flag in system_settings.
func (r *Repository) SetEnabled(ctx context.Context, enabled bool) error {
	val := "0"
	if enabled {
		val = "1"
	}
	row := models.SystemSetting{Key: keyEnabled, Value: val, UpdatedAt: time.Now()}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value", "updated_at"}),
	}).Create(&row).Error
}

// ListCollections returns collections ordered by sort_order, id, preloading requests.
func (r *Repository) ListCollections(ctx context.Context, onlyActive bool) ([]models.APICollection, error) {
	var out []models.APICollection
	q := r.db.WithContext(ctx).
		Preload("Requests", func(tx *gorm.DB) *gorm.DB {
			if onlyActive {
				tx = tx.Where("active = ?", true)
			}
			return tx.Order("sort_order ASC, id ASC")
		}).
		Order("sort_order ASC, id ASC")
	if onlyActive {
		q = q.Where("active = ?", true)
	}
	if err := q.Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

// GetCollection returns a single collection by ID or nil if missing.
func (r *Repository) GetCollection(ctx context.Context, id uint) (*models.APICollection, error) {
	var col models.APICollection
	err := r.db.WithContext(ctx).First(&col, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &col, nil
}

// NextCollectionOrder returns max(sort_order)+1 for api_collections.
func (r *Repository) NextCollectionOrder(ctx context.Context) (int, error) {
	var maxOrder int
	if err := r.db.WithContext(ctx).Model(&models.APICollection{}).
		Select("COALESCE(MAX(sort_order), 0)").
		Scan(&maxOrder).Error; err != nil {
		return 0, err
	}
	return maxOrder + 1, nil
}

// CreateCollection inserts a collection record.
func (r *Repository) CreateCollection(ctx context.Context, col *models.APICollection) error {
	return r.db.WithContext(ctx).Omit("Requests").Create(col).Error
}

// UpdateCollection updates an existing collection record.
func (r *Repository) UpdateCollection(ctx context.Context, col *models.APICollection) error {
	return r.db.WithContext(ctx).Omit("Requests", "CreatedAt", "CreatedBy").Save(col).Error
}

// DeleteCollection removes a collection (cascade removes its requests).
func (r *Repository) DeleteCollection(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Delete(&models.APICollection{}, id).Error
}

// GetRequest loads a request template by ID or returns nil if missing.
func (r *Repository) GetRequest(ctx context.Context, id uint) (*models.APIRequest, error) {
	var req models.APIRequest
	err := r.db.WithContext(ctx).First(&req, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &req, nil
}

// NextRequestOrder returns max(sort_order)+1 within a collection.
func (r *Repository) NextRequestOrder(ctx context.Context, collectionID uint) (int, error) {
	var maxOrder int
	if err := r.db.WithContext(ctx).Model(&models.APIRequest{}).
		Where("collection_id = ?", collectionID).
		Select("COALESCE(MAX(sort_order), 0)").
		Scan(&maxOrder).Error; err != nil {
		return 0, err
	}
	return maxOrder + 1, nil
}

// CreateRequest inserts a request template.
func (r *Repository) CreateRequest(ctx context.Context, req *models.APIRequest) error {
	return r.db.WithContext(ctx).Create(req).Error
}

// UpdateRequest saves updates to a request template.
func (r *Repository) UpdateRequest(ctx context.Context, req *models.APIRequest) error {
	return r.db.WithContext(ctx).Omit("CreatedAt", "CreatedBy").Save(req).Error
}

// DeleteRequest removes a request template by ID.
func (r *Repository) DeleteRequest(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Delete(&models.APIRequest{}, id).Error
}

// CreateRunLog writes an execution log entry.
func (r *Repository) CreateRunLog(ctx context.Context, row *models.APIRunLog) error {
	return r.db.WithContext(ctx).Create(row).Error
}

// ListRunLogs returns recent execution logs, optionally filtered by requestID.
func (r *Repository) ListRunLogs(ctx context.Context, requestID *uint, limit int) ([]models.APIRunLog, error) {
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	var out []models.APIRunLog
	q := r.db.WithContext(ctx).Order("created_at DESC, id DESC").Limit(limit)
	if requestID != nil && *requestID > 0 {
		q = q.Where("request_id = ?", *requestID)
	}
	if err := q.Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}
