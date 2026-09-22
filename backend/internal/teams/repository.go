package teams

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
)

// Repository is the chat data store.
type Repository struct {
	db *gorm.DB
}

// NewRepository builds a chat repository.
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// Person is the little card of a user the chat needs.
type Person struct {
	ID        uint   `json:"id"`
	Name      string `json:"name"`
	HasAvatar bool   `json:"hasAvatar"`
	Version   int64  `json:"avatarVersion,omitempty"`
	Online    bool   `json:"online"`
	LastSeen  string `json:"lastSeen,omitempty"`
	Active    bool   `json:"-"`
}

// People lists active users as chat cards.
func (r *Repository) People(ctx context.Context) ([]Person, error) {
	var users []models.User
	if err := r.db.WithContext(ctx).Where("active = TRUE").Order("name").Find(&users).Error; err != nil {
		return nil, fmt.Errorf("people could not be listed: %w", err)
	}
	out := make([]Person, 0, len(users))
	for i := range users {
		out = append(out, personOf(&users[i]))
	}
	return out, nil
}

// PeopleByID loads cards for a set of ids (any state, for message senders).
func (r *Repository) PeopleByID(ctx context.Context, ids []uint) (map[uint]Person, error) {
	out := make(map[uint]Person, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	var users []models.User
	if err := r.db.WithContext(ctx).Unscoped().Where("id IN ?", ids).Find(&users).Error; err != nil {
		return nil, fmt.Errorf("people could not be loaded: %w", err)
	}
	for i := range users {
		out[users[i].ID] = personOf(&users[i])
	}
	return out, nil
}

func personOf(u *models.User) Person {
	p := Person{ID: u.ID, Name: u.Name, HasAvatar: u.Avatar != "", Active: u.Active}
	if u.Avatar != "" {
		p.Version = u.UpdatedAt.Unix()
	}
	return p
}

// Group loads a live room, or nil.
func (r *Repository) Group(ctx context.Context, id uint) (*models.ChatGroup, error) {
	var g models.ChatGroup
	err := r.db.WithContext(ctx).Where("id = ? AND deleted_at IS NULL", id).First(&g).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("group could not be loaded: %w", err)
	}
	return &g, nil
}

// Member loads one seat, or nil.
func (r *Repository) Member(ctx context.Context, groupID, userID uint) (*models.ChatMember, error) {
	var m models.ChatMember
	err := r.db.WithContext(ctx).Where("group_id = ? AND user_id = ?", groupID, userID).First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("member could not be loaded: %w", err)
	}
	return &m, nil
}

// Members lists a room's seats.
func (r *Repository) Members(ctx context.Context, groupID uint) ([]models.ChatMember, error) {
	var out []models.ChatMember
	if err := r.db.WithContext(ctx).Where("group_id = ?", groupID).Order("joined_at").Find(&out).Error; err != nil {
		return nil, fmt.Errorf("members could not be listed: %w", err)
	}
	return out, nil
}

// MemberIDs returns the user ids seated in a room.
func (r *Repository) MemberIDs(ctx context.Context, groupID uint) ([]uint, error) {
	var ids []uint
	if err := r.db.WithContext(ctx).Model(&models.ChatMember{}).Where("group_id = ?", groupID).Pluck("user_id", &ids).Error; err != nil {
		return nil, fmt.Errorf("member ids could not be listed: %w", err)
	}
	return ids, nil
}

// MyGroups lists the rooms a user sits in, newest activity first.
func (r *Repository) MyGroups(ctx context.Context, userID uint) ([]models.ChatGroup, map[uint]models.ChatMember, error) {
	var seats []models.ChatMember
	if err := r.db.WithContext(ctx).Where("user_id = ?", userID).Find(&seats).Error; err != nil {
		return nil, nil, fmt.Errorf("seats could not be listed: %w", err)
	}
	if len(seats) == 0 {
		return []models.ChatGroup{}, map[uint]models.ChatMember{}, nil
	}
	ids := make([]uint, 0, len(seats))
	byGroup := make(map[uint]models.ChatMember, len(seats))
	for _, s := range seats {
		ids = append(ids, s.GroupID)
		byGroup[s.GroupID] = s
	}
	var groups []models.ChatGroup
	if err := r.db.WithContext(ctx).Where("id IN ? AND deleted_at IS NULL", ids).Order("updated_at DESC").Find(&groups).Error; err != nil {
		return nil, nil, fmt.Errorf("groups could not be listed: %w", err)
	}
	return groups, byGroup, nil
}

