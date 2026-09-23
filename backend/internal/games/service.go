package games

// Mini games inside Teams rooms.
//
// A match is a card in a room: people take seats in the lobby, the host
// starts it, the server referees every move and pushes the new state over
// the room's live stream. Content (words, questions, scenarios) is data the
// administrators keep, never code. A call can pause a match, and a setting
// can keep games to breaks only. Outcomes are recorded per person.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
	"github.com/toprakgureli/santral-c/backend/internal/teams"
	"github.com/toprakgureli/santral-c/backend/pkg/enums"
	"github.com/toprakgureli/santral-c/backend/pkg/errs"
)

const (
	keyEnabled     = "games_enabled"
	keyPauseOnCall = "games_pause_on_call"
	keyBreakOnly   = "games_break_only"

	statusLobby     = "lobby"
	statusPlaying   = "playing"
	statusFinished  = "finished"
	statusCancelled = "cancelled"
)

// IActorResolver loads the acting user for authorization.
type IActorResolver interface {
	GetByID(ctx context.Context, id uint) (*models.User, error)
}

// Rooms is what the games need from the chat: who sits in a room, posting
// the match card and result lines, and the live stream.
type Rooms interface {
	IsMember(ctx context.Context, userID, groupID uint) bool
	MemberIDs(ctx context.Context, groupID uint) ([]uint, error)
	PostGame(ctx context.Context, groupID, userID, gameID uint) (uint, error)
	PostSystem(ctx context.Context, groupID uint, text string)
	Push(ids []uint, event any)
}

// Service is the games engine.
type Service struct {
	repo  *Repository
	users IActorResolver
	rooms Rooms

	mu   sync.Mutex
	live map[uint]*Match
	rnd  *rand.Rand
}

// NewService builds the engine and warms it with unfinished matches.
func NewService(repo *Repository, users IActorResolver, rooms Rooms) *Service {
	s := &Service{repo: repo, users: users, rooms: rooms, live: map[uint]*Match{}, rnd: rand.New(rand.NewSource(time.Now().UnixNano()))}
	return s
}

// ---------------------------------------------------------------- match

// Player is a seat in a live match.
type Player struct {
	UserID uint   `json:"id"`
	Name   string `json:"name"`
	Team   int    `json:"team"`
	Score  int    `json:"score"`
	Left   bool   `json:"left,omitempty"`
}

// Config is what the host chose when opening the match.
type Config struct {
	Rounds  int `json:"rounds"`
	Seconds int `json:"seconds"`
}

// Match is a live game with its kind-specific data.
type Match struct {
	mu       sync.Mutex
	G        *models.Game
	Kind     Kind
	Config   Config
	Players  []Player
	Data     any
	Deadline time.Time
	// Paused by these people (a call on their side); the clock stops.
	Paused      map[uint]bool
	pauseRemain time.Duration
	// Invited but not yet seated.
	Invited map[uint]string
	// Strokes of the current drawing, kept in memory only.
	Strokes []json.RawMessage
	// touched is the last save; idle matches are closed by the clock.
	touched time.Time
}

const (
	lobbyTTL = 30 * time.Minute
	idleTTL  = 2 * time.Hour
)

type persisted struct {
	Deadline time.Time       `json:"deadline"`
	Remain   int64           `json:"remain"`
	Paused   []uint          `json:"paused"`
	Players  []Player        `json:"players"`
	Invited  map[uint]string `json:"invited"`
	Data     json.RawMessage `json:"data"`
}

func (m *Match) player(uid uint) *Player {
	for i := range m.Players {
		if m.Players[i].UserID == uid {
			return &m.Players[i]
		}
	}
	return nil
}

// active lists seats still in the match.
func (m *Match) active() []Player {
	out := make([]Player, 0, len(m.Players))
	for _, p := range m.Players {
		if !p.Left {
			out = append(out, p)
		}
	}
	return out
}

func (m *Match) addScore(uid uint, n int) {
	if p := m.player(uid); p != nil {
		p.Score += n
	}
}

func (m *Match) setDeadline(seconds int) {
	if seconds <= 0 {
		m.Deadline = time.Time{}
		return
	}
	m.Deadline = time.Now().Add(time.Duration(seconds) * time.Second)
}

func (m *Match) secondsLeft() int {
	if m.Deadline.IsZero() {
		return 0
	}
	if len(m.Paused) > 0 {
		return int(m.pauseRemain / time.Second)
	}
	d := time.Until(m.Deadline)
	if d < 0 {
		return 0
	}
	return int(d/time.Second) + 1
}

// leaders returns the highest scorers (ties share the win).
func (m *Match) leaders() []uint {
	best := -1 << 30
	var out []uint
	for _, p := range m.active() {
		if p.Score > best {
			best = p.Score
			out = []uint{p.UserID}
		} else if p.Score == best {
			out = append(out, p.UserID)
		}
	}
	if best <= 0 {
		return nil
	}
	return out
}

// ---------------------------------------------------------------- views

// PlayerView is a seat as the browser sees it.
type PlayerView struct {
	ID    uint   `json:"id"`
	Name  string `json:"name"`
	Team  int    `json:"team"`
	Score int    `json:"score"`
	Left  bool   `json:"left,omitempty"`
}

