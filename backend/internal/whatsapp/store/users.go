package store

import (
	"context"
)

// UserHeadline reads the headline on a panel user's profile.
func (r *Repository) UserHeadline(ctx context.Context, id uint) (string, error) {
	var headline string
	err := r.db.WithContext(ctx).Raw("SELECT headline FROM users WHERE id = ?", id).Scan(&headline).Error
	return headline, err
}

// ActiveUsersWithPermission lists the active panel users who hold a
// permission through one of their roles.
func (r *Repository) ActiveUsersWithPermission(ctx context.Context, key string) ([]uint, error) {
	var ids []uint
	if err := r.db.WithContext(ctx).Raw(`
		SELECT DISTINCT ur.user_id FROM user_roles ur
		JOIN role_permissions rp ON rp.role_id = ur.role_id
		JOIN permissions p ON p.id = rp.permission_id
		JOIN users u ON u.id = ur.user_id
		WHERE p.key = ? AND u.active`, key).Scan(&ids).Error; err != nil {
		return nil, err
	}
	return ids, nil
}

// AvailableAgents lists the people on shift whose state is available.
func (r *Repository) AvailableAgents(ctx context.Context) ([]uint, error) {
	var avail []uint
	if err := r.db.WithContext(ctx).Raw(`SELECT sh.user_id FROM shifts sh LEFT JOIN agent_presence ap ON ap.user_id = sh.user_id
		WHERE sh.ended_at IS NULL AND COALESCE(ap.state, 'available') = 'available'`).Scan(&avail).Error; err != nil {
		return nil, err
	}
	return avail, nil
}

// ChannelMember is a person placed on a device.
type ChannelMember struct {
	ChannelID uint
	UserID    uint
}

// ChannelMembers lists who is placed on which device.
func (r *Repository) ChannelMembers(ctx context.Context) ([]ChannelMember, error) {
	var members []ChannelMember
	if err := r.db.WithContext(ctx).Raw("SELECT channel_id, user_id FROM wa_channel_members").Scan(&members).Error; err != nil {
		return nil, err
	}
	return members, nil
}

// TeamMember is a person in a team.
type TeamMember struct {
	TeamID uint
	UserID uint
}

// TeamMembers lists who is in which team.
func (r *Repository) TeamMembers(ctx context.Context) ([]TeamMember, error) {
	var teams []TeamMember
	if err := r.db.WithContext(ctx).Raw("SELECT team_id, user_id FROM wa_team_members").Scan(&teams).Error; err != nil {
		return nil, err
	}
	return teams, nil
}

// ChannelsOfUser lists the devices a person is placed on.
func (r *Repository) ChannelsOfUser(ctx context.Context, userID uint) ([]uint, error) {
	var chans []uint
	err := r.db.WithContext(ctx).Raw("SELECT channel_id FROM wa_channel_members WHERE user_id = ?", userID).Scan(&chans).Error
	return chans, err
}

// TeamsOfUser lists the teams a person is in.
func (r *Repository) TeamsOfUser(ctx context.Context, userID uint) ([]uint, error) {
	var teams []uint
	err := r.db.WithContext(ctx).Raw("SELECT team_id FROM wa_team_members WHERE user_id = ?", userID).Scan(&teams).Error
	return teams, err
}

// Person is a panel user as shown beside a conversation: name, whether
// they have a picture and when their profile last changed, in Unix seconds.
type Person struct {
	ID        uint
	Name      string
	HasAvatar bool
	Version   int64
}

// People reads the panel users with the given ids.
func (r *Repository) People(ctx context.Context, ids []uint) ([]Person, error) {
	var rows []Person
	err := r.db.WithContext(ctx).Raw("SELECT id, name, avatar <> '' AS has_avatar, extract(epoch from updated_at)::bigint AS version FROM users WHERE id IN ?", ids).Scan(&rows).Error
	return rows, err
}
