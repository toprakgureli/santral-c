package verimor

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
)

// Repository reads and writes the SIP fields on users.
type Repository struct {
	db *gorm.DB
}

// NewRepository builds a Verimor repository.
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// GetUser loads a user, or nil when absent.
func (r *Repository) GetUser(ctx context.Context, id uint) (*models.User, error) {
	var u models.User
	err := r.db.WithContext(ctx).First(&u, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("user could not be fetched: %w", err)
	}
	return &u, nil
}

// SetPresence upserts the actor's presence state.
func (r *Repository) SetPresence(ctx context.Context, userID uint, state string) error {
	presence := models.AgentPresence{UserID: userID, State: state, UpdatedAt: time.Now()}
	if err := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "user_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"state", "updated_at"}),
		}).Create(&presence).Error; err != nil {
		return fmt.Errorf("presence could not be stored: %w", err)
	}
	return nil
}

// GetPresence returns the actor's presence state and since when it has held,
// defaulting to "available" with a zero time when no row exists yet.
func (r *Repository) GetPresence(ctx context.Context, userID uint) (string, time.Time, error) {
	var presence models.AgentPresence
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).First(&presence).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "available", time.Time{}, nil
	}
	if err != nil {
		return "available", time.Time{}, fmt.Errorf("presence could not be fetched: %w", err)
	}
	return presence.State, presence.UpdatedAt, nil
}

// PresenceByExtension maps each extension to its stored non-available presence
// state, so the live agent list can reflect who is on a break or in backoffice.
func (r *Repository) PresenceByExtension(ctx context.Context) (map[string]string, error) {
	type row struct {
		Extension string
		State     string
	}
	var rows []row
	err := r.db.WithContext(ctx).
		Table("agent_presence AS p").
		Select("u.sip_extension AS extension, p.state AS state").
		Joins("JOIN users u ON u.id = p.user_id").
		Where("p.state <> 'available' AND u.sip_extension IS NOT NULL AND u.sip_extension <> ''").
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("presence map could not be built: %w", err)
	}
	out := make(map[string]string, len(rows))
	for _, r := range rows {
		out[r.Extension] = r.State
	}
	return out, nil
}

// SetSIP stores a user's SIP extension and encrypted SIP password.
func (r *Repository) SetSIP(ctx context.Context, id uint, extension, encPassword string) error {
	if err := r.db.WithContext(ctx).
		Model(&models.User{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"sip_extension":   extension,
			"sip_secret":      encPassword,
			"sip_provisioned": true,
		}).Error; err != nil {
		return fmt.Errorf("sip credentials could not be stored: %w", err)
	}
	return nil
}