// GameView is a match as one viewer sees it.
type GameView struct {
	ID          uint         `json:"id"`
	GroupID     uint         `json:"groupId"`
	Kind        string       `json:"kind"`
	KindName    string       `json:"kindName"`
	Status      string       `json:"status"`
	HostID      uint         `json:"hostId"`
	Players     []PlayerView `json:"players"`
	Config      Config       `json:"config"`
	Version     int          `json:"version"`
	Paused      bool         `json:"paused"`
	PausedBy    []string     `json:"pausedBy"`
	SecondsLeft int          `json:"secondsLeft"`
	Winners     []uint       `json:"winners"`
	Invited     []PlayerView `json:"invited"`
	Joined      bool         `json:"joined"`
	IsHost      bool         `json:"isHost"`
	CanManage   bool         `json:"canManage"`
	Min         int          `json:"minPlayers"`
	Max         int          `json:"maxPlayers"`
	JoinLate    bool         `json:"joinLate"`
	Data        any          `json:"data"`
	CreatedAt   string       `json:"createdAt"`
}

func (s *Service) view(m *Match, viewer uint, canManage bool) *GameView {
	v := &GameView{
		ID: m.G.ID, GroupID: m.G.GroupID, Kind: m.G.Kind, KindName: m.Kind.Meta().Name, Status: m.G.Status, HostID: m.G.HostID,
		Config: m.Config, Version: m.G.Version, Paused: len(m.Paused) > 0, PausedBy: []string{}, SecondsLeft: m.secondsLeft(),
		Winners: []uint{}, IsHost: viewer == m.G.HostID, CanManage: canManage, Min: m.Kind.Meta().MinPlayers, Max: m.Kind.Meta().MaxPlayers,
		JoinLate: m.Kind.Meta().JoinLate, CreatedAt: m.G.CreatedAt.UTC().Format(time.RFC3339),
	}
	for _, p := range m.Players {
		v.Players = append(v.Players, PlayerView{ID: p.UserID, Name: p.Name, Team: p.Team, Score: p.Score, Left: p.Left})
		if p.UserID == viewer && !p.Left {
			v.Joined = true
		}
	}
	if v.Players == nil {
		v.Players = []PlayerView{}
	}
	v.Invited = []PlayerView{}
	for uid, name := range m.Invited {
		if p := m.player(uid); p == nil || p.Left {
			v.Invited = append(v.Invited, PlayerView{ID: uid, Name: name})
		}
	}
	sort.Slice(v.Invited, func(i, j int) bool { return v.Invited[i].Name < v.Invited[j].Name })
	for uid := range m.Paused {
		if p := m.player(uid); p != nil {
			v.PausedBy = append(v.PausedBy, p.Name)
		}
	}
	_ = json.Unmarshal([]byte(m.G.Winners), &v.Winners)
	if m.G.Status == statusPlaying || m.G.Status == statusFinished {
		v.Data = m.Kind.View(m, viewer)
	}
	return v
}

// ---------------------------------------------------------------- settings

// Settings is the administrators' switchboard.
type Settings struct {
	Enabled     bool `json:"enabled"`
	PauseOnCall bool `json:"pauseOnCall"`
	BreakOnly   bool `json:"breakOnly"`
}

func (s *Service) settings(ctx context.Context) Settings {
	on := func(k string, def bool) bool {
		v := s.repo.Setting(ctx, k)
		if v == "" {
			return def
		}
		return v == "1" || v == "true"
	}
	return Settings{Enabled: on(keyEnabled, true), PauseOnCall: on(keyPauseOnCall, true), BreakOnly: on(keyBreakOnly, false)}
}

// ConfigView is what every player learns when Teams opens.
type ConfigView struct {
	Settings
	CanManage bool             `json:"canManage"`
	Kinds     []Meta           `json:"kinds"`
	Counts    map[string]int64 `json:"itemCounts"`
}

// Configuration returns the switches, the game catalogue and content counts.
func (s *Service) Configuration(ctx context.Context, actorID uint) (*ConfigView, error) {
	actor, err := s.actor(ctx, actorID, "")
	if err != nil {
		return nil, err
	}
	counts, _ := s.repo.ItemCounts(ctx)
	out := &ConfigView{Settings: s.settings(ctx), CanManage: actor.Can(enums.GamesManage), Kinds: []Meta{}, Counts: counts}
	for _, k := range kinds {
		out.Kinds = append(out.Kinds, k.Meta())
	}
	return out, nil
}

// UpdateSettings flips the switches (games.manage).
func (s *Service) UpdateSettings(ctx context.Context, actorID uint, in Settings) error {
	if _, err := s.actor(ctx, actorID, enums.GamesManage); err != nil {
		return err
	}
	flag := func(b bool) string {
		if b {
			return "1"
		}
		return "0"
	}
	for k, v := range map[string]bool{keyEnabled: in.Enabled, keyPauseOnCall: in.PauseOnCall, keyBreakOnly: in.BreakOnly} {
		if err := s.repo.SetSetting(ctx, k, flag(v)); err != nil {
			return errs.Internal(err)
		}
	}
	return nil
}

// ---------------------------------------------------------------- content

// ItemView is one piece of content for the editor.
type ItemView struct {
	ID      uint     `json:"id"`
	Kind    string   `json:"kind"`
	Text    string   `json:"text"`
	Answer  string   `json:"answer"`
	Options []string `json:"options"`
	Seconds int      `json:"seconds"`
	Active  bool     `json:"active"`
}

func itemView(it *models.GameItem) ItemView {
	v := ItemView{ID: it.ID, Kind: it.Kind, Text: it.Text, Answer: it.Answer, Seconds: it.Seconds, Active: it.Active, Options: []string{}}
	if it.Options != nil {
		_ = json.Unmarshal([]byte(*it.Options), &v.Options)
	}
	return v
}

