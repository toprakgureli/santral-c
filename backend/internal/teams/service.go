// Package teams is the in-house chat: groups, direct messages, invites,
// per-room roles, soft-deleted messages, reactions, read state and a live
// event stream. Nobody sees a room they are not seated in.
package teams

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
	"github.com/toprakgureli/santral-c/backend/pkg/enums"
	"github.com/toprakgureli/santral-c/backend/pkg/errs"
)

var istanbul = time.FixedZone("+03", 3*3600)

const (
	pageSize       = 50
	maxBody        = 4000
	roleOwner      = "owner"
	roleAdmin      = "admin"
	roleMember     = "member"
	policyEveryone = "everyone"
	policyAdmins   = "admins"
)

// IActorResolver loads the acting user with roles and permissions.
type IActorResolver interface {
	GetByID(ctx context.Context, id uint) (*models.User, error)
}

// Service applies the chat rules.
type Service struct {
	repo  *Repository
	users IActorResolver
	hub   *Hub
	drive *Drive
}

// NewService builds a chat service.
func NewService(repo *Repository, users IActorResolver, hub *Hub, drive *Drive) *Service {
	return &Service{repo: repo, users: users, hub: hub, drive: drive}
}

// ---------------------------------------------------------------- views

// GroupView is one room as the viewer sees it in the list.
type GroupView struct {
	ID          uint         `json:"id"`
	Kind        string       `json:"kind"`
	Name        string       `json:"name"`
	Description string       `json:"description"`
	HasAvatar   bool         `json:"hasAvatar"`
	Version     int64        `json:"avatarVersion,omitempty"`
	PostPolicy  string       `json:"postPolicy"`
	MyRole      string       `json:"myRole"`
	CanPost     bool         `json:"canPost"`
	CanManage   bool         `json:"canManage"`
	CanAdd      bool         `json:"canAdd"`
	CanInvite   bool         `json:"canInvite"`
	CanDelete   bool         `json:"canDelete"`
	Muted       bool         `json:"muted"`
	Unread      int64        `json:"unread"`
	MemberCount int64        `json:"memberCount"`
	Peer        *Person      `json:"peer,omitempty"`
	LastMessage *MessageView `json:"lastMessage,omitempty"`
	UpdatedAt   string       `json:"updatedAt"`
}

// MemberView is one seat with the person's card.
type MemberView struct {
	Person
	Role        string `json:"role"`
	CanPost     bool   `json:"canPost"`
	JoinedAt    string `json:"joinedAt"`
	DeliveredID uint   `json:"deliveredId"`
	ReadID      uint   `json:"readId"`
}

// GroupDetail is the open room: the list card plus its members.
type GroupDetail struct {
	GroupView
	Members  []MemberView `json:"members"`
	Invited  []Person     `json:"invited"`
	Editable bool         `json:"editable"`
}

// InviteView is a pending invite as the invitee sees it.
type InviteView struct {
	ID        uint    `json:"id"`
	GroupID   uint    `json:"groupId"`
	GroupName string  `json:"groupName"`
	HasAvatar bool    `json:"hasAvatar"`
	Version   int64   `json:"avatarVersion,omitempty"`
	InvitedBy *Person `json:"invitedBy,omitempty"`
	CreatedAt string  `json:"createdAt"`
}

// ReactionView is one emoji on a message with who gave it.
type ReactionView struct {
	Emoji string   `json:"emoji"`
	Count int      `json:"count"`
	Mine  bool     `json:"mine"`
	Names []string `json:"names"`
}

// MessageView is one line as rendered.
type MessageView struct {
	ID        uint           `json:"id"`
	GroupID   uint           `json:"groupId"`
	Kind      string         `json:"kind"`
	Body      string         `json:"body"`
	Sender    *Person        `json:"sender,omitempty"`
	ReplyTo   *ReplyView     `json:"replyTo,omitempty"`
	Deleted   bool           `json:"deleted"`
	Mine      bool           `json:"mine"`
	CanDelete bool           `json:"canDelete"`
	Reactions []ReactionView `json:"reactions"`
	// Mentions are the tagged people's ids; MentionsAll is "@herkes".
	Mentions    []uint           `json:"mentions"`
	MentionsAll bool             `json:"mentionsAll"`
	EditedAt    string           `json:"editedAt,omitempty"`
	Attachments []AttachmentView `json:"attachments"`
	CreatedAt   string           `json:"createdAt"`
	// Status is set on the reader's own lines: sent, delivered or read.
	Status string   `json:"status,omitempty"`
	ReadBy []string `json:"readBy,omitempty"`
}

// ReplyView is the quoted line above a reply.
type ReplyView struct {
	ID      uint   `json:"id"`
	Body    string `json:"body"`
	Sender  string `json:"sender"`
	Deleted bool   `json:"deleted"`
}

// Overview is the whole left column in one call.
type Overview struct {
	Groups  []GroupView  `json:"groups"`
	Invites []InviteView `json:"invites"`
	Unread  int64        `json:"unread"`
}

// Event is what the live stream carries.
type Event struct {
	Type    string       `json:"type"`
	GroupID uint         `json:"groupId,omitempty"`
	Message *MessageView `json:"message,omitempty"`
	ID      uint         `json:"id,omitempty"`
	// presence: userId, online, lastSeen. receipt: groupId, userId, deliveredId, readId.
	UserID      uint   `json:"userId,omitempty"`
	Online      *bool  `json:"online,omitempty"`
	LastSeen    string `json:"lastSeen,omitempty"`
	DeliveredID uint   `json:"deliveredId,omitempty"`
	ReadID      uint   `json:"readId,omitempty"`
	// typing: groupId, userId, name. presence also carries state ("chat" or "").
	Name string `json:"name,omitempty"`
	Room *uint  `json:"room,omitempty"`
}

// ---------------------------------------------------------------- helpers

func (s *Service) actor(ctx context.Context, id uint) (*models.User, error) {
	u, err := s.users.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if u == nil || !u.Active {
		return nil, errs.Unauthorized("Oturum bulunamadı. Lütfen giriş yapın.")
	}
	if !u.Can(enums.TeamsView) {
		return nil, errs.Forbidden("Teams için yetkiniz yok.")
	}
	return u, nil
}

// seat loads the room and the actor's seat; a global chat admin may look
// into any room but has no seat unless added.
func (s *Service) seat(ctx context.Context, actor *models.User, groupID uint) (*models.ChatGroup, *models.ChatMember, error) {
	g, err := s.repo.Group(ctx, groupID)
	if err != nil {
		return nil, nil, errs.Internal(err)
	}
	if g == nil {
		return nil, nil, errs.NotFound("Grup bulunamadı.")
	}
	m, err := s.repo.Member(ctx, groupID, actor.ID)
	if err != nil {
		return nil, nil, errs.Internal(err)
	}
	if m == nil && !actor.Can(enums.TeamsAdmin) {
		return nil, nil, errs.NotFound("Grup bulunamadı.")
	}
	return g, m, nil
}

func isAdmin(m *models.ChatMember) bool {
	return m != nil && (m.Role == roleOwner || m.Role == roleAdmin)
}

func canPost(g *models.ChatGroup, m *models.ChatMember) bool {
	if m == nil {
		return false
	}
	if g.Kind == "dm" {
		return true
	}
	if isAdmin(m) {
		return true
	}
	return g.PostPolicy == policyEveryone && m.CanPost
}