// LastMessages returns the newest message per room.
func (r *Repository) LastMessages(ctx context.Context, groupIDs []uint) (map[uint]models.ChatMessage, error) {
	out := make(map[uint]models.ChatMessage)
	if len(groupIDs) == 0 {
		return out, nil
	}
	var rows []models.ChatMessage
	err := r.db.WithContext(ctx).Raw(
		"SELECT DISTINCT ON (group_id) * FROM chat_messages WHERE group_id IN ? ORDER BY group_id, id DESC", groupIDs,
	).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("last messages could not be loaded: %w", err)
	}
	for _, m := range rows {
		out[m.GroupID] = m
	}
	return out, nil
}

// Unread counts messages newer than each seat's last read, not the user's own.
func (r *Repository) Unread(ctx context.Context, userID uint) (map[uint]int64, error) {
	type row struct {
		GroupID uint
		N       int64
	}
	var rows []row
	err := r.db.WithContext(ctx).Raw(
		"SELECT m.group_id, count(*) AS n FROM chat_messages m "+
			"JOIN chat_members s ON s.group_id = m.group_id AND s.user_id = ? "+
			"WHERE m.id > s.last_read_id AND m.deleted_at IS NULL AND (m.sender_id IS NULL OR m.sender_id <> ?) "+
			"GROUP BY m.group_id", userID, userID,
	).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("unread counts could not be computed: %w", err)
	}
	out := make(map[uint]int64, len(rows))
	for _, x := range rows {
		out[x.GroupID] = x.N
	}
	return out, nil
}

// MemberCounts counts seats per room.
func (r *Repository) MemberCounts(ctx context.Context, groupIDs []uint) (map[uint]int64, error) {
	type row struct {
		GroupID uint
		N       int64
	}
	out := make(map[uint]int64)
	if len(groupIDs) == 0 {
		return out, nil
	}
	var rows []row
	if err := r.db.WithContext(ctx).Model(&models.ChatMember{}).Select("group_id, count(*) AS n").Where("group_id IN ?", groupIDs).Group("group_id").Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("member counts could not be computed: %w", err)
	}
	for _, x := range rows {
		out[x.GroupID] = x.N
	}
	return out, nil
}

// DMPeers maps each dm room to the other person's id for one user.
func (r *Repository) DMPeers(ctx context.Context, userID uint, groupIDs []uint) (map[uint]uint, error) {
	out := make(map[uint]uint)
	if len(groupIDs) == 0 {
		return out, nil
	}
	var seats []models.ChatMember
	if err := r.db.WithContext(ctx).Where("group_id IN ? AND user_id <> ?", groupIDs, userID).Find(&seats).Error; err != nil {
		return nil, fmt.Errorf("dm peers could not be loaded: %w", err)
	}
	for _, s := range seats {
		out[s.GroupID] = s.UserID
	}
	return out, nil
}

// CreateGroup inserts a room with its first seats in one transaction.
func (r *Repository) CreateGroup(ctx context.Context, g *models.ChatGroup, seats []models.ChatMember) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(g).Error; err != nil {
			return fmt.Errorf("group could not be created: %w", err)
		}
		for i := range seats {
			seats[i].GroupID = g.ID
		}
		if len(seats) > 0 {
			if err := tx.Create(&seats).Error; err != nil {
				return fmt.Errorf("members could not be seated: %w", err)
			}
		}
		return nil
	})
}

// GroupByDMKey finds an existing direct-message room.
func (r *Repository) GroupByDMKey(ctx context.Context, key string) (*models.ChatGroup, error) {
	var g models.ChatGroup
	err := r.db.WithContext(ctx).Where("dm_key = ? AND deleted_at IS NULL", key).First(&g).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("dm could not be loaded: %w", err)
	}
	return &g, nil
}