// ItemInput is what the editor sends.
type ItemInput struct {
	Text    string
	Answer  string
	Options []string
	Seconds int
	Active  *bool
}

func validKind(kind string) bool {
	for _, k := range kinds {
		if k.Meta().Key == kind && k.Meta().ItemKind != "" {
			return true
		}
	}
	return false
}

// Items lists content of a kind (games.manage sees inactive ones too).
func (s *Service) Items(ctx context.Context, actorID uint, kind string) ([]ItemView, error) {
	actor, err := s.actor(ctx, actorID, "")
	if err != nil {
		return nil, err
	}
	if !validKind(kind) {
		return nil, errs.Invalid("Bilinmeyen oyun türü.", nil)
	}
	rows, err := s.repo.Items(ctx, kind, !actor.Can(enums.GamesManage))
	if err != nil {
		return nil, errs.Internal(err)
	}
	out := make([]ItemView, 0, len(rows))
	for i := range rows {
		out = append(out, itemView(&rows[i]))
	}
	return out, nil
}

func cleanItem(kind string, in ItemInput) (*models.GameItem, error) {
	text := strings.TrimSpace(in.Text)
	if text == "" {
		return nil, errs.Invalid("Metin boş olamaz.", nil)
	}
	if len([]rune(text)) > 1000 {
		return nil, errs.Invalid("Metin en fazla 1000 karakter olabilir.", nil)
	}
	if in.Seconds < 0 || in.Seconds > 600 {
		return nil, errs.Invalid("Süre 0 ile 600 saniye arasında olmalı.", nil)
	}
	it := &models.GameItem{Kind: kind, Text: text, Answer: strings.TrimSpace(in.Answer), Seconds: in.Seconds, Active: true, CreatedAt: time.Now()}
	if in.Active != nil {
		it.Active = *in.Active
	}
	if len(in.Options) > 0 {
		raw, _ := json.Marshal(in.Options)
		str := string(raw)
		it.Options = &str
	}
	return it, nil
}

// CreateItem adds content (games.manage).
func (s *Service) CreateItem(ctx context.Context, actorID uint, kind string, in ItemInput) (*ItemView, error) {
	if _, err := s.actor(ctx, actorID, enums.GamesManage); err != nil {
		return nil, err
	}
	if !validKind(kind) {
		return nil, errs.Invalid("Bilinmeyen oyun türü.", nil)
	}
	it, err := cleanItem(kind, in)
	if err != nil {
		return nil, err
	}
	it.CreatedBy = &actorID
	if err := s.repo.CreateItem(ctx, it); err != nil {
		return nil, errs.Internal(err)
	}
	v := itemView(it)
	return &v, nil
}

// UpdateItem edits content (games.manage).
func (s *Service) UpdateItem(ctx context.Context, actorID, id uint, in ItemInput) error {
	if _, err := s.actor(ctx, actorID, enums.GamesManage); err != nil {
		return err
	}
	it, err := cleanItem("x", in)
	if err != nil {
		return err
	}
	fields := map[string]any{"text": it.Text, "answer": it.Answer, "seconds": it.Seconds, "options": it.Options}
	if in.Active != nil {
		fields["active"] = *in.Active
	}
	if err := s.repo.UpdateItem(ctx, id, fields); err != nil {
		return errs.Internal(err)
	}
	return nil
}

// DeleteItem removes content (games.manage).
func (s *Service) DeleteItem(ctx context.Context, actorID, id uint) error {
	if _, err := s.actor(ctx, actorID, enums.GamesManage); err != nil {
		return err
	}
	if err := s.repo.DeleteItem(ctx, id); err != nil {
		return errs.Internal(err)
	}
	return nil
}

// ImportItems reads a spreadsheet: one row per item, columns text, answer,
// seconds, options (options separated by |). A header row is skipped.
func (s *Service) ImportItems(ctx context.Context, actorID uint, kind, filename string, data []byte) (int, error) {
	if _, err := s.actor(ctx, actorID, enums.GamesManage); err != nil {
		return 0, err
	}
	if !validKind(kind) {
		return 0, errs.Invalid("Bilinmeyen oyun türü.", nil)
	}
	rows, err := readRows(filename, data)
	if err != nil {
		return 0, errs.Invalid("Dosya okunamadı: "+err.Error(), nil)
	}
	added := 0
	for i, row := range rows {
		cell := func(n int) string {
			if n < len(row) {
				return strings.TrimSpace(row[n])
			}
			return ""
		}
		text := cell(0)
		if text == "" {
			continue
		}
		low := strings.ToLower(text)
		if i == 0 && (low == "metin" || low == "text" || low == "soru" || low == "kelime") {
			continue
		}
		in := ItemInput{Text: text, Answer: cell(1)}
		if n, err := strconv.Atoi(cell(2)); err == nil {
			in.Seconds = n
		}
		if opts := cell(3); opts != "" {
			for _, o := range strings.Split(opts, "|") {
				if o = strings.TrimSpace(o); o != "" {
					in.Options = append(in.Options, o)
				}
			}
		}
		it, err := cleanItem(kind, in)
		if err != nil {
			continue
		}
		it.CreatedBy = &actorID
		if err := s.repo.CreateItem(ctx, it); err != nil {
			return added, errs.Internal(err)
		}
		added++
	}
	return added, nil
}

// ---------------------------------------------------------------- lifecycle

func (s *Service) actor(ctx context.Context, id uint, need enums.Permission) (*models.User, error) {
	u, err := s.users.GetByID(ctx, id)
	if err != nil || u == nil {
		return nil, errs.Forbidden("Oturum bulunamadı.")
	}
	if need != "" && !u.Can(need) {
		return nil, errs.Forbidden("Bu işlem için yetkiniz yok.")
	}
	return u, nil
}

