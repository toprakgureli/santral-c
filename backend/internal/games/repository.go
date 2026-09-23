package games

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
)

// Repository is the games' data access.
type Repository struct {
	db *gorm.DB
}

// NewRepository builds a repository.
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// ---------------------------------------------------------------- settings

func (r *Repository) Setting(ctx context.Context, key string) string {
	var value string
	_ = r.db.WithContext(ctx).Raw("SELECT value FROM system_settings WHERE key = ?", key).Scan(&value).Error
	return value
}

func (r *Repository) SetSetting(ctx context.Context, key, value string) error {
	return r.db.WithContext(ctx).Exec(
		"INSERT INTO system_settings (key, value, updated_at) VALUES (?, ?, now()) "+
			"ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now()", key, value).Error
}

// ---------------------------------------------------------------- items

// Items lists content of a kind, active ones first, newest first.
func (r *Repository) Items(ctx context.Context, kind string, onlyActive bool) ([]models.GameItem, error) {
	q := r.db.WithContext(ctx).Where("kind = ?", kind)
	if onlyActive {
		q = q.Where("active")
	}
	var out []models.GameItem
	if err := q.Order("active DESC, id DESC").Find(&out).Error; err != nil {
		return nil, fmt.Errorf("game items could not be listed: %w", err)
	}
	return out, nil
}

// ItemCounts counts active content per kind.
func (r *Repository) ItemCounts(ctx context.Context) (map[string]int64, error) {
	var rows []struct {
		Kind  string
		Count int64
	}
	if err := r.db.WithContext(ctx).Model(&models.GameItem{}).Select("kind, count(*) AS count").Where("active").Group("kind").Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("game item counts could not be computed: %w", err)
	}
	out := map[string]int64{}
	for _, row := range rows {
		out[row.Kind] = row.Count
	}
	return out, nil
}

// CreateItem stores one piece of content.
func (r *Repository) CreateItem(ctx context.Context, it *models.GameItem) error {
	if err := r.db.WithContext(ctx).Create(it).Error; err != nil {
		return fmt.Errorf("game item could not be created: %w", err)
	}
	return nil
}

// UpdateItem changes the given columns.
func (r *Repository) UpdateItem(ctx context.Context, id uint, fields map[string]any) error {
	if err := r.db.WithContext(ctx).Model(&models.GameItem{}).Where("id = ?", id).Updates(fields).Error; err != nil {
		return fmt.Errorf("game item could not be updated: %w", err)
	}
	return nil
}