// UpdateGroup writes the given columns and bumps updated_at.
func (r *Repository) UpdateGroup(ctx context.Context, id uint, fields map[string]any) error {
	fields["updated_at"] = time.Now()
	if err := r.db.WithContext(ctx).Model(&models.ChatGroup{}).Where("id = ?", id).Updates(fields).Error; err != nil {
		return fmt.Errorf("group could not be updated: %w", err)
	}
	return nil
}

// TouchGroup bumps updated_at so the room floats to the top of lists.
func (r *Repository) TouchGroup(ctx context.Context, id uint) error {
	return r.UpdateGroup(ctx, id, map[string]any{})
}

// DeleteGroup soft-deletes a room.
func (r *Repository) DeleteGroup(ctx context.Context, id uint) error {
	return r.UpdateGroup(ctx, id, map[string]any{"deleted_at": time.Now()})
}

// AddMembers seats users, skipping ones already seated.
func (r *Repository) AddMembers(ctx context.Context, seats []models.ChatMember) error {
	if len(seats) == 0 {
		return nil
	}
	if err := r.db.WithContext(ctx).Exec(
		"INSERT INTO chat_members (group_id, user_id, role, can_post, invited_by, joined_at) VALUES "+placeholders(len(seats), 6)+" ON CONFLICT DO NOTHING",
		seatArgs(seats)...,
	).Error; err != nil {
		return fmt.Errorf("members could not be added: %w", err)
	}
	return nil
}

func placeholders(rows, cols int) string {
	out := ""
	for i := 0; i < rows; i++ {
		if i > 0 {
			out += ","
		}
		out += "("
		for j := 0; j < cols; j++ {
			if j > 0 {
				out += ","
			}
			out += "?"
		}
		out += ")"
	}
	return out
}

func seatArgs(seats []models.ChatMember) []any {
	args := make([]any, 0, len(seats)*6)
	now := time.Now()
	for _, s := range seats {
		args = append(args, s.GroupID, s.UserID, s.Role, s.CanPost, s.InvitedBy, now)
	}
	return args
}

// RemoveMember unseats a user.
func (r *Repository) RemoveMember(ctx context.Context, groupID, userID uint) error {
	if err := r.db.WithContext(ctx).Where("group_id = ? AND user_id = ?", groupID, userID).Delete(&models.ChatMember{}).Error; err != nil {
		return fmt.Errorf("member could not be removed: %w", err)
	}
	return nil
}

// UpdateMember writes seat columns.
func (r *Repository) UpdateMember(ctx context.Context, groupID, userID uint, fields map[string]any) error {
	if err := r.db.WithContext(ctx).Model(&models.ChatMember{}).Where("group_id = ? AND user_id = ?", groupID, userID).Updates(fields).Error; err != nil {
		return fmt.Errorf("member could not be updated: %w", err)
	}
	return nil
}

// CreateInvites opens pending invites, skipping duplicates.
func (r *Repository) CreateInvites(ctx context.Context, groupID, by uint, userIDs []uint) error {
	for _, uid := range userIDs {
		inv := models.ChatInvite{GroupID: groupID, UserID: uid, InvitedBy: &by, Status: "pending"}
		if err := r.db.WithContext(ctx).Exec(
			"INSERT INTO chat_invites (group_id, user_id, invited_by, status, created_at) VALUES (?, ?, ?, 'pending', now()) ON CONFLICT DO NOTHING",
			inv.GroupID, inv.UserID, inv.InvitedBy,
		).Error; err != nil {
			return fmt.Errorf("invite could not be created: %w", err)
		}
	}
	return nil
}

// Invite loads one invite, or nil.
func (r *Repository) Invite(ctx context.Context, id uint) (*models.ChatInvite, error) {
	var inv models.ChatInvite
	err := r.db.WithContext(ctx).First(&inv, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("invite could not be loaded: %w", err)
	}
	return &inv, nil
}

// PendingInvites lists a user's open invites.
func (r *Repository) PendingInvites(ctx context.Context, userID uint) ([]models.ChatInvite, error) {
	var out []models.ChatInvite
	if err := r.db.WithContext(ctx).Where("user_id = ? AND status = 'pending'", userID).Order("created_at DESC").Find(&out).Error; err != nil {
		return nil, fmt.Errorf("invites could not be listed: %w", err)
	}
	return out, nil
}