func (s *Service) playable(ctx context.Context, actor *models.User) error {
	if !actor.Can(enums.GamesPlay) {
		return errs.Forbidden("Oyunlara katılma yetkiniz yok.")
	}
	st := s.settings(ctx)
	if !st.Enabled {
		return errs.Forbidden("Mini oyunlar şu an kapalı.")
	}
	if st.BreakOnly && s.repo.PresenceState(ctx, actor.ID) != "break" {
		return errs.Forbidden("Oyunlar yalnızca molada oynanabilir. Önce mola başlat.")
	}
	return nil
}

// Warm loads unfinished matches after a restart.
func (s *Service) Warm(ctx context.Context) {
	rows, err := s.repo.OpenGames(ctx)
	if err != nil {
		log.Printf("games: could not warm: %v", err)
		return
	}
	for i := range rows {
		if _, err := s.load(ctx, &rows[i]); err != nil {
			log.Printf("games: match %d could not be restored: %v", rows[i].ID, err)
		}
	}
}

func (s *Service) load(ctx context.Context, g *models.Game) (*Match, error) {
	kind := kindOf(g.Kind)
	if kind == nil {
		return nil, fmt.Errorf("unknown kind %q", g.Kind)
	}
	m := &Match{G: g, Kind: kind, Paused: map[uint]bool{}, Invited: map[uint]string{}, Data: kind.NewState()}
	_ = json.Unmarshal([]byte(g.Config), &m.Config)
	var p persisted
	if err := json.Unmarshal([]byte(g.State), &p); err == nil {
		m.Players = p.Players
		if p.Invited != nil {
			m.Invited = p.Invited
		}
		if len(p.Data) > 0 {
			_ = json.Unmarshal(p.Data, m.Data)
		}
		if !p.Deadline.IsZero() {
			m.Deadline = p.Deadline
			// The clock did not run while the server was down.
			if p.Remain > 0 {
				m.Deadline = time.Now().Add(time.Duration(p.Remain))
			}
		}
	}
	if len(m.Players) == 0 {
		seats, err := s.repo.Players(ctx, g.ID)
		if err == nil {
			ids := make([]uint, 0, len(seats))
			for _, st := range seats {
				ids = append(ids, st.UserID)
			}
			names, _ := s.repo.Names(ctx, ids)
			for _, st := range seats {
				m.Players = append(m.Players, Player{UserID: st.UserID, Name: names[st.UserID], Team: st.Team, Score: st.Score})
			}
		}
	}
	s.mu.Lock()
	s.live[g.ID] = m
	s.mu.Unlock()
	return m, nil
}

func (s *Service) match(ctx context.Context, id uint) (*Match, error) {
	s.mu.Lock()
	m, ok := s.live[id]
	s.mu.Unlock()
	if ok {
		return m, nil
	}
	g, err := s.repo.Game(ctx, id)
	if err != nil {
		return nil, errs.Internal(err)
	}
	if g == nil {
		return nil, errs.NotFound("Oyun bulunamadı.")
	}
	m, err = s.load(ctx, g)
	if err != nil {
		return nil, errs.Internal(err)
	}
	return m, nil
}

// save persists a match and bumps its version. Called with m.mu held.
func (s *Service) save(ctx context.Context, m *Match) {
	m.G.Version++
	m.touched = time.Now()
	data, _ := json.Marshal(m.Data)
	p := persisted{Deadline: m.Deadline, Players: m.Players, Invited: m.Invited, Data: data}
	if !m.Deadline.IsZero() {
		p.Remain = int64(time.Until(m.Deadline))
		if len(m.Paused) > 0 {
			p.Remain = int64(m.pauseRemain)
		}
	}
	for uid := range m.Paused {
		p.Paused = append(p.Paused, uid)
	}
	raw, _ := json.Marshal(p)
	m.G.State = string(raw)
	if err := s.repo.SaveGame(ctx, m.G); err != nil {
		log.Printf("games: match %d could not be saved: %v", m.G.ID, err)
	}
}

// broadcast tells the room the match changed. Called with m.mu held or not.
func (s *Service) broadcast(ctx context.Context, m *Match) {
	ids, err := s.rooms.MemberIDs(ctx, m.G.GroupID)
	if err != nil {
		return
	}
	s.rooms.Push(ids, teams.Event{Type: "game", GroupID: m.G.GroupID, GameID: m.G.ID, ID: uint(m.G.Version)})
}

// push sends a light payload (strokes, frames) to the room without a save.
func (s *Service) push(ctx context.Context, m *Match, typ string, payload any) {
	ids, err := s.rooms.MemberIDs(ctx, m.G.GroupID)
	if err != nil {
		return
	}
	raw, _ := json.Marshal(payload)
	s.rooms.Push(ids, teams.Event{Type: typ, GroupID: m.G.GroupID, GameID: m.G.ID, Payload: raw})
}

// CreateInput is what the host picks.
type CreateInput struct {
	Kind    string
	Rounds  int
	Seconds int
}