// DeleteItem removes one piece of content.
func (r *Repository) DeleteItem(ctx context.Context, id uint) error {
	if err := r.db.WithContext(ctx).Where("id = ?", id).Delete(&models.GameItem{}).Error; err != nil {
		return fmt.Errorf("game item could not be deleted: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------- games

// Game loads one match, or nil.
func (r *Repository) Game(ctx context.Context, id uint) (*models.Game, error) {
	var g models.Game
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&g).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("game could not be loaded: %w", err)
	}
	return &g, nil
}

// OpenGames lists matches not yet finished, for warming the engine.
func (r *Repository) OpenGames(ctx context.Context) ([]models.Game, error) {
	var out []models.Game
	if err := r.db.WithContext(ctx).Where("status IN ('lobby', 'playing')").Find(&out).Error; err != nil {
		return nil, fmt.Errorf("open games could not be listed: %w", err)
	}
	return out, nil
}

// OpenGamesInGroup lists a room's unfinished matches, newest first.
func (r *Repository) OpenGamesInGroup(ctx context.Context, groupID uint) ([]models.Game, error) {
	var out []models.Game
	if err := r.db.WithContext(ctx).Where("group_id = ? AND status IN ('lobby', 'playing')", groupID).Order("id DESC").Find(&out).Error; err != nil {
		return nil, fmt.Errorf("open games could not be listed: %w", err)
	}
	return out, nil
}

// CreateGame stores a new match.
func (r *Repository) CreateGame(ctx context.Context, g *models.Game) error {
	if err := r.db.WithContext(ctx).Create(g).Error; err != nil {
		return fmt.Errorf("game could not be created: %w", err)
	}
	return nil
}

// SaveGame writes the match's status, state and stamps.
func (r *Repository) SaveGame(ctx context.Context, g *models.Game) error {
	if err := r.db.WithContext(ctx).Model(&models.Game{}).Where("id = ?", g.ID).Updates(map[string]any{
		"status": g.Status, "config": g.Config, "state": g.State, "winners": g.Winners, "version": g.Version,
		"message_id": g.MessageID, "started_at": g.StartedAt, "finished_at": g.FinishedAt,
	}).Error; err != nil {
		return fmt.Errorf("game could not be saved: %w", err)
	}
	return nil
}

// Players lists a match's seats in joining order.
func (r *Repository) Players(ctx context.Context, gameID uint) ([]models.GamePlayer, error) {
	var out []models.GamePlayer
	if err := r.db.WithContext(ctx).Where("game_id = ?", gameID).Order("joined_at, user_id").Find(&out).Error; err != nil {
		return nil, fmt.Errorf("players could not be listed: %w", err)
	}
	return out, nil
}

// AddPlayer seats a person.
func (r *Repository) AddPlayer(ctx context.Context, p *models.GamePlayer) error {
	if err := r.db.WithContext(ctx).Create(p).Error; err != nil {
		return fmt.Errorf("player could not be added: %w", err)
	}
	return nil
}

// RemovePlayer frees a seat.
func (r *Repository) RemovePlayer(ctx context.Context, gameID, userID uint) error {
	if err := r.db.WithContext(ctx).Where("game_id = ? AND user_id = ?", gameID, userID).Delete(&models.GamePlayer{}).Error; err != nil {
		return fmt.Errorf("player could not be removed: %w", err)
	}
	return nil
}

// SetScores writes every seat's score.
func (r *Repository) SetScores(ctx context.Context, gameID uint, scores map[uint]int) error {
	for uid, sc := range scores {
		if err := r.db.WithContext(ctx).Model(&models.GamePlayer{}).Where("game_id = ? AND user_id = ?", gameID, uid).Update("score", sc).Error; err != nil {
			return fmt.Errorf("score could not be saved: %w", err)
		}
	}
	return nil
}

// RecordResults writes the outcome rows of a finished match.
func (r *Repository) RecordResults(ctx context.Context, rows []models.GameResult) error {
	if len(rows) == 0 {
		return nil
	}
	if err := r.db.WithContext(ctx).Create(&rows).Error; err != nil {
		return fmt.Errorf("results could not be recorded: %w", err)
	}
	return nil
}

// LeaderRow is one person's tally over a period.
type LeaderRow struct {
	UserID uint  `json:"userId"`
	Played int64 `json:"played"`
	Wins   int64 `json:"wins"`
	Points int64 `json:"points"`
}

// Leaderboard tallies results since a point in time, optionally for one kind.
func (r *Repository) Leaderboard(ctx context.Context, since time.Time, kind string) ([]LeaderRow, error) {
	q := r.db.WithContext(ctx).Model(&models.GameResult{}).
		Select("user_id, count(*) AS played, count(*) FILTER (WHERE won) AS wins, COALESCE(SUM(score), 0) AS points").
		Where("finished_at >= ?", since)
	if kind != "" {
		q = q.Where("kind = ?", kind)
	}
	var out []LeaderRow
	if err := q.Group("user_id").Order("wins DESC, points DESC, played ASC").Limit(50).Scan(&out).Error; err != nil {
		return nil, fmt.Errorf("leaderboard could not be computed: %w", err)
	}
	return out, nil
}

// UserTally is one person's overall and per-kind record.
type UserTally struct {
	Kind   string `json:"kind"`
	Played int64  `json:"played"`
	Wins   int64  `json:"wins"`
	Points int64  `json:"points"`
}

// UserRecord tallies one person's results per kind, all time.
func (r *Repository) UserRecord(ctx context.Context, userID uint) ([]UserTally, error) {
	var out []UserTally
	if err := r.db.WithContext(ctx).Model(&models.GameResult{}).
		Select("kind, count(*) AS played, count(*) FILTER (WHERE won) AS wins, COALESCE(SUM(score), 0) AS points").
		Where("user_id = ?", userID).Group("kind").Order("played DESC").Scan(&out).Error; err != nil {
		return nil, fmt.Errorf("record could not be computed: %w", err)
	}
	return out, nil
}

// RecentLines picks up to n live text lines by the given people from a room
// in the last week, for "Kim Söyledi".
func (r *Repository) RecentLines(ctx context.Context, groupID uint, userIDs []uint, n int) ([]models.ChatMessage, error) {
	var out []models.ChatMessage
	if len(userIDs) == 0 {
		return out, nil
	}
	if err := r.db.WithContext(ctx).
		Where("group_id = ? AND kind = 'text' AND deleted_at IS NULL AND sender_id IN ? AND created_at >= ? AND length(body) BETWEEN 8 AND 200", groupID, userIDs, time.Now().Add(-7*24*time.Hour)).
		Order("random()").Limit(n).Find(&out).Error; err != nil {
		return nil, fmt.Errorf("recent lines could not be picked: %w", err)
	}
	return out, nil
}

// PresenceState reads a person's live status (available, break, ...).
func (r *Repository) PresenceState(ctx context.Context, userID uint) string {
	var state string
	_ = r.db.WithContext(ctx).Raw("SELECT state FROM agent_presence WHERE user_id = ?", userID).Scan(&state).Error
	return state
}

// Candidate is a person on the poll's ballot.
type Candidate struct {
	ID        uint   `json:"id"`
	Name      string `json:"name"`
	HasAvatar bool   `json:"hasAvatar"`
}

// ActiveUsers lists everyone active, for a ballot wider than the room.
func (r *Repository) ActiveUsers(ctx context.Context) ([]Candidate, error) {
	var rows []struct {
		ID     uint
		Name   string
		Avatar string
	}
	if err := r.db.WithContext(ctx).Model(&models.User{}).Select("id, name, avatar").Where("active").Order("name").Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("users could not be listed: %w", err)
	}
	out := make([]Candidate, 0, len(rows))
	for _, row := range rows {
		out = append(out, Candidate{ID: row.ID, Name: row.Name, HasAvatar: row.Avatar != ""})
	}
	return out, nil
}

// Names resolves display names.
func (r *Repository) Names(ctx context.Context, ids []uint) (map[uint]string, error) {
	out := map[uint]string{}
	if len(ids) == 0 {
		return out, nil
	}
	var rows []struct {
		ID   uint
		Name string
	}
	if err := r.db.WithContext(ctx).Model(&models.User{}).Unscoped().Select("id, name").Where("id IN ?", ids).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("names could not be loaded: %w", err)
	}
	for _, row := range rows {
		out[row.ID] = row.Name
	}
	return out, nil
}