// PendingInviteIDs lists who has an open invite to a room.
func (r *Repository) PendingInviteIDs(ctx context.Context, groupID uint) ([]uint, error) {
	var ids []uint
	if err := r.db.WithContext(ctx).Model(&models.ChatInvite{}).Where("group_id = ? AND status = 'pending'", groupID).Pluck("user_id", &ids).Error; err != nil {
		return nil, fmt.Errorf("invitees could not be listed: %w", err)
	}
	return ids, nil
}

// DecideInvite closes an invite and, when accepted, seats the person.
func (r *Repository) DecideInvite(ctx context.Context, inv *models.ChatInvite, accept bool) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		status := "declined"
		if accept {
			status = "accepted"
		}
		now := time.Now()
		if err := tx.Model(&models.ChatInvite{}).Where("id = ? AND status = 'pending'", inv.ID).
			Updates(map[string]any{"status": status, "decided_at": now}).Error; err != nil {
			return fmt.Errorf("invite could not be decided: %w", err)
		}
		if accept {
			if err := tx.Exec(
				"INSERT INTO chat_members (group_id, user_id, role, can_post, invited_by, joined_at) VALUES (?, ?, 'member', true, ?, now()) ON CONFLICT DO NOTHING",
				inv.GroupID, inv.UserID, inv.InvitedBy,
			).Error; err != nil {
				return fmt.Errorf("member could not be seated: %w", err)
			}
		}
		return nil
	})
}

// Messages pages a room's history, newest first, before a given id.
func (r *Repository) Messages(ctx context.Context, groupID uint, beforeID uint, limit int) ([]models.ChatMessage, error) {
	q := r.db.WithContext(ctx).Where("group_id = ?", groupID)
	if beforeID > 0 {
		q = q.Where("id < ?", beforeID)
	}
	var out []models.ChatMessage
	if err := q.Order("id DESC").Limit(limit).Find(&out).Error; err != nil {
		return nil, fmt.Errorf("messages could not be listed: %w", err)
	}
	return out, nil
}

// Message loads one message, or nil.
func (r *Repository) Message(ctx context.Context, id uint) (*models.ChatMessage, error) {
	var m models.ChatMessage
	err := r.db.WithContext(ctx).First(&m, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("message could not be loaded: %w", err)
	}
	return &m, nil
}

// CreateMessage inserts a line and bumps the room.
func (r *Repository) CreateMessage(ctx context.Context, m *models.ChatMessage) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(m).Error; err != nil {
			return fmt.Errorf("message could not be created: %w", err)
		}
		if err := tx.Model(&models.ChatGroup{}).Where("id = ?", m.GroupID).Update("updated_at", time.Now()).Error; err != nil {
			return fmt.Errorf("group could not be touched: %w", err)
		}
		return nil
	})
}

// SoftDeleteMessage marks a line deleted; the row stays.
func (r *Repository) SoftDeleteMessage(ctx context.Context, id, by uint) error {
	if err := r.db.WithContext(ctx).Model(&models.ChatMessage{}).Where("id = ? AND deleted_at IS NULL", id).
		Updates(map[string]any{"deleted_at": time.Now(), "deleted_by": by}).Error; err != nil {
		return fmt.Errorf("message could not be deleted: %w", err)
	}
	return nil
}

// Reactions lists reactions of a set of messages.
func (r *Repository) Reactions(ctx context.Context, messageIDs []uint) ([]models.ChatReaction, error) {
	if len(messageIDs) == 0 {
		return nil, nil
	}
	var out []models.ChatReaction
	if err := r.db.WithContext(ctx).Where("message_id IN ?", messageIDs).Order("created_at").Find(&out).Error; err != nil {
		return nil, fmt.Errorf("reactions could not be listed: %w", err)
	}
	return out, nil
}

// ToggleReaction adds the emoji for the user, or removes it if present.
func (r *Repository) ToggleReaction(ctx context.Context, messageID, userID uint, emoji string) error {
	res := r.db.WithContext(ctx).Where("message_id = ? AND user_id = ? AND emoji = ?", messageID, userID, emoji).Delete(&models.ChatReaction{})
	if res.Error != nil {
		return fmt.Errorf("reaction could not be toggled: %w", res.Error)
	}
	if res.RowsAffected > 0 {
		return nil
	}
	if err := r.db.WithContext(ctx).Create(&models.ChatReaction{MessageID: messageID, UserID: userID, Emoji: emoji, CreatedAt: time.Now()}).Error; err != nil {
		return fmt.Errorf("reaction could not be added: %w", err)
	}
	return nil
}