// Create opens a match in a room and posts its card.
func (s *Service) Create(ctx context.Context, actorID, groupID uint, in CreateInput) (*GameView, error) {
	actor, err := s.actor(ctx, actorID, "")
	if err != nil {
		return nil, err
	}
	if err := s.playable(ctx, actor); err != nil {
		return nil, err
	}
	if !s.rooms.IsMember(ctx, actorID, groupID) {
		return nil, errs.Forbidden("Bu odada değilsiniz.")
	}
	kind := kindOf(in.Kind)
	if kind == nil {
		return nil, errs.Invalid("Bilinmeyen oyun.", nil)
	}
	meta := kind.Meta()
	if meta.ItemKind != "" {
		items, err := s.repo.Items(ctx, meta.ItemKind, true)
		if err != nil {
			return nil, errs.Internal(err)
		}
		if len(items) < meta.MinItems {
			return nil, errs.Invalid(fmt.Sprintf("%s için en az %d içerik gerekli, şu an %d var. Yönetim > Mini oyunlar ekranından ekleyin.", meta.Name, meta.MinItems, len(items)), nil)
		}
	}
	open, err := s.repo.OpenGamesInGroup(ctx, groupID)
	if err != nil {
		return nil, errs.Internal(err)
	}
	if len(open) >= 3 {
		return nil, errs.Invalid("Bu odada zaten açık üç oyun var. Oyun başlat penceresindeki listeden birini kapatın; boş lobiler 30 dakika sonra kendiliğinden kapanır.", nil)
	}
	cfg := Config{Rounds: meta.DefaultRounds, Seconds: meta.DefaultSeconds}
	if in.Rounds > 0 && in.Rounds <= 30 {
		cfg.Rounds = in.Rounds
	}
	if in.Seconds >= 5 && in.Seconds <= 600 {
		cfg.Seconds = in.Seconds
	}
	cfgRaw, _ := json.Marshal(cfg)
	g := &models.Game{GroupID: groupID, Kind: meta.Key, Status: statusLobby, HostID: actorID, Config: string(cfgRaw), State: "{}", Winners: "[]", CreatedAt: time.Now()}
	if err := s.repo.CreateGame(ctx, g); err != nil {
		return nil, errs.Internal(err)
	}
	if err := s.repo.AddPlayer(ctx, &models.GamePlayer{GameID: g.ID, UserID: actorID, JoinedAt: time.Now()}); err != nil {
		return nil, errs.Internal(err)
	}
	m := &Match{G: g, Kind: kind, Config: cfg, Paused: map[uint]bool{}, Invited: map[uint]string{}, Data: kind.NewState(), Players: []Player{{UserID: actorID, Name: actor.Name}}}
	s.mu.Lock()
	s.live[g.ID] = m
	s.mu.Unlock()
	// Without its card in the room nobody can find the lobby: fail loudly.
	mid, err := s.rooms.PostGame(ctx, groupID, actorID, g.ID)
	if err != nil {
		g.Status = statusCancelled
		_ = s.repo.SaveGame(ctx, g)
		s.forget(g.ID)
		return nil, errs.Internal(fmt.Errorf("game card could not be posted: %w", err))
	}
	g.MessageID = &mid
	m.mu.Lock()
	s.save(ctx, m)
	m.mu.Unlock()
	s.broadcast(ctx, m)
	return s.view(m, actorID, actor.Can(enums.GamesManage)), nil
}

