package teams

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

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
	// InRoom: looking at the room this card is shown in right now.
	InRoom bool `json:"inRoom,omitempty"`
	Active bool `json:"-"`
}

// People lists active users as chat cards. withOwner keeps the invisible
// admins in the list; only another invisible admin sees them.
func (r *Repository) People(ctx context.Context, withOwner bool) ([]Person, error) {
	var users []models.User
	q := r.db.WithContext(ctx).Where("active = TRUE")
	if !withOwner {
		q = q.Where("id NOT IN (" + models.InvisibleAdminIDsSQL + ")")
	}
	if err := q.Order("name").Find(&users).Error; err != nil {
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

// LastMessages returns, per room, the newest live line the user may read:
// a line from before their seat's history start never shows as a preview.
func (r *Repository) LastMessages(ctx context.Context, userID uint, groupIDs []uint) (map[uint]models.ChatMessage, error) {
	out := make(map[uint]models.ChatMessage)
	if len(groupIDs) == 0 {
		return out, nil
	}
	var rows []models.ChatMessage
	// One index probe per room, from the top of its lines.
	err := r.db.WithContext(ctx).Raw(
		"SELECT m.* FROM chat_members s CROSS JOIN LATERAL ("+
			"SELECT * FROM chat_messages x WHERE x.group_id = s.group_id AND x.deleted_at IS NULL "+
			"AND x.id >= s.history_from ORDER BY x.id DESC LIMIT 1) m "+
			"WHERE s.user_id = ? AND s.group_id IN ?", userID, groupIDs,
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
	// Each room counts at most unreadCap lines: the panel shows "99+" past
	// that, and a room with a long unread tail must not be counted in full
	// on every refresh.
	err := r.db.WithContext(ctx).Raw(
		"SELECT s.group_id, (SELECT count(*) FROM (SELECT 1 FROM chat_messages m "+
			"WHERE m.group_id = s.group_id AND m.id > s.last_read_id AND m.id >= s.history_from AND m.deleted_at IS NULL "+
			"AND (m.sender_id IS NULL OR m.sender_id <> s.user_id) LIMIT ?) c) AS n "+
			"FROM chat_members s WHERE s.user_id = ?", unreadCap, userID,
	).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("unread counts could not be computed: %w", err)
	}
	out := make(map[uint]int64, len(rows))
	for _, x := range rows {
		if x.N > 0 {
			out[x.GroupID] = x.N
		}
	}
	return out, nil
}

// unreadCap is the most unread lines counted per room.
const unreadCap = 100

// receiptWindow is how far back per-line delivery and read times are
// written. Older lines are covered by the seat's pointers alone (shown with
// the line's own time), so someone returning to a long history does not
// write a row for every line in it.
const receiptWindow = "7 days"

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

// DeleteGroup soft-deletes a room.
func (r *Repository) DeleteGroup(ctx context.Context, id uint) error {
	return r.UpdateGroup(ctx, id, map[string]any{"deleted_at": time.Now()})
}

// History is how much of a room's earlier conversation a newcomer may read.
type History string

// The choices offered when people are added or invited.
const (
	HistoryNone History = "none"
	History50   History = "50"
	History100  History = "100"
	HistoryAll  History = "all"
)

// ParseHistory reads a choice; an empty one means nothing earlier.
func ParseHistory(v string) (History, bool) {
	switch h := History(v); h {
	case "":
		return HistoryNone, true
	case HistoryNone, History50, History100, HistoryAll:
		return h, true
	}
	return "", false
}

// lines is how many earlier lines the choice shows, or -1 for all of them.
func (h History) lines() int {
	switch h {
	case History50:
		return 50
	case History100:
		return 100
	case HistoryAll:
		return -1
	}
	return 0
}

// startOf works out, inside the seating transaction, where a newcomer's
// view of a room begins and which line is its newest. They start with
// everything up to that line read, whatever they may scroll back to.
// floor is the earliest line the person seating them can read (0: all);
// the newcomer never starts before it.
func startOf(tx *gorm.DB, groupID uint, h History, floor uint) (from, top uint, err error) {
	from, top, err = historyStart(tx, groupID, h)
	if err == nil && floor > from {
		from = floor
	}
	return from, top, err
}

func historyStart(tx *gorm.DB, groupID uint, h History) (from, top uint, err error) {
	if err := tx.Raw("SELECT COALESCE(max(id), 0) FROM chat_messages WHERE group_id = ?", groupID).Scan(&top).Error; err != nil {
		return 0, 0, fmt.Errorf("newest line could not be read: %w", err)
	}
	n := h.lines()
	switch {
	case n < 0:
		return 0, top, nil
	case n == 0:
		return top + 1, top, nil
	}
	// The last n messages people wrote; the grey notices between them come
	// along, and a room with fewer than n shows them all.
	var ids []uint
	if err := tx.Raw(
		"SELECT id FROM chat_messages WHERE group_id = ? AND kind <> 'system' AND deleted_at IS NULL "+
			"ORDER BY id DESC OFFSET ? LIMIT 1", groupID, n-1,
	).Scan(&ids).Error; err != nil {
		return 0, 0, fmt.Errorf("history start could not be found: %w", err)
	}
	if len(ids) == 0 {
		return 0, top, nil
	}
	return ids[0], top, nil
}

// AddMembers seats users in one room, skipping ones already seated. The
// choice decides how far back each newcomer may read, never before floor
// (the adder's own start). Their pending invites to the room are closed:
// they are in.
func (r *Repository) AddMembers(ctx context.Context, groupID uint, seats []models.ChatMember, h History, floor uint) error {
	if len(seats) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		from, top, err := startOf(tx, groupID, h, floor)
		if err != nil {
			return err
		}
		for i := range seats {
			seats[i].GroupID = groupID
			seats[i].HistoryFrom = from
			seats[i].LastReadID = top
			seats[i].LastDeliveredID = top
		}
		if err := tx.Exec(
			"INSERT INTO chat_members (group_id, user_id, role, can_post, invited_by, joined_at, history_from, last_read_id, last_delivered_id) VALUES "+
				placeholders(len(seats), seatCols)+" ON CONFLICT DO NOTHING",
			seatArgs(seats)...,
		).Error; err != nil {
			return fmt.Errorf("members could not be added: %w", err)
		}
		ids := make([]uint, len(seats))
		for i, st := range seats {
			ids[i] = st.UserID
		}
		return closeInvites(tx, groupID, ids)
	})
}