// MarkRead advances the seat's read pointer, never backwards.
func (r *Repository) MarkRead(ctx context.Context, groupID, userID, messageID uint) error {
	if err := r.db.WithContext(ctx).Model(&models.ChatMember{}).
		Where("group_id = ? AND user_id = ? AND last_read_id < ?", groupID, userID, messageID).
		Updates(map[string]any{"last_read_id": messageID, "last_delivered_id": gorm.Expr("GREATEST(last_delivered_id, ?)", messageID)}).Error; err != nil {
		return fmt.Errorf("read pointer could not be moved: %w", err)
	}
	return nil
}

// Seats loads the seats of several rooms at once, keyed by room.
func (r *Repository) Seats(ctx context.Context, groupIDs []uint) (map[uint][]models.ChatMember, error) {
	out := map[uint][]models.ChatMember{}
	if len(groupIDs) == 0 {
		return out, nil
	}
	var rows []models.ChatMember
	if err := r.db.WithContext(ctx).Where("group_id IN ?", groupIDs).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("seats could not be loaded: %w", err)
	}
	for _, st := range rows {
		out[st.GroupID] = append(out[st.GroupID], st)
	}
	return out, nil
}

// MarkDelivered advances the seat's delivery pointer, never backwards, and
// reports whether it moved.
func (r *Repository) MarkDelivered(ctx context.Context, groupID, userID, messageID uint) (bool, error) {
	res := r.db.WithContext(ctx).Model(&models.ChatMember{}).
		Where("group_id = ? AND user_id = ? AND last_delivered_id < ?", groupID, userID, messageID).
		Update("last_delivered_id", messageID)
	if res.Error != nil {
		return false, fmt.Errorf("delivery pointer could not be moved: %w", res.Error)
	}
	return res.RowsAffected > 0, nil
}

// Delivered is a room whose delivery pointer just moved.
type Delivered struct {
	GroupID     uint
	DeliveredID uint
	ReadID      uint
}

// MarkDeliveredAll moves every seat of the user up to its room's newest
// line (the client has just fetched the room list, previews included) and
// returns the rooms that moved.
func (r *Repository) MarkDeliveredAll(ctx context.Context, userID uint) ([]Delivered, error) {
	var out []Delivered
	err := r.db.WithContext(ctx).Raw(
		"UPDATE chat_members s SET last_delivered_id = x.max_id "+
			"FROM (SELECT group_id, MAX(id) AS max_id FROM chat_messages GROUP BY group_id) x "+
			"WHERE s.group_id = x.group_id AND s.user_id = ? AND s.last_delivered_id < x.max_id "+
			"RETURNING s.group_id AS group_id, s.last_delivered_id AS delivered_id, s.last_read_id AS read_id", userID).
		Scan(&out).Error
	if err != nil {
		return nil, fmt.Errorf("delivery pointers could not be moved: %w", err)
	}
	return out, nil
}

// TouchSeen records that the person has the chat open right now.
func (r *Repository) TouchSeen(ctx context.Context, userID uint) error {
	return r.db.WithContext(ctx).Exec(
		"INSERT INTO chat_presence (user_id, last_seen_at) VALUES (?, now()) "+
			"ON CONFLICT (user_id) DO UPDATE SET last_seen_at = EXCLUDED.last_seen_at", userID).Error
}

// LastSeen loads when each of the given people last had the chat open.
func (r *Repository) LastSeen(ctx context.Context, ids []uint) (map[uint]time.Time, error) {
	out := map[uint]time.Time{}
	if len(ids) == 0 {
		return out, nil
	}
	var rows []models.ChatPresence
	if err := r.db.WithContext(ctx).Where("user_id IN ?", ids).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("presence could not be loaded: %w", err)
	}
	for _, p := range rows {
		out[p.UserID] = p.LastSeenAt
	}
	return out, nil
}