// Get returns a match as the viewer sees it.
func (s *Service) Get(ctx context.Context, actorID, gameID uint) (*GameView, error) {
	actor, err := s.actor(ctx, actorID, "")
	if err != nil {
		return nil, err
	}
	m, err := s.match(ctx, gameID)
	if err != nil {
		return nil, err
	}
	if !s.rooms.IsMember(ctx, actorID, m.G.GroupID) && !actor.Can(enums.TeamsAdmin) {
		return nil, errs.Forbidden("Bu odada değilsiniz.")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return s.view(m, actorID, actor.Can(enums.GamesManage)), nil
}

// Open lists a room's unfinished matches.
func (s *Service) Open(ctx context.Context, actorID, groupID uint) ([]GameView, error) {
	actor, err := s.actor(ctx, actorID, "")
	if err != nil {
		return nil, err
	}
	if !s.rooms.IsMember(ctx, actorID, groupID) {
		return nil, errs.Forbidden("Bu odada değilsiniz.")
	}
	rows, err := s.repo.OpenGamesInGroup(ctx, groupID)
	if err != nil {
		return nil, errs.Internal(err)
	}
	out := []GameView{}
	for i := range rows {
		m, err := s.match(ctx, rows[i].ID)
		if err != nil {
			continue
		}
		m.mu.Lock()
		out = append(out, *s.view(m, actorID, actor.Can(enums.GamesManage)))
		m.mu.Unlock()
	}
	return out, nil
}

// Join takes a seat.
func (s *Service) Join(ctx context.Context, actorID, gameID uint) (*GameView, error) {
	actor, err := s.actor(ctx, actorID, "")
	if err != nil {
		return nil, err
	}
	if err := s.playable(ctx, actor); err != nil {
		return nil, err
	}
	m, err := s.match(ctx, gameID)
	if err != nil {
		return nil, err
	}
	if !s.rooms.IsMember(ctx, actorID, m.G.GroupID) {
		return nil, errs.Forbidden("Bu odada değilsiniz.")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	meta := m.Kind.Meta()
	if m.G.Status != statusLobby && !(m.G.Status == statusPlaying && meta.JoinLate) {
		return nil, errs.Invalid("Oyun başladı, katılım kapalı.", nil)
	}
	if p := m.player(actorID); p != nil {
		if p.Left {
			p.Left = false
		}
	} else {
		if meta.MaxPlayers > 0 && len(m.active()) >= meta.MaxPlayers {
			return nil, errs.Invalid("Koltuklar dolu.", nil)
		}
		m.Players = append(m.Players, Player{UserID: actorID, Name: actor.Name})
		delete(m.Invited, actorID)
		_ = s.repo.AddPlayer(ctx, &models.GamePlayer{GameID: m.G.ID, UserID: actorID, JoinedAt: time.Now()})
		if m.G.Status == statusPlaying {
			if j, ok := m.Kind.(lateJoiner); ok {
				j.Joined(m, s, ctx, actorID)
			}
		}
	}
	s.save(ctx, m)
	s.broadcast(ctx, m)
	return s.view(m, actorID, actor.Can(enums.GamesManage)), nil
}

// Leave frees a seat. During play a two-seat game is forfeited; in a
// bigger one the person simply drops out.
func (s *Service) Leave(ctx context.Context, actorID, gameID uint) (*GameView, error) {
	actor, err := s.actor(ctx, actorID, "")
	if err != nil {
		return nil, err
	}
	m, err := s.match(ctx, gameID)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	p := m.player(actorID)
	if p == nil || p.Left {
		return s.view(m, actorID, actor.Can(enums.GamesManage)), nil
	}
	switch m.G.Status {
	case statusLobby:
		out := m.Players[:0]
		for _, q := range m.Players {
			if q.UserID != actorID {
				out = append(out, q)
			}
		}
		m.Players = out
		_ = s.repo.RemovePlayer(ctx, m.G.ID, actorID)
		if len(m.Players) == 0 {
			m.G.Status = statusCancelled
		} else if m.G.HostID == actorID {
			m.G.HostID = m.Players[0].UserID
		}
	case statusPlaying:
		p.Left = true
		delete(m.Paused, actorID)
		if m.Kind.Meta().MaxPlayers == 2 {
			var winner []uint
			for _, q := range m.active() {
				winner = append(winner, q.UserID)
			}
			s.finish(ctx, m, winner, actor.Name+" oyundan ayrıldı")
		} else if len(m.active()) < m.Kind.Meta().MinPlayers {
			s.finish(ctx, m, m.leaders(), "yeterli oyuncu kalmadı")
		} else {
			m.Kind.Left(m, s, ctx, actorID)
		}
	}
	s.save(ctx, m)
	s.broadcast(ctx, m)
	return s.view(m, actorID, actor.Can(enums.GamesManage)), nil
}

// Invite asks people in the room to take a seat; each gets a card that
// waits until they act on it.
func (s *Service) Invite(ctx context.Context, actorID, gameID uint, userIDs []uint) (*GameView, error) {
	actor, err := s.actor(ctx, actorID, "")
	if err != nil {
		return nil, err
	}
	m, err := s.match(ctx, gameID)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if p := m.player(actorID); p == nil || p.Left {
		return nil, errs.Forbidden("Davet için oyunda oturuyor olmalısınız.")
	}
	if m.G.Status != statusLobby && !(m.G.Status == statusPlaying && m.Kind.Meta().JoinLate) {
		return nil, errs.Invalid("Oyun başladı, davet gönderilemez.", nil)
	}
	names, _ := s.repo.Names(ctx, userIDs)
	targets := []uint{}
	for _, uid := range userIDs {
		if uid == actorID || !s.rooms.IsMember(ctx, uid, m.G.GroupID) {
			continue
		}
		if p := m.player(uid); p != nil && !p.Left {
			continue
		}
		m.Invited[uid] = names[uid]
		targets = append(targets, uid)
	}
	if len(targets) > 0 {
		payload, _ := json.Marshal(map[string]any{"kindName": m.Kind.Meta().Name, "kind": m.G.Kind})
		s.rooms.Push(targets, teams.Event{Type: "game.invite", GroupID: m.G.GroupID, GameID: m.G.ID, UserID: actorID, Name: actor.Name, Payload: payload})
	}
	s.save(ctx, m)
	s.broadcast(ctx, m)
	return s.view(m, actorID, actor.Can(enums.GamesManage)), nil
}

// Start begins the match (host or games.manage).
func (s *Service) Start(ctx context.Context, actorID, gameID uint) (*GameView, error) {
	actor, err := s.actor(ctx, actorID, "")
	if err != nil {
		return nil, err
	}
	m, err := s.match(ctx, gameID)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.G.Status != statusLobby {
		return nil, errs.Invalid("Oyun zaten başladı.", nil)
	}
	if m.G.HostID != actorID && !actor.Can(enums.GamesManage) {
		return nil, errs.Forbidden("Oyunu yalnızca kurucu başlatabilir.")
	}
	meta := m.Kind.Meta()
	if len(m.active()) < meta.MinPlayers {
		return nil, errs.Invalid(fmt.Sprintf("En az %d oyuncu gerekli.", meta.MinPlayers), nil)
	}
	if err := m.Kind.Start(m, s, ctx); err != nil {
		return nil, err
	}
	now := time.Now()
	m.G.Status = statusPlaying
	m.G.StartedAt = &now
	s.save(ctx, m)
	s.broadcast(ctx, m)
	return s.view(m, actorID, actor.Can(enums.GamesManage)), nil
}

// Cancel scraps a match without a record (host or games.manage).
func (s *Service) Cancel(ctx context.Context, actorID, gameID uint) error {
	actor, err := s.actor(ctx, actorID, "")
	if err != nil {
		return err
	}
	m, err := s.match(ctx, gameID)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.G.Status == statusFinished || m.G.Status == statusCancelled {
		return nil
	}
	if m.G.HostID != actorID && !actor.Can(enums.GamesManage) {
		return errs.Forbidden("Oyunu yalnızca kurucu iptal edebilir.")
	}
	now := time.Now()
	m.G.Status = statusCancelled
	m.G.FinishedAt = &now
	s.save(ctx, m)
	s.broadcast(ctx, m)
	s.forget(m.G.ID)
	return nil
}

// Pause stops or restarts the clock for a player who is on a call.
func (s *Service) Pause(ctx context.Context, actorID, gameID uint, paused bool) (*GameView, error) {
	actor, err := s.actor(ctx, actorID, "")
	if err != nil {
		return nil, err
	}
	m, err := s.match(ctx, gameID)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.G.Status != statusPlaying || m.player(actorID) == nil {
		return s.view(m, actorID, actor.Can(enums.GamesManage)), nil
	}
	if !s.settings(ctx).PauseOnCall && paused {
		return s.view(m, actorID, actor.Can(enums.GamesManage)), nil
	}
	was := len(m.Paused) > 0
	if paused {
		m.Paused[actorID] = true
	} else {
		delete(m.Paused, actorID)
	}
	now := len(m.Paused) > 0
	if !was && now {
		m.pauseRemain = time.Until(m.Deadline)
		if m.Deadline.IsZero() {
			m.pauseRemain = 0
		}
	}
	if was && !now && !m.Deadline.IsZero() {
		m.Deadline = time.Now().Add(m.pauseRemain)
	}
	s.save(ctx, m)
	s.broadcast(ctx, m)
	return s.view(m, actorID, actor.Can(enums.GamesManage)), nil
}

// Act applies a player's move.
func (s *Service) Act(ctx context.Context, actorID, gameID uint, action string, payload json.RawMessage) (*GameView, error) {
	actor, err := s.actor(ctx, actorID, "")
	if err != nil {
		return nil, err
	}
	m, err := s.match(ctx, gameID)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.G.Status != statusPlaying {
		return nil, errs.Invalid("Oyun oynanmıyor.", nil)
	}
	p := m.player(actorID)
	if p == nil || p.Left {
		return nil, errs.Forbidden("Bu oyunda oturmuyorsunuz.")
	}
	if len(m.Paused) > 0 && action != "move" {
		return nil, errs.Invalid("Oyun duraklatıldı.", nil)
	}
	changed, err := m.Kind.Act(m, s, ctx, actorID, action, payload)
	if err != nil {
		return nil, err
	}
	if changed {
		s.save(ctx, m)
		s.broadcast(ctx, m)
	}
	return s.view(m, actorID, actor.Can(enums.GamesManage)), nil
}

// Move is the hockey mallet's fast lane: no user lookup, no view built,
// just the seat check and the physics. Called dozens of times a second.
func (s *Service) Move(ctx context.Context, actorID, gameID uint, x, y float64) error {
	s.mu.Lock()
	m, ok := s.live[gameID]
	s.mu.Unlock()
	if !ok {
		var err error
		if m, err = s.match(ctx, gameID); err != nil {
			return err
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.G.Status != statusPlaying || len(m.Paused) > 0 {
		return nil
	}
	if p := m.player(actorID); p == nil || p.Left {
		return errs.Forbidden("Bu oyunda oturmuyorsunuz.")
	}
	raw, _ := json.Marshal(map[string]float64{"x": x, "y": y})
	_, err := m.Kind.Act(m, s, ctx, actorID, "move", raw)
	return err
}

// finish closes a match, records the outcome and tells the room. Called
// with m.mu held.
func (s *Service) finish(ctx context.Context, m *Match, winners []uint, note string) {
	if m.G.Status == statusFinished {
		return
	}
	now := time.Now()
	m.G.Status = statusFinished
	m.G.FinishedAt = &now
	m.Deadline = time.Time{}
	m.Paused = map[uint]bool{}
	sort.Slice(winners, func(i, j int) bool { return winners[i] < winners[j] })
	raw, _ := json.Marshal(winners)
	if winners == nil {
		raw = []byte("[]")
	}
	m.G.Winners = string(raw)
	won := map[uint]bool{}
	for _, w := range winners {
		won[w] = true
	}
	scores := map[uint]int{}
	rows := make([]models.GameResult, 0, len(m.Players))
	for _, p := range m.Players {
		scores[p.UserID] = p.Score
		rows = append(rows, models.GameResult{GameID: m.G.ID, UserID: p.UserID, Kind: m.G.Kind, Won: won[p.UserID], Score: p.Score, FinishedAt: now})
	}
	_ = s.repo.SetScores(ctx, m.G.ID, scores)
	_ = s.repo.RecordResults(ctx, rows)
	names := []string{}
	for _, w := range winners {
		if p := m.player(w); p != nil {
			names = append(names, p.Name)
		}
	}
	text := "🎮 " + m.Kind.Meta().Name + " bitti"
	switch {
	case len(names) == 1:
		text += ": " + names[0] + " kazandı"
	case len(names) > 1:
		text += ": " + strings.Join(names, ", ") + " berabere"
	default:
		text += ", kazanan çıkmadı"
	}
	if note != "" {
		text += " (" + note + ")"
	}
	s.rooms.PostSystem(ctx, m.G.GroupID, text)
	go s.forgetLater(m.G.ID)
}

func (s *Service) forget(id uint) {
	s.mu.Lock()
	delete(s.live, id)
	s.mu.Unlock()
}

func (s *Service) forgetLater(id uint) {
	time.Sleep(10 * time.Minute)
	s.forget(id)
}

// StartClock runs the one-second referee and the hockey simulation.
func (s *Service) StartClock(ctx context.Context) {
	s.Warm(ctx)
	go func() {
		t := time.NewTicker(time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-t.C:
				s.tick(ctx, now)
			}
		}
	}()
	go func() {
		t := time.NewTicker(time.Second / 30)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				s.frame(ctx)
			}
		}
	}()
}

func (s *Service) tick(ctx context.Context, now time.Time) {
	s.mu.Lock()
	list := make([]*Match, 0, len(s.live))
	for _, m := range s.live {
		list = append(list, m)
	}
	s.mu.Unlock()
	for _, m := range list {
		m.mu.Lock()
		// Forgotten matches: a lobby nobody started, a game nobody touched
		// for hours, a bingo from another day.
		if m.touched.IsZero() {
			m.touched = now
		}
		stale := false
		switch m.G.Status {
		case statusLobby:
			stale = now.Sub(m.G.CreatedAt) > lobbyTTL
		case statusPlaying:
			if m.G.Kind == "bingo" {
				stale = m.G.StartedAt != nil && m.G.StartedAt.In(istanbul).Format("2006-01-02") != now.In(istanbul).Format("2006-01-02")
			} else {
				stale = len(m.Paused) == 0 && now.Sub(m.touched) > idleTTL
			}
		}
		if stale {
			if m.G.Status == statusLobby {
				m.G.Status = statusCancelled
				t := now
				m.G.FinishedAt = &t
			} else {
				s.finish(ctx, m, m.leaders(), "hareketsizlikten kapandı")
			}
			s.save(ctx, m)
			m.mu.Unlock()
			s.broadcast(ctx, m)
			s.forget(m.G.ID)
			continue
		}
		if m.G.Status == statusPlaying && len(m.Paused) == 0 && !m.Deadline.IsZero() && now.After(m.Deadline) {
			m.Deadline = time.Time{}
			m.Kind.Timeout(m, s, ctx)
			s.save(ctx, m)
			m.mu.Unlock()
			s.broadcast(ctx, m)
			continue
		}
		m.mu.Unlock()
	}
}

func (s *Service) frame(ctx context.Context) {
	s.mu.Lock()
	list := make([]*Match, 0)
	for _, m := range s.live {
		if m.G.Kind == "hockey" && m.G.Status == statusPlaying {
			list = append(list, m)
		}
	}
	s.mu.Unlock()
	for _, m := range list {
		m.mu.Lock()
		if len(m.Paused) > 0 {
			m.mu.Unlock()
			continue
		}
		h, ok := m.Kind.(*hockeyKind)
		if !ok {
			m.mu.Unlock()
			continue
		}
		done := h.step(m, s, ctx)
		payload := h.frame(m)
		if done {
			s.save(ctx, m)
		}
		m.mu.Unlock()
		s.push(ctx, m, "game.frame", payload)
		if done {
			s.broadcast(ctx, m)
		}
	}
}

// OnEscalation marks bingo cards when a matching category is logged.
func (s *Service) OnEscalation(userID uint, category string) {
	s.mu.Lock()
	list := make([]*Match, 0)
	for _, m := range s.live {
		if m.G.Kind == "bingo" && m.G.Status == statusPlaying {
			list = append(list, m)
		}
	}
	s.mu.Unlock()
	ctx := context.Background()
	for _, m := range list {
		m.mu.Lock()
		b, ok := m.Kind.(*bingoKind)
		if ok && m.player(userID) != nil && b.autoMark(m, s, ctx, userID, category) {
			s.save(ctx, m)
			m.mu.Unlock()
			s.broadcast(ctx, m)
			continue
		}
		m.mu.Unlock()
	}
}

// ---------------------------------------------------------------- records

// LeaderView is one row of the table.
type LeaderView struct {
	LeaderRow
	Name string `json:"name"`
}

// Leaderboard tallies a period (month or all), optionally one kind.
func (s *Service) Leaderboard(ctx context.Context, actorID uint, period, kind string) ([]LeaderView, error) {
	if _, err := s.actor(ctx, actorID, ""); err != nil {
		return nil, err
	}
	since := time.Time{}
	if period != "all" {
		now := time.Now()
		since = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	}
	rows, err := s.repo.Leaderboard(ctx, since, kind)
	if err != nil {
		return nil, errs.Internal(err)
	}
	ids := make([]uint, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.UserID)
	}
	names, _ := s.repo.Names(ctx, ids)
	out := make([]LeaderView, 0, len(rows))
	for _, r := range rows {
		out = append(out, LeaderView{LeaderRow: r, Name: names[r.UserID]})
	}
	return out, nil
}

// Record is one person's tally for the profile.
type Record struct {
	Played int64       `json:"played"`
	Wins   int64       `json:"wins"`
	Points int64       `json:"points"`
	Kinds  []UserTally `json:"kinds"`
}

// UserRecord tallies one person's games.
func (s *Service) UserRecord(ctx context.Context, actorID, userID uint) (*Record, error) {
	if _, err := s.actor(ctx, actorID, ""); err != nil {
		return nil, err
	}
	rows, err := s.repo.UserRecord(ctx, userID)
	if err != nil {
		return nil, errs.Internal(err)
	}
	out := &Record{Kinds: []UserTally{}}
	for _, r := range rows {
		out.Played += r.Played
		out.Wins += r.Wins
		out.Points += r.Points
		if k := kindOf(r.Kind); k != nil {
			r.Kind = k.Meta().Name
		}
		out.Kinds = append(out.Kinds, r)
	}
	return out, nil
}

var errNotYourTurn = errors.New("sıra sizde değil")

var istanbul = time.FixedZone("+03", 3*3600)