// presence fills the online flag and last-seen stamp of each card and
// returns which room each person is looking at, for the caller to mark
// "in this room" where it matters.
func (s *Service) presence(ctx context.Context, people map[uint]Person) map[uint]uint {
	ids := make([]uint, 0, len(people))
	for id := range people {
		ids = append(ids, id)
	}
	online := s.hub.Online(ids)
	rooms := s.hub.Rooms(ids)
	seen, _ := s.repo.LastSeen(ctx, ids)
	for id, p := range people {
		p.Online = online[id]
		if !p.Online {
			if t, ok := seen[id]; ok {
				p.LastSeen = stamp(t)
			}
		}
		people[id] = p
	}
	return rooms
}

// status grades one of the actor's own lines against the other seats:
// read when everyone else read it, delivered when everyone else received
// it, sent otherwise. ReadBy names who has read it so far.
func (s *Service) status(actorID uint, msg *models.ChatMessage, seats []models.ChatMember, people map[uint]Person) (string, []string) {
	if msg.SenderID == nil || *msg.SenderID != actorID || msg.Kind != "text" || msg.DeletedAt != nil {
		return "", nil
	}
	others, delivered, read := 0, 0, 0
	readBy := []string{}
	for _, st := range seats {
		if st.UserID == actorID {
			continue
		}
		others++
		if st.LastReadID >= msg.ID {
			read++
			delivered++
			if p, ok := people[st.UserID]; ok {
				readBy = append(readBy, p.Name)
			}
		} else if st.LastDeliveredID >= msg.ID {
			delivered++
		}
	}
	switch {
	case others == 0:
		return "sent", nil
	case read == others:
		return "read", readBy
	case delivered == others:
		return "delivered", readBy
	default:
		return "sent", readBy
	}
}

// markDelivered moves the actor's delivery pointer and tells the room.
func (s *Service) markDelivered(ctx context.Context, actorID uint, m *models.ChatMember, messageID uint) {
	if m == nil || messageID == 0 || m.LastDeliveredID >= messageID {
		return
	}
	moved, err := s.repo.MarkDelivered(ctx, m.GroupID, actorID, messageID)
	if err != nil || !moved {
		return
	}
	s.notifyGroup(ctx, m.GroupID, Event{Type: "receipt", GroupID: m.GroupID, UserID: actorID, DeliveredID: messageID, ReadID: m.LastReadID})
}

func stamp(t time.Time) string {
	return t.In(istanbul).Format(time.RFC3339)
}

func dmKey(a, b uint) string {
	if a > b {
		a, b = b, a
	}
	return fmt.Sprintf("%d:%d", a, b)
}

// ---------------------------------------------------------------- people

// People lists the active users the actor may add, invite or message.
func (s *Service) People(ctx context.Context, actorID uint) ([]Person, error) {
	if _, err := s.actor(ctx, actorID); err != nil {
		return nil, err
	}
	out, err := s.repo.People(ctx)
	if err != nil {
		return nil, errs.Internal(err)
	}
	byID := make(map[uint]Person, len(out))
	for _, p := range out {
		byID[p.ID] = p
	}
	s.presence(ctx, byID)
	for i := range out {
		out[i] = byID[out[i].ID]
	}
	return out, nil
}

// ---------------------------------------------------------------- list

// Overview returns the actor's rooms, invites and total unread.
func (s *Service) Overview(ctx context.Context, actorID uint) (*Overview, error) {
	actor, err := s.actor(ctx, actorID)
	if err != nil {
		return nil, err
	}
	// The room list carries previews, so everything newest is now delivered.
	moved, _ := s.repo.MarkDeliveredAll(ctx, actorID)
	groups, seats, err := s.repo.MyGroups(ctx, actorID)
	if err != nil {
		return nil, errs.Internal(err)
	}
	ids := make([]uint, 0, len(groups))
	for _, g := range groups {
		ids = append(ids, g.ID)
	}
	allSeats, err := s.repo.Seats(ctx, ids)
	if err != nil {
		return nil, errs.Internal(err)
	}
	last, err := s.repo.LastMessages(ctx, ids)
	if err != nil {
		return nil, errs.Internal(err)
	}
	lastIDs := make([]uint, 0, len(last))
	for _, lm := range last {
		lastIDs = append(lastIDs, lm.ID)
	}
	lastFiles, err := s.repo.AttachmentsByMessage(ctx, lastIDs)
	if err != nil {
		return nil, errs.Internal(err)
	}
	unread, err := s.repo.Unread(ctx, actorID)
	if err != nil {
		return nil, errs.Internal(err)
	}
	counts, err := s.repo.MemberCounts(ctx, ids)
	if err != nil {
		return nil, errs.Internal(err)
	}
	peers, err := s.repo.DMPeers(ctx, actorID, ids)
	if err != nil {
		return nil, errs.Internal(err)
	}
	// People behind dm rooms and last-message senders, in one load.
	need := make([]uint, 0)
	for _, id := range peers {
		need = append(need, id)
	}
	for _, m := range last {
		if m.SenderID != nil {
			need = append(need, *m.SenderID)
		}
	}
	people, err := s.repo.PeopleByID(ctx, need)
	if err != nil {
		return nil, errs.Internal(err)
	}
	rooms := s.presence(ctx, people)
	out := &Overview{Groups: make([]GroupView, 0, len(groups)), Invites: []InviteView{}}
	for i := range groups {
		g := groups[i]
		seat := seats[g.ID]
		if seat.MarkedUnread && unread[g.ID] == 0 {
			unread[g.ID] = 1
		}
		view := s.groupView(actor, &g, &seat, unread[g.ID], counts[g.ID])
		if g.Kind == "dm" {
			if p, ok := people[peers[g.ID]]; ok {
				p.InRoom = p.Online && rooms[p.ID] == g.ID
				view.Peer = &p
				view.Name = p.Name
			}
		}
		if lm, ok := last[g.ID]; ok {
			mv := s.messageView(actor, &g, &seat, &lm, people, nil)
			mv.Status, _ = s.status(actor.ID, &lm, allSeats[g.ID], nil)
			for _, a := range lastFiles[lm.ID] {
				mv.Attachments = append(mv.Attachments, attachmentView(&a))
			}
			view.LastMessage = &mv
		}
		if !seat.Muted {
			out.Unread += unread[g.ID]
		}
		out.Groups = append(out.Groups, view)
	}
	for _, d := range moved {
		s.notifyGroup(ctx, d.GroupID, Event{Type: "receipt", GroupID: d.GroupID, UserID: actorID, DeliveredID: d.DeliveredID, ReadID: d.ReadID})
	}
	invites, err := s.repo.PendingInvites(ctx, actorID)
	if err != nil {
		return nil, errs.Internal(err)
	}
	for _, inv := range invites {
		g, err := s.repo.Group(ctx, inv.GroupID)
		if err != nil || g == nil {
			continue
		}
		iv := InviteView{ID: inv.ID, GroupID: g.ID, GroupName: g.Name, HasAvatar: g.Avatar != "", CreatedAt: stamp(inv.CreatedAt)}
		if g.Avatar != "" {
			iv.Version = g.UpdatedAt.Unix()
		}
		if inv.InvitedBy != nil {
			by, _ := s.repo.PeopleByID(ctx, []uint{*inv.InvitedBy})
			if p, ok := by[*inv.InvitedBy]; ok {
				iv.InvitedBy = &p
			}
		}
		out.Invites = append(out.Invites, iv)
	}
	return out, nil
}