// closeInvites ends the pending invites of these people to a room, so an
// old invite (perhaps with a wider history choice) cannot seat them later.
func closeInvites(tx *gorm.DB, groupID uint, userIDs []uint) error {
	if len(userIDs) == 0 {
		return nil
	}
	if err := tx.Exec("UPDATE chat_invites SET status = 'declined', decided_at = now() WHERE group_id = ? AND user_id IN ? AND status = 'pending'",
		groupID, userIDs).Error; err != nil {
		return fmt.Errorf("pending invites could not be closed: %w", err)
	}
	return nil
}

// seatCols is how many columns seatArgs fills per seat.
const seatCols = 9

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
	args := make([]any, 0, len(seats)*seatCols)
	now := time.Now()
	for _, s := range seats {
		args = append(args, s.GroupID, s.UserID, s.Role, s.CanPost, s.InvitedBy, now, s.HistoryFrom, s.LastReadID, s.LastDeliveredID)
	}
	return args
}

// RemoveMember unseats a user.
// Their pending invites to the room go too, so leaving and accepting an
// older invite cannot widen what they may read.
func (r *Repository) RemoveMember(ctx context.Context, groupID, userID uint) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("group_id = ? AND user_id = ?", groupID, userID).Delete(&models.ChatMember{}).Error; err != nil {
			return fmt.Errorf("member could not be removed: %w", err)
		}
		return closeInvites(tx, groupID, []uint{userID})
	})
}

// UpdateMember writes seat columns.
func (r *Repository) UpdateMember(ctx context.Context, groupID, userID uint, fields map[string]any) error {
	if err := r.db.WithContext(ctx).Model(&models.ChatMember{}).Where("group_id = ? AND user_id = ?", groupID, userID).Updates(fields).Error; err != nil {
		return fmt.Errorf("member could not be updated: %w", err)
	}
	return nil
}

