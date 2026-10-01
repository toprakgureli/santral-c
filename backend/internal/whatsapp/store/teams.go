package store

import (
	"context"

	"gorm.io/gorm"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
)

// AllTeams reads every team, in no particular order.
func (r *Repository) AllTeams(ctx context.Context) ([]models.WATeam, error) {
	var teams []models.WATeam
	if err := r.db.WithContext(ctx).Find(&teams).Error; err != nil {
		return nil, err
	}
	return teams, nil
}

// TeamsByName reads every team in the order of their names.
func (r *Repository) TeamsByName(ctx context.Context) ([]models.WATeam, error) {
	var list []models.WATeam
	if err := r.db.WithContext(ctx).Order("name").Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// TeamMemberIDs lists the people in a team, by id.
func (r *Repository) TeamMemberIDs(ctx context.Context, teamID uint) ([]uint, error) {
	var ids []uint
	if err := r.db.WithContext(ctx).Raw("SELECT user_id FROM wa_team_members WHERE team_id = ? ORDER BY user_id", teamID).Scan(&ids).Error; err != nil {
		return nil, err
	}
	return ids, nil
}

// SaveTeam creates a team (id 0) or renames and recolours one, and sets
// its members to exactly the given people, all in one transaction.
func (r *Repository) SaveTeam(ctx context.Context, id uint, name, color string, memberIDs []uint) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if id == 0 {
			t := &models.WATeam{Name: name, Color: color}
			if err := tx.Create(t).Error; err != nil {
				return err
			}
			id = t.ID
		} else if err := tx.Exec("UPDATE wa_teams SET name = ?, color = ? WHERE id = ?", name, color, id).Error; err != nil {
			return err
		}
		if err := tx.Exec("DELETE FROM wa_team_members WHERE team_id = ?", id).Error; err != nil {
			return err
		}
		for _, uid := range memberIDs {
			if err := tx.Exec("INSERT INTO wa_team_members (team_id, user_id) VALUES (?, ?) ON CONFLICT DO NOTHING", id, uid).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// DeleteTeam removes a team.
func (r *Repository) DeleteTeam(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Delete(&models.WATeam{}, id).Error
}

// TeamName reads a team's name inside a transaction; it is empty when the
// team does not exist.
func TeamName(tx *gorm.DB, id uint) (string, error) {
	var name string
	err := tx.Raw("SELECT name FROM wa_teams WHERE id = ?", id).Scan(&name).Error
	return name, err
}

// TeamName reads a team's name; it is empty when the team does not exist.
func (r *Repository) TeamName(ctx context.Context, id uint) (string, error) {
	return TeamName(r.db.WithContext(ctx), id)
}