func (s *Service) groupView(actor *models.User, g *models.ChatGroup, m *models.ChatMember, unread, members int64) GroupView {
	admin := isAdmin(m) || actor.Can(enums.TeamsAdmin)
	v := GroupView{
		ID:          g.ID,
		Kind:        g.Kind,
		Name:        g.Name,
		Description: g.Description,
		HasAvatar:   g.Avatar != "",
		PostPolicy:  g.PostPolicy,
		CanPost:     canPost(g, m) || (m == nil && actor.Can(enums.TeamsAdmin)),
		CanManage:   admin && g.Kind != "dm",
		CanAdd:      g.Kind != "dm" && (admin || actor.Can(enums.TeamsMemberAdd)),
		CanInvite:   g.Kind != "dm" && (admin || actor.Can(enums.TeamsMemberInvite)),
		CanDelete:   g.Kind != "dm" && ((m != nil && m.Role == roleOwner) || actor.Can(enums.TeamsAdmin)),
		Unread:      unread,
		MemberCount: members,
		UpdatedAt:   stamp(g.UpdatedAt),
	}
	if g.Avatar != "" {
		v.Version = g.UpdatedAt.Unix()
	}
	if m != nil {
		v.MyRole = m.Role
		v.Muted = m.Muted
	}
	return v
}

// ---------------------------------------------------------------- rooms

// CreateInput is what a new group needs.
type CreateInput struct {
	Name        string
	Description string
	PostPolicy  string
	MemberIDs   []uint
}