// CreateInvites opens pending invites with the history choice and the
// sender's own start (floor). A new invite to someone already invited
// replaces the waiting one, so the latest choice is the one that counts.
func (r *Repository) CreateInvites(ctx context.Context, groupID, by uint, userIDs []uint, h History, floor uint) error {
	for _, uid := range userIDs {
		if err := r.db.WithContext(ctx).Exec(
			"INSERT INTO chat_invites (group_id, user_id, invited_by, status, history, history_floor, created_at) VALUES (?, ?, ?, 'pending', ?, ?, now()) "+
				"ON CONFLICT (group_id, user_id) WHERE status = 'pending' DO UPDATE SET "+
				"invited_by = EXCLUDED.invited_by, history = EXCLUDED.history, history_floor = EXCLUDED.history_floor, created_at = EXCLUDED.created_at",
			groupID, uid, by, string(h), floor,
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

// DecideInvite closes an invite and, when accepted, seats the person the
// same way an added member is seated: starting at the newest line, read,
// with as much history as the inviter chose.
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
			h, ok := ParseHistory(inv.History)
			if !ok {
				h = HistoryNone
			}
			from, top, err := startOf(tx, inv.GroupID, h, inv.HistoryFloor)
			if err != nil {
				return err
			}
			if err := tx.Exec(
				"INSERT INTO chat_members (group_id, user_id, role, can_post, invited_by, joined_at, history_from, last_read_id, last_delivered_id) "+
					"VALUES (?, ?, 'member', true, ?, now(), ?, ?, ?) ON CONFLICT DO NOTHING",
				inv.GroupID, inv.UserID, inv.InvitedBy, from, top, top,
			).Error; err != nil {
				return fmt.Errorf("member could not be seated: %w", err)
			}
		}
		return nil
	})
}

// Messages pages a room's history, newest first, before a given id and
// from the reader's history start (0: from the beginning).
func (r *Repository) Messages(ctx context.Context, groupID, from, beforeID uint, limit int) ([]models.ChatMessage, error) {
	q := r.db.WithContext(ctx).Where("group_id = ?", groupID)
	if from > 0 {
		q = q.Where("id >= ?", from)
	}
	if beforeID > 0 {
		q = q.Where("id < ?", beforeID)
	}
	var out []models.ChatMessage
	if err := q.Order("id DESC").Limit(limit).Find(&out).Error; err != nil {
		return nil, fmt.Errorf("messages could not be listed: %w", err)
	}
	return out, nil
}

// MessagesAfter lists lines newer than an id, oldest first, never before
// the reader's history start.
func (r *Repository) MessagesAfter(ctx context.Context, groupID, from, afterID uint, limit int) ([]models.ChatMessage, error) {
	if from > 0 && afterID < from-1 {
		afterID = from - 1
	}
	var out []models.ChatMessage
	if err := r.db.WithContext(ctx).Where("group_id = ? AND id > ?", groupID, afterID).Order("id ASC").Limit(limit).Find(&out).Error; err != nil {
		return nil, fmt.Errorf("messages could not be listed: %w", err)
	}
	return out, nil
}

// SearchMessages finds live text lines containing the words, newest first,
// from the reader's history start.
func (r *Repository) SearchMessages(ctx context.Context, groupID, from uint, q string, limit int) ([]models.ChatMessage, error) {
	db := r.db.WithContext(ctx).Where("group_id = ? AND id >= ? AND kind = 'text' AND deleted_at IS NULL", groupID, from)
	for _, w := range strings.Fields(q) {
		db = db.Where("body ILIKE ?", "%"+strings.NewReplacer("%", "\\%", "_", "\\_").Replace(w)+"%")
	}
	var out []models.ChatMessage
	if err := db.Order("id DESC").Limit(limit).Find(&out).Error; err != nil {
		return nil, fmt.Errorf("messages could not be searched: %w", err)
	}
	return out, nil
}