// Create opens a group with the actor as owner and the chosen members seated.
func (s *Service) Create(ctx context.Context, actorID uint, in CreateInput) (*GroupDetail, error) {
	actor, err := s.actor(ctx, actorID)
	if err != nil {
		return nil, err
	}
	if !actor.Can(enums.TeamsGroupCreate) {
		return nil, errs.Forbidden("Grup oluşturma yetkiniz yok.")
	}
	name := strings.TrimSpace(in.Name)
	if len([]rune(name)) < 2 || len([]rune(name)) > 120 {
		return nil, errs.Invalid("Grup adı 2 ile 120 karakter arasında olmalı.", nil)
	}
	policy := in.PostPolicy
	if policy != policyAdmins {
		policy = policyEveryone
	}
	g := &models.ChatGroup{Kind: "group", Name: name, Description: strings.TrimSpace(in.Description), PostPolicy: policy, CreatedBy: &actorID, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	seats := []models.ChatMember{{UserID: actorID, Role: roleOwner, CanPost: true, JoinedAt: time.Now()}}
	seen := map[uint]bool{actorID: true}
	for _, id := range in.MemberIDs {
		if id == 0 || seen[id] {
			continue
		}
		seen[id] = true
		seats = append(seats, models.ChatMember{UserID: id, Role: roleMember, CanPost: true, InvitedBy: &actorID, JoinedAt: time.Now()})
	}
	if err := s.repo.CreateGroup(ctx, g, seats); err != nil {
		return nil, errs.Internal(err)
	}
	s.system(ctx, g.ID, actor.Name+" grubu oluşturdu.")
	ids := make([]uint, 0, len(seats))
	for _, st := range seats {
		ids = append(ids, st.UserID)
	}
	s.hub.Send(ids, Event{Type: "group", GroupID: g.ID})
	return s.Detail(ctx, actorID, g.ID)
}

// OpenDM returns the direct-message room with another user, creating it.
func (s *Service) OpenDM(ctx context.Context, actorID, otherID uint) (*GroupDetail, error) {
	if _, err := s.actor(ctx, actorID); err != nil {
		return nil, err
	}
	if otherID == actorID {
		return nil, errs.Invalid("Kendinize mesaj gönderemezsiniz.", nil)
	}
	other, err := s.users.GetByID(ctx, otherID)
	if err != nil {
		return nil, err
	}
	if other == nil || !other.Active {
		return nil, errs.NotFound("Kullanıcı bulunamadı.")
	}
	key := dmKey(actorID, otherID)
	g, err := s.repo.GroupByDMKey(ctx, key)
	if err != nil {
		return nil, errs.Internal(err)
	}
	if g == nil {
		g = &models.ChatGroup{Kind: "dm", DMKey: &key, CreatedBy: &actorID, CreatedAt: time.Now(), UpdatedAt: time.Now()}
		seats := []models.ChatMember{
			{UserID: actorID, Role: roleMember, CanPost: true, JoinedAt: time.Now()},
			{UserID: otherID, Role: roleMember, CanPost: true, JoinedAt: time.Now()},
		}
		if err := s.repo.CreateGroup(ctx, g, seats); err != nil {
			return nil, errs.Internal(err)
		}
		s.hub.Send([]uint{otherID}, Event{Type: "group", GroupID: g.ID})
	}
	return s.Detail(ctx, actorID, g.ID)
}

// Detail returns the open room with members.
func (s *Service) Detail(ctx context.Context, actorID, groupID uint) (*GroupDetail, error) {
	actor, err := s.actor(ctx, actorID)
	if err != nil {
		return nil, err
	}
	g, m, err := s.seat(ctx, actor, groupID)
	if err != nil {
		return nil, err
	}
	seats, err := s.repo.Members(ctx, groupID)
	if err != nil {
		return nil, errs.Internal(err)
	}
	invited, err := s.repo.PendingInviteIDs(ctx, groupID)
	if err != nil {
		return nil, errs.Internal(err)
	}
	ids := make([]uint, 0, len(seats)+len(invited))
	for _, st := range seats {
		ids = append(ids, st.UserID)
	}
	ids = append(ids, invited...)
	people, err := s.repo.PeopleByID(ctx, ids)
	if err != nil {
		return nil, errs.Internal(err)
	}
	rooms := s.presence(ctx, people)
	for id, p := range people {
		p.InRoom = p.Online && rooms[id] == groupID
		people[id] = p
	}
	unread, err := s.repo.Unread(ctx, actorID)
	if err != nil {
		return nil, errs.Internal(err)
	}
	if m != nil && m.MarkedUnread && unread[groupID] == 0 {
		unread[groupID] = 1
	}
	d := &GroupDetail{GroupView: s.groupView(actor, g, m, unread[groupID], int64(len(seats))), Members: make([]MemberView, 0, len(seats)), Invited: []Person{}}
	d.Editable = d.CanManage
	for _, st := range seats {
		p := people[st.UserID]
		d.Members = append(d.Members, MemberView{Person: p, Role: st.Role, CanPost: canPost(g, &st), JoinedAt: stamp(st.JoinedAt), DeliveredID: st.LastDeliveredID, ReadID: st.LastReadID})
		if g.Kind == "dm" && st.UserID != actorID {
			peer := p
			d.Peer = &peer
			d.Name = p.Name
		}
	}
	sort.SliceStable(d.Members, func(i, j int) bool {
		rank := map[string]int{roleOwner: 0, roleAdmin: 1, roleMember: 2}
		if rank[d.Members[i].Role] != rank[d.Members[j].Role] {
			return rank[d.Members[i].Role] < rank[d.Members[j].Role]
		}
		return d.Members[i].Name < d.Members[j].Name
	})
	for _, id := range invited {
		if p, ok := people[id]; ok {
			d.Invited = append(d.Invited, p)
		}
	}
	return d, nil
}

// UpdateInput edits a group's text and posting policy.
type UpdateInput struct {
	Name        string
	Description string
	PostPolicy  string
}

// Update edits name, description and posting policy (owner or admin).
func (s *Service) Update(ctx context.Context, actorID, groupID uint, in UpdateInput) (*GroupDetail, error) {
	actor, err := s.actor(ctx, actorID)
	if err != nil {
		return nil, err
	}
	g, m, err := s.seat(ctx, actor, groupID)
	if err != nil {
		return nil, err
	}
	if g.Kind == "dm" || !(isAdmin(m) || actor.Can(enums.TeamsAdmin)) {
		return nil, errs.Forbidden("Grubu düzenleme yetkiniz yok.")
	}
	name := strings.TrimSpace(in.Name)
	if len([]rune(name)) < 2 || len([]rune(name)) > 120 {
		return nil, errs.Invalid("Grup adı 2 ile 120 karakter arasında olmalı.", nil)
	}
	policy := in.PostPolicy
	if policy != policyAdmins {
		policy = policyEveryone
	}
	if err := s.repo.UpdateGroup(ctx, groupID, map[string]any{"name": name, "description": strings.TrimSpace(in.Description), "post_policy": policy}); err != nil {
		return nil, errs.Internal(err)
	}
	// The Drive folder follows the group's name.
	if g.DriveFolder != "" && name != g.Name {
		renamed := *g
		renamed.Name = name
		go func(id, label string) {
			_ = s.drive.Rename(context.Background(), id, label)
		}(g.DriveFolder, s.roomFolderName(ctx, &renamed))
	}
	s.notifyGroup(ctx, groupID, Event{Type: "group", GroupID: groupID})
	return s.Detail(ctx, actorID, groupID)
}

// SetAvatar stores the group photo (already checked by the caller).
func (s *Service) SetAvatar(ctx context.Context, actorID, groupID uint, avatar string) (*GroupDetail, error) {
	actor, err := s.actor(ctx, actorID)
	if err != nil {
		return nil, err
	}
	g, m, err := s.seat(ctx, actor, groupID)
	if err != nil {
		return nil, err
	}
	if g.Kind == "dm" || !(isAdmin(m) || actor.Can(enums.TeamsAdmin)) {
		return nil, errs.Forbidden("Grubu düzenleme yetkiniz yok.")
	}
	if err := s.repo.UpdateGroup(ctx, groupID, map[string]any{"avatar": avatar}); err != nil {
		return nil, errs.Internal(err)
	}
	s.notifyGroup(ctx, groupID, Event{Type: "group", GroupID: groupID})
	return s.Detail(ctx, actorID, groupID)
}

// Avatar returns the group's stored photo (data URI or "") to a member.
func (s *Service) Avatar(ctx context.Context, actorID, groupID uint) (string, error) {
	actor, err := s.actor(ctx, actorID)
	if err != nil {
		return "", err
	}
	g, _, err := s.seat(ctx, actor, groupID)
	if err != nil {
		return "", err
	}
	return g.Avatar, nil
}

// Delete removes a group (owner or global chat admin).
func (s *Service) Delete(ctx context.Context, actorID, groupID uint) error {
	actor, err := s.actor(ctx, actorID)
	if err != nil {
		return err
	}
	g, m, err := s.seat(ctx, actor, groupID)
	if err != nil {
		return err
	}
	if g.Kind == "dm" || !((m != nil && m.Role == roleOwner) || actor.Can(enums.TeamsAdmin)) {
		return errs.Forbidden("Grubu silme yetkiniz yok.")
	}
	ids, _ := s.repo.MemberIDs(ctx, groupID)
	if err := s.repo.DeleteGroup(ctx, groupID); err != nil {
		return errs.Internal(err)
	}
	// The room's folder and everything in it go with the room.
	if g.DriveFolder != "" {
		go func(id string) {
			_ = s.drive.Delete(context.Background(), id)
		}(g.DriveFolder)
	}
	s.hub.Send(ids, Event{Type: "group", GroupID: groupID})
	return nil
}

// ---------------------------------------------------------------- members

// AddMembers seats users directly (group admin, or the global add permission).
func (s *Service) AddMembers(ctx context.Context, actorID, groupID uint, userIDs []uint) (*GroupDetail, error) {
	actor, err := s.actor(ctx, actorID)
	if err != nil {
		return nil, err
	}
	g, m, err := s.seat(ctx, actor, groupID)
	if err != nil {
		return nil, err
	}
	if g.Kind == "dm" {
		return nil, errs.Invalid("Özel mesaja üye eklenemez.", nil)
	}
	if !(isAdmin(m) || actor.Can(enums.TeamsAdmin) || actor.Can(enums.TeamsMemberAdd)) {
		return nil, errs.Forbidden("Üye ekleme yetkiniz yok. Davet gönderebilirsiniz.")
	}
	seats := make([]models.ChatMember, 0, len(userIDs))
	for _, id := range userIDs {
		if id == 0 {
			continue
		}
		seats = append(seats, models.ChatMember{GroupID: groupID, UserID: id, Role: roleMember, CanPost: true, InvitedBy: &actorID})
	}
	if err := s.repo.AddMembers(ctx, seats); err != nil {
		return nil, errs.Internal(err)
	}
	people, _ := s.repo.PeopleByID(ctx, userIDs)
	names := make([]string, 0, len(userIDs))
	for _, id := range userIDs {
		if p, ok := people[id]; ok {
			names = append(names, p.Name)
		}
	}
	if len(names) > 0 {
		s.system(ctx, groupID, actor.Name+", "+strings.Join(names, ", ")+" kişisini gruba ekledi.")
	}
	s.notifyGroup(ctx, groupID, Event{Type: "group", GroupID: groupID})
	return s.Detail(ctx, actorID, groupID)
}

// Invite opens pending invites (group admin, or the global invite permission).
func (s *Service) Invite(ctx context.Context, actorID, groupID uint, userIDs []uint) (*GroupDetail, error) {
	actor, err := s.actor(ctx, actorID)
	if err != nil {
		return nil, err
	}
	g, m, err := s.seat(ctx, actor, groupID)
	if err != nil {
		return nil, err
	}
	if g.Kind == "dm" {
		return nil, errs.Invalid("Özel mesaja davet gönderilemez.", nil)
	}
	if !(isAdmin(m) || actor.Can(enums.TeamsAdmin) || actor.Can(enums.TeamsMemberInvite)) {
		return nil, errs.Forbidden("Davet gönderme yetkiniz yok.")
	}
	if err := s.repo.CreateInvites(ctx, groupID, actorID, userIDs); err != nil {
		return nil, errs.Internal(err)
	}
	s.hub.Send(userIDs, Event{Type: "invite"})
	return s.Detail(ctx, actorID, groupID)
}

// DecideInvite accepts or declines the actor's own invite.
func (s *Service) DecideInvite(ctx context.Context, actorID, inviteID uint, accept bool) error {
	actor, err := s.actor(ctx, actorID)
	if err != nil {
		return err
	}
	inv, err := s.repo.Invite(ctx, inviteID)
	if err != nil {
		return errs.Internal(err)
	}
	if inv == nil || inv.UserID != actorID || inv.Status != "pending" {
		return errs.NotFound("Davet bulunamadı.")
	}
	if err := s.repo.DecideInvite(ctx, inv, accept); err != nil {
		return errs.Internal(err)
	}
	if accept {
		s.system(ctx, inv.GroupID, actor.Name+" daveti kabul edip gruba katıldı.")
		s.notifyGroup(ctx, inv.GroupID, Event{Type: "group", GroupID: inv.GroupID})
	}
	return nil
}

// RemoveMember unseats a user: admins remove others, anyone may leave.
func (s *Service) RemoveMember(ctx context.Context, actorID, groupID, userID uint) error {
	actor, err := s.actor(ctx, actorID)
	if err != nil {
		return err
	}
	g, m, err := s.seat(ctx, actor, groupID)
	if err != nil {
		return err
	}
	if g.Kind == "dm" {
		return errs.Invalid("Özel mesajdan çıkılamaz.", nil)
	}
	target, err := s.repo.Member(ctx, groupID, userID)
	if err != nil {
		return errs.Internal(err)
	}
	if target == nil {
		return errs.NotFound("Üye bulunamadı.")
	}
	self := userID == actorID
	if !self && !(isAdmin(m) || actor.Can(enums.TeamsAdmin)) {
		return errs.Forbidden("Üye çıkarma yetkiniz yok.")
	}
	if target.Role == roleOwner && !self {
		return errs.Forbidden("Grup sahibi çıkarılamaz.")
	}
	if target.Role == roleOwner && self {
		return errs.Invalid("Grup sahibi ayrılamaz, önce sahipliği devredin ya da grubu silin.", nil)
	}
	if err := s.repo.RemoveMember(ctx, groupID, userID); err != nil {
		return errs.Internal(err)
	}
	people, _ := s.repo.PeopleByID(ctx, []uint{userID})
	name := people[userID].Name
	if self {
		s.system(ctx, groupID, name+" gruptan ayrıldı.")
	} else {
		s.system(ctx, groupID, actor.Name+", "+name+" kişisini gruptan çıkardı.")
	}
	s.hub.Send([]uint{userID}, Event{Type: "group", GroupID: groupID})
	s.notifyGroup(ctx, groupID, Event{Type: "group", GroupID: groupID})
	return nil
}

// MemberInput edits a seat: role (owner only) and posting right (admins).
type MemberInput struct {
	Role    *string
	CanPost *bool
}

// UpdateMember changes a seat's role or posting right.
func (s *Service) UpdateMember(ctx context.Context, actorID, groupID, userID uint, in MemberInput) (*GroupDetail, error) {
	actor, err := s.actor(ctx, actorID)
	if err != nil {
		return nil, err
	}
	g, m, err := s.seat(ctx, actor, groupID)
	if err != nil {
		return nil, err
	}
	if g.Kind == "dm" || !(isAdmin(m) || actor.Can(enums.TeamsAdmin)) {
		return nil, errs.Forbidden("Üye düzenleme yetkiniz yok.")
	}
	target, err := s.repo.Member(ctx, groupID, userID)
	if err != nil {
		return nil, errs.Internal(err)
	}
	if target == nil {
		return nil, errs.NotFound("Üye bulunamadı.")
	}
	fields := map[string]any{}
	if in.Role != nil {
		owner := (m != nil && m.Role == roleOwner) || actor.Can(enums.TeamsAdmin)
		if !owner {
			return nil, errs.Forbidden("Rolleri yalnızca grup sahibi değiştirebilir.")
		}
		switch *in.Role {
		case roleAdmin, roleMember:
			if target.Role == roleOwner {
				return nil, errs.Invalid("Grup sahibinin rolü düşürülemez.", nil)
			}
			fields["role"] = *in.Role
		case roleOwner:
			// Transfer: the old owner becomes admin.
			if m != nil && m.Role == roleOwner {
				if err := s.repo.UpdateMember(ctx, groupID, actorID, map[string]any{"role": roleAdmin}); err != nil {
					return nil, errs.Internal(err)
				}
			}
			fields["role"] = roleOwner
		default:
			return nil, errs.Invalid("Geçersiz rol.", nil)
		}
	}
	if in.CanPost != nil {
		fields["can_post"] = *in.CanPost
	}
	if len(fields) > 0 {
		if err := s.repo.UpdateMember(ctx, groupID, userID, fields); err != nil {
			return nil, errs.Internal(err)
		}
	}
	s.notifyGroup(ctx, groupID, Event{Type: "group", GroupID: groupID})
	return s.Detail(ctx, actorID, groupID)
}

// SetMuted silences or restores a room for the actor only.
func (s *Service) SetMuted(ctx context.Context, actorID, groupID uint, muted bool) error {
	actor, err := s.actor(ctx, actorID)
	if err != nil {
		return err
	}
	if _, m, err := s.seat(ctx, actor, groupID); err != nil {
		return err
	} else if m == nil {
		return errs.NotFound("Grup bulunamadı.")
	}
	if err := s.repo.UpdateMember(ctx, groupID, actorID, map[string]any{"muted": muted}); err != nil {
		return errs.Internal(err)
	}
	return nil
}

// MarkRead moves the actor's read pointer.
func (s *Service) MarkRead(ctx context.Context, actorID, groupID, messageID uint) error {
	actor, err := s.actor(ctx, actorID)
	if err != nil {
		return err
	}
	_, m, err := s.seat(ctx, actor, groupID)
	if err != nil {
		return err
	}
	if m == nil {
		return nil
	}
	explicit := messageID == 0
	if explicit {
		last, err := s.repo.LastMessageID(ctx, groupID)
		if err != nil {
			return errs.Internal(err)
		}
		messageID = last
	}
	if err := s.repo.MarkRead(ctx, groupID, actorID, messageID); err != nil {
		return errs.Internal(err)
	}
	if m.LastReadID < messageID {
		s.notifyGroup(ctx, groupID, Event{Type: "receipt", GroupID: groupID, UserID: actorID, DeliveredID: messageID, ReadID: messageID})
	}
	// An explicit "mark read" or a cleared manual flag: the person's other
	// tabs drop the badge. Ordinary reading while the room is open is not
	// worth a refresh on every line.
	if explicit || m.MarkedUnread {
		s.hub.Send([]uint{actorID}, Event{Type: "group", GroupID: groupID})
	}
	return nil
}

// MarkUnread flags the room unread for the actor until they read it again.
// It does not touch read receipts: what the others saw stays seen.
func (s *Service) MarkUnread(ctx context.Context, actorID, groupID uint) error {
	actor, err := s.actor(ctx, actorID)
	if err != nil {
		return err
	}
	_, m, err := s.seat(ctx, actor, groupID)
	if err != nil {
		return err
	}
	if m == nil {
		return errs.Forbidden("Bu odada üye değilsiniz.")
	}
	if err := s.repo.MarkUnread(ctx, groupID, actorID); err != nil {
		return errs.Internal(err)
	}
	s.hub.Send([]uint{actorID}, Event{Type: "group", GroupID: groupID})
	return nil
}

// ---------------------------------------------------------------- messages

// Messages pages a room's history, oldest first within the page.
func (s *Service) Messages(ctx context.Context, actorID, groupID, beforeID uint) ([]MessageView, bool, error) {
	actor, err := s.actor(ctx, actorID)
	if err != nil {
		return nil, false, err
	}
	g, m, err := s.seat(ctx, actor, groupID)
	if err != nil {
		return nil, false, err
	}
	rows, err := s.repo.Messages(ctx, groupID, beforeID, pageSize+1)
	if err != nil {
		return nil, false, errs.Internal(err)
	}
	more := len(rows) > pageSize
	if more {
		rows = rows[:pageSize]
	}
	if beforeID == 0 && len(rows) > 0 {
		s.markDelivered(ctx, actorID, m, rows[0].ID)
	}
	views, err := s.views(ctx, actor, g, m, rows)
	if err != nil {
		return nil, false, err
	}
	// Oldest first for rendering.
	for i, j := 0, len(views)-1; i < j; i, j = i+1, j-1 {
		views[i], views[j] = views[j], views[i]
	}
	return views, more, nil
}

func (s *Service) views(ctx context.Context, actor *models.User, g *models.ChatGroup, m *models.ChatMember, rows []models.ChatMessage) ([]MessageView, error) {
	ids := make([]uint, 0, len(rows))
	need := make([]uint, 0)
	replyIDs := make([]uint, 0)
	for _, r := range rows {
		ids = append(ids, r.ID)
		if r.SenderID != nil {
			need = append(need, *r.SenderID)
		}
		if r.ReplyToID != nil {
			replyIDs = append(replyIDs, *r.ReplyToID)
		}
	}
	replies := map[uint]models.ChatMessage{}
	for _, id := range replyIDs {
		if rm, err := s.repo.Message(ctx, id); err == nil && rm != nil {
			replies[id] = *rm
			if rm.SenderID != nil {
				need = append(need, *rm.SenderID)
			}
		}
	}
	reactions, err := s.repo.Reactions(ctx, ids)
	if err != nil {
		return nil, errs.Internal(err)
	}
	for _, r := range reactions {
		need = append(need, r.UserID)
	}
	seats, err := s.repo.Members(ctx, g.ID)
	if err != nil {
		return nil, errs.Internal(err)
	}
	for _, st := range seats {
		need = append(need, st.UserID)
	}
	mentions, err := s.repo.Mentions(ctx, ids)
	if err != nil {
		return nil, errs.Internal(err)
	}
	files, err := s.repo.AttachmentsByMessage(ctx, ids)
	if err != nil {
		return nil, errs.Internal(err)
	}
	people, err := s.repo.PeopleByID(ctx, need)
	if err != nil {
		return nil, errs.Internal(err)
	}
	byMsg := map[uint][]models.ChatReaction{}
	for _, r := range reactions {
		byMsg[r.MessageID] = append(byMsg[r.MessageID], r)
	}
	out := make([]MessageView, 0, len(rows))
	for i := range rows {
		v := s.messageView(actor, g, m, &rows[i], people, byMsg[rows[i].ID])
		v.Status, v.ReadBy = s.status(actor.ID, &rows[i], seats, people)
		if ids := mentions[rows[i].ID]; len(ids) > 0 {
			v.Mentions = ids
		}
		for _, a := range files[rows[i].ID] {
			v.Attachments = append(v.Attachments, attachmentView(&a))
		}
		if rows[i].ReplyToID != nil {
			if rm, ok := replies[*rows[i].ReplyToID]; ok {
				rv := ReplyView{ID: rm.ID, Body: rm.Body, Deleted: rm.DeletedAt != nil}
				if rm.SenderID != nil {
					rv.Sender = people[*rm.SenderID].Name
				}
				if rv.Deleted {
					rv.Body = ""
				}
				v.ReplyTo = &rv
			}
		}
		out = append(out, v)
	}
	return out, nil
}

func (s *Service) messageView(actor *models.User, g *models.ChatGroup, m *models.ChatMember, r *models.ChatMessage, people map[uint]Person, reactions []models.ChatReaction) MessageView {
	v := MessageView{ID: r.ID, GroupID: r.GroupID, Kind: r.Kind, Body: r.Body, Deleted: r.DeletedAt != nil, Reactions: []ReactionView{}, Mentions: []uint{}, MentionsAll: r.MentionsAll, Attachments: []AttachmentView{}, CreatedAt: stamp(r.CreatedAt)}
	if r.SenderID != nil {
		if p, ok := people[*r.SenderID]; ok {
			sender := p
			v.Sender = &sender
		}
		v.Mine = *r.SenderID == actor.ID
	}
	if v.Deleted {
		v.Body = ""
	}
	if r.EditedAt != nil {
		v.EditedAt = stamp(*r.EditedAt)
	}
	v.CanDelete = !v.Deleted && r.Kind == "text" && (v.Mine || isAdmin(m) || actor.Can(enums.TeamsAdmin))
	grouped := map[string]*ReactionView{}
	order := []string{}
	for _, rx := range reactions {
		rv, ok := grouped[rx.Emoji]
		if !ok {
			rv = &ReactionView{Emoji: rx.Emoji, Names: []string{}}
			grouped[rx.Emoji] = rv
			order = append(order, rx.Emoji)
		}
		rv.Count++
		if rx.UserID == actor.ID {
			rv.Mine = true
		}
		if p, ok := people[rx.UserID]; ok {
			rv.Names = append(rv.Names, p.Name)
		}
	}
	for _, e := range order {
		v.Reactions = append(v.Reactions, *grouped[e])
	}
	_ = g
	return v
}

// Send posts a text line to a room the actor may write in.
func (s *Service) Send(ctx context.Context, actorID, groupID uint, body string, replyTo uint, mentionIDs []uint, mentionsAll bool, attachmentIDs []uint) (*MessageView, error) {
	actor, err := s.actor(ctx, actorID)
	if err != nil {
		return nil, err
	}
	g, m, err := s.seat(ctx, actor, groupID)
	if err != nil {
		return nil, err
	}
	if !canPost(g, m) {
		return nil, errs.Forbidden("Bu grupta yazma yetkiniz yok.")
	}
	body = strings.TrimSpace(body)
	if body == "" && len(attachmentIDs) == 0 {
		return nil, errs.Invalid("Mesaj boş olamaz.", nil)
	}
	if len([]rune(body)) > maxBody {
		return nil, errs.Invalid("Mesaj en fazla 4000 karakter olabilir.", nil)
	}
	msg := &models.ChatMessage{GroupID: groupID, SenderID: &actorID, Kind: "text", Body: body, MentionsAll: mentionsAll && g.Kind == "group", CreatedAt: time.Now()}
	if replyTo > 0 {
		rm, err := s.repo.Message(ctx, replyTo)
		if err != nil {
			return nil, errs.Internal(err)
		}
		if rm != nil && rm.GroupID == groupID {
			msg.ReplyToID = &replyTo
		}
	}
	if err := s.repo.CreateMessage(ctx, msg); err != nil {
		return nil, errs.Internal(err)
	}
	// Only seated people can be tagged; the sender tagging themself is noise.
	tagged, err := s.tagged(ctx, actorID, groupID, mentionIDs)
	if err != nil {
		return nil, err
	}
	if err := s.repo.CreateMentions(ctx, msg.ID, tagged); err != nil {
		return nil, errs.Internal(err)
	}
	files, err := s.bindAttachments(ctx, actorID, groupID, msg.ID, attachmentIDs)
	if err != nil {
		return nil, err
	}
	if body == "" && len(files) == 0 {
		_ = s.repo.SoftDeleteMessage(ctx, msg.ID, actorID)
		return nil, errs.Invalid("Ekler hazır değil, mesaj gönderilmedi.", nil)
	}
	_ = s.repo.MarkRead(ctx, groupID, actorID, msg.ID)
	view, err := s.broadcastMessage(ctx, g, msg, tagged, files)
	if err != nil {
		return nil, err
	}
	mine := *view
	mine.Mine = true
	mine.CanDelete = true
	if seats, err := s.repo.Members(ctx, groupID); err == nil {
		mine.Status, _ = s.status(actorID, msg, seats, nil)
	}
	return &mine, nil
}

// tagged keeps the seated, distinct, non-self ids of a mention list.
func (s *Service) tagged(ctx context.Context, actorID, groupID uint, mentionIDs []uint) ([]uint, error) {
	out := []uint{}
	if len(mentionIDs) == 0 {
		return out, nil
	}
	memberIDs, err := s.repo.MemberIDs(ctx, groupID)
	if err != nil {
		return nil, errs.Internal(err)
	}
	seated := map[uint]bool{}
	for _, id := range memberIDs {
		seated[id] = true
	}
	seen := map[uint]bool{}
	for _, id := range mentionIDs {
		if id != actorID && seated[id] && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out, nil
}

// EditMessage rewrites the author's own line and tells the room.
func (s *Service) EditMessage(ctx context.Context, actorID, groupID, messageID uint, body string, mentionIDs []uint, mentionsAll bool) (*MessageView, error) {
	actor, err := s.actor(ctx, actorID)
	if err != nil {
		return nil, err
	}
	g, m, err := s.seat(ctx, actor, groupID)
	if err != nil {
		return nil, err
	}
	if !canPost(g, m) {
		return nil, errs.Forbidden("Bu grupta yazma yetkiniz yok.")
	}
	msg, err := s.repo.Message(ctx, messageID)
	if err != nil {
		return nil, errs.Internal(err)
	}
	if msg == nil || msg.GroupID != groupID || msg.DeletedAt != nil {
		return nil, errs.NotFound("Mesaj bulunamadı.")
	}
	if msg.Kind != "text" || msg.SenderID == nil || *msg.SenderID != actorID {
		return nil, errs.Forbidden("Yalnızca kendi mesajınızı düzenleyebilirsiniz.")
	}
	body = strings.TrimSpace(body)
	if body == "" {
		existing, err := s.repo.AttachmentsByMessage(ctx, []uint{messageID})
		if err != nil || len(existing[messageID]) == 0 {
			return nil, errs.Invalid("Mesaj boş olamaz.", nil)
		}
	}
	if len([]rune(body)) > maxBody {
		return nil, errs.Invalid("Mesaj en fazla 4000 karakter olabilir.", nil)
	}
	mentionsAll = mentionsAll && g.Kind == "group"
	if err := s.repo.UpdateMessage(ctx, messageID, body, mentionsAll); err != nil {
		return nil, errs.Internal(err)
	}
	tags, err := s.tagged(ctx, actorID, groupID, mentionIDs)
	if err != nil {
		return nil, err
	}
	if err := s.repo.DeleteMentions(ctx, messageID); err != nil {
		return nil, errs.Internal(err)
	}
	if err := s.repo.CreateMentions(ctx, messageID, tags); err != nil {
		return nil, errs.Internal(err)
	}
	fresh, err := s.repo.Message(ctx, messageID)
	if err != nil || fresh == nil {
		return nil, errs.Internal(err)
	}
	views, err := s.views(ctx, actor, g, m, []models.ChatMessage{*fresh})
	if err != nil || len(views) == 0 {
		return nil, errs.Internal(err)
	}
	mine := views[0]
	// The room gets a neutral copy: no "mine", no delete right baked in.
	shared := mine
	shared.Mine = false
	shared.CanDelete = false
	shared.Status = ""
	shared.ReadBy = nil
	s.notifyGroup(ctx, groupID, Event{Type: "message.edited", GroupID: groupID, ID: messageID, Message: &shared})
	return &mine, nil
}

// Typing tells the other seats the actor is writing. Nothing is stored.
func (s *Service) Typing(ctx context.Context, actorID, groupID uint) error {
	actor, err := s.actor(ctx, actorID)
	if err != nil {
		return err
	}
	g, m, err := s.seat(ctx, actor, groupID)
	if err != nil {
		return err
	}
	if !canPost(g, m) {
		return nil
	}
	ids, err := s.repo.MemberIDs(ctx, groupID)
	if err != nil {
		return errs.Internal(err)
	}
	others := ids[:0]
	for _, id := range ids {
		if id != actorID {
			others = append(others, id)
		}
	}
	s.hub.Send(others, Event{Type: "typing", GroupID: groupID, UserID: actorID, Name: actor.Name})
	return nil
}

// SetPresence records which room the actor is looking at (0: none). The
// rooms involved hear about it; nobody else needs to.
func (s *Service) SetPresence(ctx context.Context, actorID uint, room uint) error {
	actor, err := s.actor(ctx, actorID)
	if err != nil {
		return err
	}
	if room != 0 {
		if _, m, err := s.seat(ctx, actor, room); err != nil || m == nil {
			room = 0
		}
	}
	changed, old := s.hub.SetRoom(actorID, room)
	if !changed {
		return nil
	}
	on := true
	if old != 0 {
		none := uint(0)
		s.notifyGroup(ctx, old, Event{Type: "presence", GroupID: old, UserID: actorID, Online: &on, Room: &none})
	}
	if room != 0 {
		r := room
		s.notifyGroup(ctx, room, Event{Type: "presence", GroupID: room, UserID: actorID, Online: &on, Room: &r})
	}
	return nil
}

// ReceiptView is one person's delivery and read time for a line.
type ReceiptView struct {
	Person
	DeliveredAt string `json:"deliveredAt,omitempty"`
	ReadAt      string `json:"readAt,omitempty"`
}

// MessageReceipts lists, for a line, when each other seat received and
// read it.
func (s *Service) MessageReceipts(ctx context.Context, actorID, groupID, messageID uint) ([]ReceiptView, error) {
	actor, err := s.actor(ctx, actorID)
	if err != nil {
		return nil, err
	}
	if _, _, err := s.seat(ctx, actor, groupID); err != nil {
		return nil, err
	}
	msg, err := s.repo.Message(ctx, messageID)
	if err != nil {
		return nil, errs.Internal(err)
	}
	if msg == nil || msg.GroupID != groupID {
		return nil, errs.NotFound("Mesaj bulunamadı.")
	}
	seats, err := s.repo.Members(ctx, groupID)
	if err != nil {
		return nil, errs.Internal(err)
	}
	logged, err := s.repo.Receipts(ctx, messageID)
	if err != nil {
		return nil, errs.Internal(err)
	}
	byUser := map[uint]Receipt{}
	for _, r := range logged {
		byUser[r.UserID] = r
	}
	ids := make([]uint, 0, len(seats))
	for _, st := range seats {
		ids = append(ids, st.UserID)
	}
	people, err := s.repo.PeopleByID(ctx, ids)
	if err != nil {
		return nil, errs.Internal(err)
	}
	out := []ReceiptView{}
	for _, st := range seats {
		if msg.SenderID != nil && st.UserID == *msg.SenderID {
			continue
		}
		v := ReceiptView{Person: people[st.UserID]}
		if r, ok := byUser[st.UserID]; ok {
			if r.DeliveredAt != nil {
				v.DeliveredAt = stamp(*r.DeliveredAt)
			}
			if r.ReadAt != nil {
				v.ReadAt = stamp(*r.ReadAt)
			}
		}
		// Pointers moved before the log existed still count, without a time.
		if v.ReadAt == "" && st.LastReadID >= messageID {
			v.ReadAt = stamp(msg.CreatedAt)
		}
		if v.DeliveredAt == "" && (st.LastDeliveredID >= messageID || v.ReadAt != "") {
			v.DeliveredAt = v.ReadAt
			if v.DeliveredAt == "" {
				v.DeliveredAt = stamp(msg.CreatedAt)
			}
		}
		out = append(out, v)
	}
	return out, nil
}

// system posts a grey line nobody wrote ("X joined").
func (s *Service) system(ctx context.Context, groupID uint, text string) {
	g, err := s.repo.Group(ctx, groupID)
	if err != nil || g == nil {
		return
	}
	msg := &models.ChatMessage{GroupID: groupID, Kind: "system", Body: text, CreatedAt: time.Now()}
	if err := s.repo.CreateMessage(ctx, msg); err != nil {
		return
	}
	_, _ = s.broadcastMessage(ctx, g, msg, nil, nil)
}

// broadcastMessage renders a fresh line neutrally and pushes it to members.
func (s *Service) broadcastMessage(ctx context.Context, g *models.ChatGroup, msg *models.ChatMessage, mentions []uint, files []models.ChatAttachment) (*MessageView, error) {
	need := []uint{}
	if msg.SenderID != nil {
		need = append(need, *msg.SenderID)
	}
	people, err := s.repo.PeopleByID(ctx, need)
	if err != nil {
		return nil, errs.Internal(err)
	}
	neutral := &models.User{}
	view := s.messageView(neutral, g, nil, msg, people, nil)
	if len(mentions) > 0 {
		view.Mentions = mentions
	}
	for i := range files {
		view.Attachments = append(view.Attachments, attachmentView(&files[i]))
	}
	view.CanDelete = false
	if msg.ReplyToID != nil {
		if rm, err := s.repo.Message(ctx, *msg.ReplyToID); err == nil && rm != nil {
			rv := ReplyView{ID: rm.ID, Body: rm.Body, Deleted: rm.DeletedAt != nil}
			if rm.SenderID != nil {
				if rp, err := s.repo.PeopleByID(ctx, []uint{*rm.SenderID}); err == nil {
					rv.Sender = rp[*rm.SenderID].Name
				}
			}
			if rv.Deleted {
				rv.Body = ""
			}
			view.ReplyTo = &rv
		}
	}
	seats, err := s.repo.Members(ctx, g.ID)
	if err != nil {
		return nil, errs.Internal(err)
	}
	ids := make([]uint, 0, len(seats))
	for _, st := range seats {
		ids = append(ids, st.UserID)
	}
	s.hub.Send(ids, Event{Type: "message", GroupID: g.ID, Message: &view})
	// Whoever has the chat open right now has received the line.
	online := s.hub.Online(ids)
	for _, st := range seats {
		if !online[st.UserID] || (msg.SenderID != nil && st.UserID == *msg.SenderID) {
			continue
		}
		if moved, err := s.repo.MarkDelivered(ctx, g.ID, st.UserID, msg.ID); err == nil && moved {
			s.hub.Send(ids, Event{Type: "receipt", GroupID: g.ID, UserID: st.UserID, DeliveredID: msg.ID, ReadID: st.LastReadID})
		}
	}
	return &view, nil
}

// DeleteMessage soft-deletes a line: its author, a group admin or a chat admin.
func (s *Service) DeleteMessage(ctx context.Context, actorID, groupID, messageID uint) error {
	actor, err := s.actor(ctx, actorID)
	if err != nil {
		return err
	}
	_, m, err := s.seat(ctx, actor, groupID)
	if err != nil {
		return err
	}
	msg, err := s.repo.Message(ctx, messageID)
	if err != nil {
		return errs.Internal(err)
	}
	if msg == nil || msg.GroupID != groupID {
		return errs.NotFound("Mesaj bulunamadı.")
	}
	if msg.Kind != "text" {
		return errs.Invalid("Sistem mesajı silinemez.", nil)
	}
	mine := msg.SenderID != nil && *msg.SenderID == actorID
	if !(mine || isAdmin(m) || actor.Can(enums.TeamsAdmin)) {
		return errs.Forbidden("Bu mesajı silme yetkiniz yok.")
	}
	if err := s.repo.SoftDeleteMessage(ctx, messageID, actorID); err != nil {
		return errs.Internal(err)
	}
	s.dropAttachments(ctx, messageID)
	s.notifyGroup(ctx, groupID, Event{Type: "message.deleted", GroupID: groupID, ID: messageID})
	return nil
}

// React toggles the actor's emoji on a line.
func (s *Service) React(ctx context.Context, actorID, groupID, messageID uint, emoji string) error {
	actor, err := s.actor(ctx, actorID)
	if err != nil {
		return err
	}
	if _, _, err := s.seat(ctx, actor, groupID); err != nil {
		return err
	}
	emoji = strings.TrimSpace(emoji)
	if emoji == "" || len([]rune(emoji)) > 4 {
		return errs.Invalid("Geçersiz tepki.", nil)
	}
	msg, err := s.repo.Message(ctx, messageID)
	if err != nil {
		return errs.Internal(err)
	}
	if msg == nil || msg.GroupID != groupID || msg.DeletedAt != nil {
		return errs.NotFound("Mesaj bulunamadı.")
	}
	if err := s.repo.ToggleReaction(ctx, messageID, actorID, emoji); err != nil {
		return errs.Internal(err)
	}
	s.notifyGroup(ctx, groupID, Event{Type: "reaction", GroupID: groupID, ID: messageID})
	return nil
}

func (s *Service) notifyGroup(ctx context.Context, groupID uint, ev Event) {
	ids, err := s.repo.MemberIDs(ctx, groupID)
	if err != nil {
		return
	}
	s.hub.Send(ids, ev)
}

// Subscribe opens the actor's live stream.
func (s *Service) Subscribe(ctx context.Context, actorID uint) (chan []byte, error) {
	if _, err := s.actor(ctx, actorID); err != nil {
		return nil, err
	}
	ch, first := s.hub.Subscribe(actorID)
	if first {
		_ = s.repo.TouchSeen(context.Background(), actorID)
		on := true
		s.hub.Broadcast(Event{Type: "presence", UserID: actorID, Online: &on})
	}
	return ch, nil
}

// Unsubscribe closes a live stream; the last tab going away means offline.
func (s *Service) Unsubscribe(actorID uint, ch chan []byte) {
	if s.hub.Unsubscribe(actorID, ch) {
		_ = s.repo.TouchSeen(context.Background(), actorID)
		off := false
		s.hub.Broadcast(Event{Type: "presence", UserID: actorID, Online: &off, LastSeen: stamp(time.Now())})
	}
}