// GroupAttachments pages a room's shared files of one kind, newest first,
// only from lines at or after the reader's history start.
func (r *Repository) GroupAttachments(ctx context.Context, groupID, from uint, kind string, beforeID uint, limit int) ([]models.ChatAttachment, error) {
	db := r.db.WithContext(ctx).
		Where("chat_attachments.group_id = ? AND chat_attachments.deleted_at IS NULL AND chat_attachments.status = 'ready' AND chat_attachments.message_id IS NOT NULL", groupID).
		Joins("JOIN chat_messages m ON m.id = chat_attachments.message_id AND m.deleted_at IS NULL AND m.id >= ?", from)
	if kind != "" {
		db = db.Where("chat_attachments.kind = ?", kind)
	}
	if beforeID > 0 {
		db = db.Where("chat_attachments.id < ?", beforeID)
	}
	var out []models.ChatAttachment
	if err := db.Order("chat_attachments.id DESC").Limit(limit).Find(&out).Error; err != nil {
		return nil, fmt.Errorf("attachments could not be listed: %w", err)
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
func (r *Repository) ToggleReaction(ctx context.Context, messageID, userID uint, emoji string) (bool, error) {
	res := r.db.WithContext(ctx).Where("message_id = ? AND user_id = ? AND emoji = ?", messageID, userID, emoji).Delete(&models.ChatReaction{})
	if res.Error != nil {
		return false, fmt.Errorf("reaction could not be toggled: %w", res.Error)
	}
	if res.RowsAffected > 0 {
		return false, nil
	}
	if err := r.db.WithContext(ctx).Create(&models.ChatReaction{MessageID: messageID, UserID: userID, Emoji: emoji, CreatedAt: time.Now()}).Error; err != nil {
		return false, fmt.Errorf("reaction could not be added: %w", err)
	}
	return true, nil
}

// MarkRead advances the seat's read pointer, never backwards, and clears a
// manual unread mark.
func (r *Repository) MarkRead(ctx context.Context, groupID, userID, messageID uint) error {
	// Log the lines this move passes over before the pointer forgets them.
	if err := r.db.WithContext(ctx).Exec(
		"INSERT INTO chat_receipts (message_id, user_id, delivered_at, read_at) "+
			"SELECT m.id, s.user_id, now(), now() FROM chat_members s "+
			"JOIN chat_messages m ON m.group_id = s.group_id AND m.id > s.last_read_id AND m.id <= ? AND m.id >= s.history_from "+
			"AND m.created_at > now() - interval '"+receiptWindow+"' "+
			"WHERE s.group_id = ? AND s.user_id = ? AND (m.sender_id IS NULL OR m.sender_id <> s.user_id) "+
			"ON CONFLICT (message_id, user_id) DO UPDATE SET "+
			"read_at = COALESCE(chat_receipts.read_at, EXCLUDED.read_at), "+
			"delivered_at = COALESCE(chat_receipts.delivered_at, EXCLUDED.delivered_at)",
		messageID, groupID, userID).Error; err != nil {
		return fmt.Errorf("read receipts could not be logged: %w", err)
	}
	if err := r.db.WithContext(ctx).Model(&models.ChatMember{}).
		Where("group_id = ? AND user_id = ?", groupID, userID).
		Updates(map[string]any{
			"last_read_id":      gorm.Expr("GREATEST(last_read_id, ?)", messageID),
			"last_delivered_id": gorm.Expr("GREATEST(last_delivered_id, ?)", messageID),
			"marked_unread":     false,
		}).Error; err != nil {
		return fmt.Errorf("read pointer could not be moved: %w", err)
	}
	return nil
}

// MarkUnread raises the seat's manual unread flag.
func (r *Repository) MarkUnread(ctx context.Context, groupID, userID uint) error {
	if err := r.db.WithContext(ctx).Model(&models.ChatMember{}).
		Where("group_id = ? AND user_id = ?", groupID, userID).
		Update("marked_unread", true).Error; err != nil {
		return fmt.Errorf("unread mark could not be set: %w", err)
	}
	return nil
}

// LastMessageID returns the newest line id of a room, or 0.
func (r *Repository) LastMessageID(ctx context.Context, groupID uint) (uint, error) {
	var id uint
	if err := r.db.WithContext(ctx).Model(&models.ChatMessage{}).Where("group_id = ?", groupID).Select("COALESCE(MAX(id), 0)").Scan(&id).Error; err != nil {
		return 0, fmt.Errorf("last message id could not be read: %w", err)
	}
	return id, nil
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
	if err := r.db.WithContext(ctx).Exec(
		"INSERT INTO chat_receipts (message_id, user_id, delivered_at) "+
			"SELECT m.id, s.user_id, now() FROM chat_members s "+
			"JOIN chat_messages m ON m.group_id = s.group_id AND m.id > s.last_delivered_id AND m.id <= ? AND m.id >= s.history_from "+
			"AND m.created_at > now() - interval '"+receiptWindow+"' "+
			"WHERE s.group_id = ? AND s.user_id = ? AND (m.sender_id IS NULL OR m.sender_id <> s.user_id) "+
			"ON CONFLICT (message_id, user_id) DO NOTHING",
		messageID, groupID, userID).Error; err != nil {
		return false, fmt.Errorf("delivery receipts could not be logged: %w", err)
	}
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
	if err := r.db.WithContext(ctx).Exec(
		"INSERT INTO chat_receipts (message_id, user_id, delivered_at) "+
			"SELECT m.id, s.user_id, now() FROM chat_members s "+
			"JOIN chat_messages m ON m.group_id = s.group_id AND m.id > s.last_delivered_id AND m.id >= s.history_from "+
			"AND m.created_at > now() - interval '"+receiptWindow+"' "+
			"WHERE s.user_id = ? AND (m.sender_id IS NULL OR m.sender_id <> s.user_id) "+
			"ON CONFLICT (message_id, user_id) DO NOTHING", userID).Error; err != nil {
		return nil, fmt.Errorf("delivery receipts could not be logged: %w", err)
	}
	var out []Delivered
	err := r.db.WithContext(ctx).Raw(
		// Only the person's own rooms, each read from the top of its index.
		"UPDATE chat_members s SET last_delivered_id = x.max_id "+
			"FROM (SELECT o.group_id, (SELECT max(m.id) FROM chat_messages m WHERE m.group_id = o.group_id) AS max_id "+
			"FROM chat_members o WHERE o.user_id = ?) x "+
			"WHERE s.group_id = x.group_id AND s.user_id = ? AND s.last_delivered_id < x.max_id "+
			"RETURNING s.group_id AS group_id, s.last_delivered_id AS delivered_id, s.last_read_id AS read_id", userID, userID).
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

// CreateMentions records who a line tags.
func (r *Repository) CreateMentions(ctx context.Context, messageID uint, userIDs []uint) error {
	if len(userIDs) == 0 {
		return nil
	}
	rows := make([]models.ChatMention, 0, len(userIDs))
	for _, id := range userIDs {
		rows = append(rows, models.ChatMention{MessageID: messageID, UserID: id})
	}
	if err := r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&rows).Error; err != nil {
		return fmt.Errorf("mentions could not be saved: %w", err)
	}
	return nil
}

// Mentions loads the tagged people of several lines, keyed by line.
func (r *Repository) Mentions(ctx context.Context, messageIDs []uint) (map[uint][]uint, error) {
	out := map[uint][]uint{}
	if len(messageIDs) == 0 {
		return out, nil
	}
	var rows []models.ChatMention
	if err := r.db.WithContext(ctx).Where("message_id IN ?", messageIDs).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("mentions could not be loaded: %w", err)
	}
	for _, m := range rows {
		out[m.MessageID] = append(out[m.MessageID], m.UserID)
	}
	return out, nil
}

// UpdateMessage rewrites a line's text and stamps it edited.
func (r *Repository) UpdateMessage(ctx context.Context, id uint, body string, mentionsAll bool) error {
	if err := r.db.WithContext(ctx).Model(&models.ChatMessage{}).Where("id = ? AND deleted_at IS NULL", id).
		Updates(map[string]any{"body": body, "mentions_all": mentionsAll, "edited_at": time.Now()}).Error; err != nil {
		return fmt.Errorf("message could not be edited: %w", err)
	}
	return nil
}

// DeleteMentions drops a line's tags before they are written again.
func (r *Repository) DeleteMentions(ctx context.Context, messageID uint) error {
	if err := r.db.WithContext(ctx).Where("message_id = ?", messageID).Delete(&models.ChatMention{}).Error; err != nil {
		return fmt.Errorf("mentions could not be cleared: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------- attachments

// CreateAttachment records a pending upload.
func (r *Repository) CreateAttachment(ctx context.Context, a *models.ChatAttachment) error {
	if err := r.db.WithContext(ctx).Create(a).Error; err != nil {
		return fmt.Errorf("attachment could not be created: %w", err)
	}
	return nil
}

// Attachment loads one row, or nil.
func (r *Repository) Attachment(ctx context.Context, id uint) (*models.ChatAttachment, error) {
	var a models.ChatAttachment
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&a).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("attachment could not be loaded: %w", err)
	}
	return &a, nil
}

// UpdateAttachment changes the given columns.
func (r *Repository) UpdateAttachment(ctx context.Context, id uint, fields map[string]any) error {
	if err := r.db.WithContext(ctx).Model(&models.ChatAttachment{}).Where("id = ?", id).Updates(fields).Error; err != nil {
		return fmt.Errorf("attachment could not be updated: %w", err)
	}
	return nil
}

// DeleteAttachmentRow removes a row for good (pending or swept uploads).
func (r *Repository) DeleteAttachmentRow(ctx context.Context, id uint) error {
	if err := r.db.WithContext(ctx).Where("id = ?", id).Delete(&models.ChatAttachment{}).Error; err != nil {
		return fmt.Errorf("attachment could not be removed: %w", err)
	}
	return nil
}

// BindAttachments ties the uploader's ready, unbound files of a room to a
// line. Anything else in the id list is silently left alone.
func (r *Repository) BindAttachments(ctx context.Context, ids []uint, uploaderID, groupID, messageID uint) error {
	if len(ids) == 0 {
		return nil
	}
	if err := r.db.WithContext(ctx).Model(&models.ChatAttachment{}).
		Where("id IN ? AND uploader_id = ? AND group_id = ? AND message_id IS NULL AND status = 'ready' AND deleted_at IS NULL", ids, uploaderID, groupID).
		Update("message_id", messageID).Error; err != nil {
		return fmt.Errorf("attachments could not be bound: %w", err)
	}
	return nil
}

// AttachmentsByMessage loads the live files of several lines.
func (r *Repository) AttachmentsByMessage(ctx context.Context, messageIDs []uint) (map[uint][]models.ChatAttachment, error) {
	out := map[uint][]models.ChatAttachment{}
	if len(messageIDs) == 0 {
		return out, nil
	}
	var rows []models.ChatAttachment
	if err := r.db.WithContext(ctx).Where("message_id IN ? AND deleted_at IS NULL", messageIDs).Order("id").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("attachments could not be loaded: %w", err)
	}
	for _, a := range rows {
		if a.MessageID != nil {
			out[*a.MessageID] = append(out[*a.MessageID], a)
		}
	}
	return out, nil
}

// SoftDeleteAttachments hides a deleted line's files.
func (r *Repository) SoftDeleteAttachments(ctx context.Context, messageID uint) error {
	if err := r.db.WithContext(ctx).Model(&models.ChatAttachment{}).
		Where("message_id = ? AND deleted_at IS NULL", messageID).
		Update("deleted_at", time.Now()).Error; err != nil {
		return fmt.Errorf("attachments could not be deleted: %w", err)
	}
	return nil
}

// OrphanAttachments lists uploads never bound to a line before the cutoff.
func (r *Repository) OrphanAttachments(ctx context.Context, before time.Time) ([]models.ChatAttachment, error) {
	var rows []models.ChatAttachment
	if err := r.db.WithContext(ctx).Where("message_id IS NULL AND deleted_at IS NULL AND created_at < ?", before).Limit(200).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("orphan attachments could not be listed: %w", err)
	}
	return rows, nil
}

// Receipt is when one person received and read one line.
type Receipt struct {
	UserID      uint       `gorm:"column:user_id"`
	DeliveredAt *time.Time `gorm:"column:delivered_at"`
	ReadAt      *time.Time `gorm:"column:read_at"`
}

// Receipts lists the logged receipts of a line.
func (r *Repository) Receipts(ctx context.Context, messageID uint) ([]Receipt, error) {
	var out []Receipt
	if err := r.db.WithContext(ctx).Raw("SELECT user_id, delivered_at, read_at FROM chat_receipts WHERE message_id = ?", messageID).Scan(&out).Error; err != nil {
		return nil, fmt.Errorf("receipts could not be loaded: %w", err)
	}
	return out, nil
}

// ClearDriveFolders forgets every room folder, after the account changed.
func (r *Repository) ClearDriveFolders(ctx context.Context) error {
	if err := r.db.WithContext(ctx).Exec("UPDATE chat_groups SET drive_folder = '' WHERE drive_folder <> ''").Error; err != nil {
		return fmt.Errorf("room folders could not be cleared: %w", err)
	}
	return nil
}
