package games

// The games themselves. Each kind keeps its own data struct inside the
// match, answers moves, reacts to the clock and renders a viewer-specific
// picture. Rules live here; content comes from game_items.

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"
	"unicode"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
	"github.com/toprakgureli/santral-c/backend/pkg/errs"
)

// Meta describes a kind to the catalogue and the lobby.
type Meta struct {
	Key            string `json:"key"`
	Name           string `json:"name"`
	Tagline        string `json:"tagline"`
	How            string `json:"how"`
	MinPlayers     int    `json:"minPlayers"`
	MaxPlayers     int    `json:"maxPlayers"` // 0: no limit
	ItemKind       string `json:"itemKind"`   // content table this kind draws from, "" for none
	ItemLabel      string `json:"itemLabel"`  // what one item is called in the editor
	ItemHint       string `json:"itemHint"`   // column guide for the editor and import
	MinItems       int    `json:"minItems"`
	DefaultRounds  int    `json:"defaultRounds"`
	RoundsLabel    string `json:"roundsLabel"`
	DefaultSeconds int    `json:"defaultSeconds"`
	SecondsLabel   string `json:"secondsLabel"`
	Realtime       bool   `json:"realtime"`
	JoinLate       bool   `json:"joinLate"`
	Icon           string `json:"icon"`
}

// Kind is one game's rules.
type Kind interface {
	Meta() Meta
	NewState() any
	Start(m *Match, s *Service, ctx context.Context) error
	Act(m *Match, s *Service, ctx context.Context, userID uint, action string, payload json.RawMessage) (bool, error)
	Timeout(m *Match, s *Service, ctx context.Context)
	Left(m *Match, s *Service, ctx context.Context, userID uint)
	View(m *Match, viewer uint) any
}

// lateJoiner is a kind that seats people after the start (bingo).
type lateJoiner interface {
	Joined(m *Match, s *Service, ctx context.Context, userID uint)
}

var kinds = []Kind{&drawKind{}, &pollKind{}, &truthKind{}, &solveKind{}, &storyKind{}, &whosaidKind{}, &connect4Kind{}, &hockeyKind{}, &bingoKind{}}

func kindOf(key string) Kind {
	for _, k := range kinds {
		if k.Meta().Key == key {
			return k
		}
	}
	return nil
}

// ---------------------------------------------------------------- helpers

func norm(s string) string {
	s = strings.TrimSpace(strings.ToLowerSpecial(unicode.TurkishCase, s))
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == ' ' {
			b.WriteRune(r)
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

func (s *Service) pickItems(ctx context.Context, kind string, n int) []models.GameItem {
	items, err := s.repo.Items(ctx, kind, true)
	if err != nil || len(items) == 0 {
		return nil
	}
	s.rnd.Shuffle(len(items), func(i, j int) { items[i], items[j] = items[j], items[i] })
	if n > 0 && len(items) > n {
		items = items[:n]
	}
	return items
}

func decode(payload json.RawMessage, v any) error {
	if len(payload) == 0 {
		return errs.Invalid("Hamle eksik.", nil)
	}
	if err := json.Unmarshal(payload, v); err != nil {
		return errs.Invalid("Hamle okunamadı.", nil)
	}
	return nil
}

func secondsOf(itemSeconds, def int) int {
	if itemSeconds > 0 {
		return itemSeconds
	}
	return def
}

// ================================================================ Çiz & Bil

type drawState struct {
	Order    []uint          `json:"order"`
	Turn     int             `json:"turn"`
	Total    int             `json:"total"`
	Phase    string          `json:"phase"` // choose | draw | reveal
	Choices  []string        `json:"choices"`
	Word     string          `json:"word"`
	Seconds  int             `json:"seconds"`
	Guessed  map[uint]int    `json:"guessed"`
	Guesses  []drawGuess     `json:"guesses"`
	Used     map[string]bool `json:"used"`
	Finished bool            `json:"finished"`
}

type drawGuess struct {
	UserID  uint   `json:"userId"`
	Name    string `json:"name"`
	Text    string `json:"text"`
	Correct bool   `json:"correct"`
}

type drawKind struct{}

func (drawKind) Meta() Meta {
	return Meta{Key: "draw", Name: "Çiz & Bil", Tagline: "Biri çizer, kalanlar bilir.", Icon: "pencil",
		How:        "Sırası gelen üç kelimeden birini seçer ve çizer. Diğerleri sohbet kutusuna tahmin yazar; erken bilen daha çok puan alır, çizen de bilen sayısı kadar. Herkes çizince tur biter.",
		MinPlayers: 3, MaxPlayers: 12, ItemKind: "draw", ItemLabel: "Kelime", ItemHint: "Metin: çizilecek kelime. Süre: o kelimeye özel saniye (boş: oyun ayarı).", MinItems: 6,
		DefaultRounds: 2, RoundsLabel: "Tur (herkes kaç kez çizer)", DefaultSeconds: 75, SecondsLabel: "Çizim süresi (sn)"}
}
func (drawKind) NewState() any { return &drawState{} }

func (k *drawKind) Start(m *Match, s *Service, ctx context.Context) error {
	st := m.Data.(*drawState)
	st.Order = nil
	for _, p := range m.active() {
		st.Order = append(st.Order, p.UserID)
	}
	s.rnd.Shuffle(len(st.Order), func(i, j int) { st.Order[i], st.Order[j] = st.Order[j], st.Order[i] })
	st.Total = m.Config.Rounds * len(st.Order)
	st.Turn = -1
	st.Used = map[string]bool{}
	k.next(m, s, ctx)
	return nil
}

func (k *drawKind) drawer(m *Match) uint {
	st := m.Data.(*drawState)
	if st.Turn < 0 || len(st.Order) == 0 {
		return 0
	}
	return st.Order[st.Turn%len(st.Order)]
}

func (k *drawKind) next(m *Match, s *Service, ctx context.Context) {
	st := m.Data.(*drawState)
	for {
		st.Turn++
		if st.Turn >= st.Total {
			st.Finished = true
			s.finish(ctx, m, m.leaders(), "")
			return
		}
		if p := m.player(k.drawer(m)); p != nil && !p.Left {
			break
		}
	}
	items := s.pickItems(ctx, "draw", 0)
	st.Choices = nil
	for _, it := range items {
		if st.Used[it.Text] {
			continue
		}
		st.Choices = append(st.Choices, it.Text)
		if len(st.Choices) == 3 {
			break
		}
	}
	if len(st.Choices) == 0 {
		st.Used = map[string]bool{}
		for i, it := range items {
			if i == 3 {
				break
			}
			st.Choices = append(st.Choices, it.Text)
		}
	}
	st.Phase = "choose"
	st.Word = ""
	st.Guessed = map[uint]int{}
	st.Guesses = nil
	m.Strokes = nil
	m.setDeadline(15)
}

func (k *drawKind) begin(m *Match, s *Service, ctx context.Context, word string) {
	st := m.Data.(*drawState)
	st.Word = word
	st.Used[word] = true
	st.Phase = "draw"
	st.Seconds = m.Config.Seconds
	if items, err := s.repo.Items(ctx, "draw", true); err == nil {
		for _, it := range items {
			if it.Text == word && it.Seconds > 0 {
				st.Seconds = it.Seconds
			}
		}
	}
	m.Strokes = nil
	s.push(ctx, m, "game.stroke", map[string]any{"clear": true})
	m.setDeadline(st.Seconds)
}

func (k *drawKind) Act(m *Match, s *Service, ctx context.Context, uid uint, action string, payload json.RawMessage) (bool, error) {
	st := m.Data.(*drawState)
	drawer := k.drawer(m)
	switch action {
	case "choose":
		var in struct{ Index int }
		if err := decode(payload, &in); err != nil {
			return false, err
		}
		if uid != drawer || st.Phase != "choose" || in.Index < 0 || in.Index >= len(st.Choices) {
			return false, errs.Invalid("Şu an kelime seçemezsiniz.", nil)
		}
		k.begin(m, s, ctx, st.Choices[in.Index])
		return true, nil
	case "stroke":
		if uid != drawer || st.Phase != "draw" {
			return false, errs.Forbidden("Çizen siz değilsiniz.")
		}
		if len(payload) > 20000 {
			return false, errs.Invalid("Çizgi çok büyük.", nil)
		}
		if len(m.Strokes) < 1500 {
			m.Strokes = append(m.Strokes, payload)
		}
		s.push(ctx, m, "game.stroke", json.RawMessage(payload))
		return false, nil
	case "clear":
		if uid != drawer || st.Phase != "draw" {
			return false, errs.Forbidden("Çizen siz değilsiniz.")
		}
		m.Strokes = nil
		s.push(ctx, m, "game.stroke", map[string]any{"clear": true})
		return false, nil
	case "guess":
		var in struct{ Text string }
		if err := decode(payload, &in); err != nil {
			return false, err
		}
		if uid == drawer || st.Phase != "draw" {
			return false, errs.Invalid("Şu an tahmin edemezsiniz.", nil)
		}
		if _, done := st.Guessed[uid]; done {
			return false, errs.Invalid("Zaten bildiniz.", nil)
		}
		text := strings.TrimSpace(in.Text)
		if text == "" || len([]rune(text)) > 60 {
			return false, errs.Invalid("Tahmin boş ya da çok uzun.", nil)
		}
		name := ""
		if p := m.player(uid); p != nil {
			name = p.Name
		}
		if norm(text) == norm(st.Word) {
			left := m.secondsLeft()
			pts := 50 + int(50*float64(left)/float64(max(1, st.Seconds)))
			st.Guessed[uid] = pts
			m.addScore(uid, pts)
			m.addScore(drawer, 20)
			st.Guesses = append(st.Guesses, drawGuess{UserID: uid, Name: name, Correct: true})
			remaining := 0
			for _, p := range m.active() {
				if p.UserID != drawer {
					if _, ok := st.Guessed[p.UserID]; !ok {
						remaining++
					}
				}
			}
			if remaining == 0 {
				st.Phase = "reveal"
				m.setDeadline(4)
			}
			return true, nil
		}
		st.Guesses = append(st.Guesses, drawGuess{UserID: uid, Name: name, Text: text})
		if len(st.Guesses) > 60 {
			st.Guesses = st.Guesses[len(st.Guesses)-60:]
		}
		return true, nil
	}
	return false, errs.Invalid("Bilinmeyen hamle.", nil)
}

func (k *drawKind) Timeout(m *Match, s *Service, ctx context.Context) {
	st := m.Data.(*drawState)
	switch st.Phase {
	case "choose":
		if len(st.Choices) > 0 {
			k.begin(m, s, ctx, st.Choices[0])
		} else {
			k.next(m, s, ctx)
		}
	case "draw":
		st.Phase = "reveal"
		m.setDeadline(4)
	case "reveal":
		k.next(m, s, ctx)
	}
}

func (k *drawKind) Left(m *Match, s *Service, ctx context.Context, uid uint) {
	if k.drawer(m) == uid {
		st := m.Data.(*drawState)
		st.Phase = "reveal"
		m.setDeadline(2)
	}
}

func (k *drawKind) View(m *Match, viewer uint) any {
	st := m.Data.(*drawState)
	drawer := k.drawer(m)
	mask := ""
	for _, r := range st.Word {
		if r == ' ' {
			mask += "  "
		} else {
			mask += "_ "
		}
	}
	out := map[string]any{
		"phase": st.Phase, "drawer": drawer, "turn": st.Turn + 1, "total": st.Total, "hint": strings.TrimSpace(mask),
		"wordLength": len([]rune(st.Word)), "guesses": st.Guesses, "guessed": st.Guessed, "strokes": m.Strokes, "seconds": st.Seconds,
	}
	if viewer == drawer || st.Phase == "reveal" || m.G.Status == statusFinished {
		out["word"] = st.Word
	}
	if viewer == drawer && st.Phase == "choose" {
		out["choices"] = st.Choices
	}
	return out
}

// ================================================================ Kalem Kâğıt Anketi

type pollState struct {
	Items   []pollQ       `json:"items"`
	Round   int           `json:"round"`
	Phase   string        `json:"phase"` // vote | reveal
	Votes   map[uint]uint `json:"votes"`
	Results []pollResult  `json:"results"`
}

type pollQ struct {
	Text    string `json:"text"`
	Seconds int    `json:"seconds"`
}

type pollResult struct {
	Question string       `json:"question"`
	Counts   map[uint]int `json:"counts"`
	Top      []uint       `json:"top"`
}

type pollKind struct{}

func (pollKind) Meta() Meta {
	return Meta{Key: "poll", Name: "Kalem Kâğıt Anketi", Tagline: "Ekipte kim...?", Icon: "vote",
		How:        "Her turda bir soru gelir: 'Ekipte kim bir müşteriye \"modemi kapatıp açtınız mı\" demeden günü bitiremez?' Herkes bir kişiye oy verir, süre bitince sonuç ve tacı alan görünür. En çok taç toplayan kazanır.",
		MinPlayers: 3, MaxPlayers: 0, ItemKind: "poll", ItemLabel: "Soru", ItemHint: "Metin: soru. Süre: o soruya özel saniye (boş: oyun ayarı).", MinItems: 5,
		DefaultRounds: 6, RoundsLabel: "Soru sayısı", DefaultSeconds: 30, SecondsLabel: "Oylama süresi (sn)"}
}
func (pollKind) NewState() any { return &pollState{} }

func (k *pollKind) Start(m *Match, s *Service, ctx context.Context) error {
	st := m.Data.(*pollState)
	items := s.pickItems(ctx, "poll", m.Config.Rounds)
	if len(items) == 0 {
		return errs.Invalid("Soru havuzu boş.", nil)
	}
	st.Items = nil
	for _, it := range items {
		st.Items = append(st.Items, pollQ{Text: it.Text, Seconds: secondsOf(it.Seconds, m.Config.Seconds)})
	}
	st.Round = 0
	st.Results = nil
	k.open(m)
	return nil
}

func (k *pollKind) open(m *Match) {
	st := m.Data.(*pollState)
	st.Phase = "vote"
	st.Votes = map[uint]uint{}
	m.setDeadline(st.Items[st.Round].Seconds)
}

func (k *pollKind) reveal(m *Match, s *Service, ctx context.Context) {
	st := m.Data.(*pollState)
	counts := map[uint]int{}
	best := 0
	for _, target := range st.Votes {
		counts[target]++
		if counts[target] > best {
			best = counts[target]
		}
	}
	var top []uint
	for uid, n := range counts {
		if n == best && best > 0 {
			top = append(top, uid)
			m.addScore(uid, 1)
		}
	}
	st.Results = append(st.Results, pollResult{Question: st.Items[st.Round].Text, Counts: counts, Top: top})
	st.Phase = "reveal"
	m.setDeadline(6)
}

func (k *pollKind) Act(m *Match, s *Service, ctx context.Context, uid uint, action string, payload json.RawMessage) (bool, error) {
	st := m.Data.(*pollState)
	if action != "vote" || st.Phase != "vote" {
		return false, errs.Invalid("Şu an oy verilemez.", nil)
	}
	var in struct{ Target uint }
	if err := decode(payload, &in); err != nil {
		return false, err
	}
	if p := m.player(in.Target); p == nil || p.Left {
		return false, errs.Invalid("Oyuncu bulunamadı.", nil)
	}
	st.Votes[uid] = in.Target
	if len(st.Votes) >= len(m.active()) {
		k.reveal(m, s, ctx)
	}
	return true, nil
}

func (k *pollKind) Timeout(m *Match, s *Service, ctx context.Context) {
	st := m.Data.(*pollState)
	if st.Phase == "vote" {
		k.reveal(m, s, ctx)
		return
	}
	st.Round++
	if st.Round >= len(st.Items) {
		s.finish(ctx, m, m.leaders(), "")
		return
	}
	k.open(m)
}

func (k *pollKind) Left(m *Match, s *Service, ctx context.Context, uid uint) {}

func (k *pollKind) View(m *Match, viewer uint) any {
	st := m.Data.(*pollState)
	q := ""
	if st.Round < len(st.Items) {
		q = st.Items[st.Round].Text
	}
	return map[string]any{"phase": st.Phase, "round": st.Round + 1, "total": len(st.Items), "question": q, "voted": len(st.Votes), "myVote": st.Votes[viewer], "results": st.Results}
}

// ================================================================ Yalan mı Gerçek mi

type truthEntry struct {
	Statements []string `json:"statements"`
	Lie        int      `json:"lie"`
}

type truthState struct {
	Phase   string              `json:"phase"` // write | vote | reveal
	Entries map[uint]truthEntry `json:"entries"`
	Order   []uint              `json:"order"`
	Turn    int                 `json:"turn"`
	Votes   map[uint]int        `json:"votes"`
	Rounds  []truthRound        `json:"rounds"`
}

type truthRound struct {
	Author uint         `json:"author"`
	Lie    int          `json:"lie"`
	Votes  map[uint]int `json:"votes"`
}

type truthKind struct{}

func (truthKind) Meta() Meta {
	return Meta{Key: "truth", Name: "Yalan mı Gerçek mi?", Tagline: "İki gerçek, bir uydurma.", Icon: "drama",
		How:        "Herkes başından geçen üç çağrı hikâyesi yazar, biri uydurmadır. Sırayla her yazarın hikâyeleri gelir, diğerleri uydurmayı bulmaya çalışır. Doğru bulan 10, kandıran yazar yanılttığı kişi başına 5 puan alır.",
		MinPlayers: 3, MaxPlayers: 10, DefaultRounds: 1, RoundsLabel: "", DefaultSeconds: 45, SecondsLabel: "Tahmin süresi (sn)"}
}
func (truthKind) NewState() any { return &truthState{} }

func (k *truthKind) Start(m *Match, s *Service, ctx context.Context) error {
	st := m.Data.(*truthState)
	st.Phase = "write"
	st.Entries = map[uint]truthEntry{}
	st.Rounds = nil
	m.setDeadline(max(120, 3*m.Config.Seconds))
	return nil
}

func (k *truthKind) startVoting(m *Match, s *Service, ctx context.Context) {
	st := m.Data.(*truthState)
	st.Order = nil
	for _, p := range m.active() {
		if _, ok := st.Entries[p.UserID]; ok {
			st.Order = append(st.Order, p.UserID)
		}
	}
	if len(st.Order) == 0 {
		s.finish(ctx, m, nil, "kimse hikâye yazmadı")
		return
	}
	st.Turn = -1
	k.nextAuthor(m, s, ctx)
}

func (k *truthKind) nextAuthor(m *Match, s *Service, ctx context.Context) {
	st := m.Data.(*truthState)
	st.Turn++
	if st.Turn >= len(st.Order) {
		s.finish(ctx, m, m.leaders(), "")
		return
	}
	st.Phase = "vote"
	st.Votes = map[uint]int{}
	m.setDeadline(m.Config.Seconds)
}

func (k *truthKind) reveal(m *Match, s *Service, ctx context.Context) {
	st := m.Data.(*truthState)
	author := st.Order[st.Turn]
	e := st.Entries[author]
	for uid, idx := range st.Votes {
		if idx == e.Lie {
			m.addScore(uid, 10)
		} else {
			m.addScore(author, 5)
		}
	}
	st.Rounds = append(st.Rounds, truthRound{Author: author, Lie: e.Lie, Votes: st.Votes})
	st.Phase = "reveal"
	m.setDeadline(7)
}

func (k *truthKind) Act(m *Match, s *Service, ctx context.Context, uid uint, action string, payload json.RawMessage) (bool, error) {
	st := m.Data.(*truthState)
	switch action {
	case "write":
		if st.Phase != "write" {
			return false, errs.Invalid("Yazma süresi bitti.", nil)
		}
		var in truthEntry
		if err := decode(payload, &in); err != nil {
			return false, err
		}
		if len(in.Statements) != 3 || in.Lie < 0 || in.Lie > 2 {
			return false, errs.Invalid("Üç hikâye ve bir uydurma seçilmeli.", nil)
		}
		for i := range in.Statements {
			in.Statements[i] = strings.TrimSpace(in.Statements[i])
			if in.Statements[i] == "" || len([]rune(in.Statements[i])) > 300 {
				return false, errs.Invalid("Her hikâye 1 ile 300 karakter arasında olmalı.", nil)
			}
		}
		st.Entries[uid] = in
		if len(st.Entries) >= len(m.active()) {
			k.startVoting(m, s, ctx)
		}
		return true, nil
	case "vote":
		if st.Phase != "vote" {
			return false, errs.Invalid("Şu an oy verilemez.", nil)
		}
		author := st.Order[st.Turn]
		if uid == author {
			return false, errs.Invalid("Kendi hikâyene oy veremezsin.", nil)
		}
		var in struct{ Index int }
		if err := decode(payload, &in); err != nil {
			return false, err
		}
		if in.Index < 0 || in.Index > 2 {
			return false, errs.Invalid("Geçersiz seçim.", nil)
		}
		st.Votes[uid] = in.Index
		voters := 0
		for _, p := range m.active() {
			if p.UserID != author {
				voters++
			}
		}
		if len(st.Votes) >= voters {
			k.reveal(m, s, ctx)
		}
		return true, nil
	}
	return false, errs.Invalid("Bilinmeyen hamle.", nil)
}

func (k *truthKind) Timeout(m *Match, s *Service, ctx context.Context) {
	st := m.Data.(*truthState)
	switch st.Phase {
	case "write":
		k.startVoting(m, s, ctx)
	case "vote":
		k.reveal(m, s, ctx)
	case "reveal":
		k.nextAuthor(m, s, ctx)
	}
}

func (k *truthKind) Left(m *Match, s *Service, ctx context.Context, uid uint) {
	st := m.Data.(*truthState)
	if st.Phase == "vote" && st.Turn < len(st.Order) && st.Order[st.Turn] == uid {
		k.reveal(m, s, ctx)
	}
}

func (k *truthKind) View(m *Match, viewer uint) any {
	st := m.Data.(*truthState)
	out := map[string]any{"phase": st.Phase, "written": len(st.Entries), "rounds": st.Rounds, "order": st.Order}
	if _, ok := st.Entries[viewer]; ok {
		out["mine"] = st.Entries[viewer]
	}
	if (st.Phase == "vote" || st.Phase == "reveal") && st.Turn < len(st.Order) {
		author := st.Order[st.Turn]
		e := st.Entries[author]
		out["author"] = author
		out["turn"] = st.Turn + 1
		out["statements"] = e.Statements
		out["myVote"] = -1
		if v, ok := st.Votes[viewer]; ok {
			out["myVote"] = v
		}
		out["voted"] = len(st.Votes)
		if st.Phase == "reveal" {
			out["lie"] = e.Lie
			out["votes"] = st.Votes
		}
	}
	return out
}

// ================================================================ 60 Saniyede Çöz

type solveState struct {
	Items   []pollQ         `json:"items"`
	Round   int             `json:"round"`
	Phase   string          `json:"phase"` // write | vote | reveal
	Answers map[uint]string `json:"answers"`
	Votes   map[uint]uint   `json:"votes"`
	Results []solveResult   `json:"results"`
}

type solveResult struct {
	Scenario string          `json:"scenario"`
	Answers  map[uint]string `json:"answers"`
	Votes    map[uint]int    `json:"votes"`
	Best     []uint          `json:"best"`
}

type solveKind struct{}

func (solveKind) Meta() Meta {
	return Meta{Key: "solve", Name: "60 Saniyede Çöz", Tagline: "Aynı senaryo, en iyi çözüm.", Icon: "timer",
		How:        "Herkese aynı teknik senaryo gelir: 'Telefon çalıyor ama internet yok, kablo ışığı turuncu.' Süre içinde çözüm adımlarını yazarsınız. Sonra herkes başkasının çözümüne oy verir; oy başına 10 puan.",
		MinPlayers: 2, MaxPlayers: 0, ItemKind: "solve", ItemLabel: "Senaryo", ItemHint: "Metin: senaryo. Cevap: örnek çözüm (sonuçta gösterilir, isteğe bağlı). Süre: yazma saniyesi (boş: oyun ayarı).", MinItems: 3,
		DefaultRounds: 3, RoundsLabel: "Senaryo sayısı", DefaultSeconds: 60, SecondsLabel: "Yazma süresi (sn)"}
}
func (solveKind) NewState() any { return &solveState{} }

func (k *solveKind) Start(m *Match, s *Service, ctx context.Context) error {
	st := m.Data.(*solveState)
	items := s.pickItems(ctx, "solve", m.Config.Rounds)
	if len(items) == 0 {
		return errs.Invalid("Senaryo havuzu boş.", nil)
	}
	st.Items = nil
	st.Results = nil
	for _, it := range items {
		st.Items = append(st.Items, pollQ{Text: it.Text + "\x00" + it.Answer, Seconds: secondsOf(it.Seconds, m.Config.Seconds)})
	}
	st.Round = 0
	k.open(m)
	return nil
}

func (k *solveKind) scenario(st *solveState) (string, string) {
	parts := strings.SplitN(st.Items[st.Round].Text, "\x00", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return parts[0], ""
}

func (k *solveKind) open(m *Match) {
	st := m.Data.(*solveState)
	st.Phase = "write"
	st.Answers = map[uint]string{}
	st.Votes = map[uint]uint{}
	m.setDeadline(st.Items[st.Round].Seconds)
}

func (k *solveKind) vote(m *Match, s *Service, ctx context.Context) {
	st := m.Data.(*solveState)
	if len(st.Answers) < 2 {
		k.reveal(m, s, ctx)
		return
	}
	st.Phase = "vote"
	m.setDeadline(40)
}

func (k *solveKind) reveal(m *Match, s *Service, ctx context.Context) {
	st := m.Data.(*solveState)
	counts := map[uint]int{}
	best := 0
	for _, target := range st.Votes {
		counts[target]++
		if counts[target] > best {
			best = counts[target]
		}
	}
	var bestIDs []uint
	for uid, n := range counts {
		m.addScore(uid, 10*n)
		if n == best && best > 0 {
			bestIDs = append(bestIDs, uid)
		}
	}
	text, _ := k.scenario(st)
	st.Results = append(st.Results, solveResult{Scenario: text, Answers: st.Answers, Votes: counts, Best: bestIDs})
	st.Phase = "reveal"
	m.setDeadline(12)
}

func (k *solveKind) Act(m *Match, s *Service, ctx context.Context, uid uint, action string, payload json.RawMessage) (bool, error) {
	st := m.Data.(*solveState)
	switch action {
	case "answer":
		if st.Phase != "write" {
			return false, errs.Invalid("Yazma süresi bitti.", nil)
		}
		var in struct{ Text string }
		if err := decode(payload, &in); err != nil {
			return false, err
		}
		text := strings.TrimSpace(in.Text)
		if text == "" || len([]rune(text)) > 600 {
			return false, errs.Invalid("Çözüm 1 ile 600 karakter arasında olmalı.", nil)
		}
		st.Answers[uid] = text
		if len(st.Answers) >= len(m.active()) {
			k.vote(m, s, ctx)
		}
		return true, nil
	case "vote":
		if st.Phase != "vote" {
			return false, errs.Invalid("Şu an oy verilemez.", nil)
		}
		var in struct{ Target uint }
		if err := decode(payload, &in); err != nil {
			return false, err
		}
		if in.Target == uid {
			return false, errs.Invalid("Kendi çözümüne oy veremezsin.", nil)
		}
		if _, ok := st.Answers[in.Target]; !ok {
			return false, errs.Invalid("Böyle bir çözüm yok.", nil)
		}
		st.Votes[uid] = in.Target
		if len(st.Votes) >= len(m.active()) {
			k.reveal(m, s, ctx)
		}
		return true, nil
	}
	return false, errs.Invalid("Bilinmeyen hamle.", nil)
}

func (k *solveKind) Timeout(m *Match, s *Service, ctx context.Context) {
	st := m.Data.(*solveState)
	switch st.Phase {
	case "write":
		k.vote(m, s, ctx)
	case "vote":
		k.reveal(m, s, ctx)
	case "reveal":
		st.Round++
		if st.Round >= len(st.Items) {
			s.finish(ctx, m, m.leaders(), "")
			return
		}
		k.open(m)
	}
}

func (k *solveKind) Left(m *Match, s *Service, ctx context.Context, uid uint) {}

func (k *solveKind) View(m *Match, viewer uint) any {
	st := m.Data.(*solveState)
	out := map[string]any{"phase": st.Phase, "round": st.Round + 1, "total": len(st.Items), "results": st.Results, "answered": len(st.Answers), "voted": len(st.Votes)}
	if st.Round < len(st.Items) {
		text, sample := k.scenario(st)
		out["scenario"] = text
		if st.Phase == "reveal" {
			out["sample"] = sample
		}
	}
	out["mine"] = st.Answers[viewer]
	out["myVote"] = st.Votes[viewer]
	if st.Phase == "vote" || st.Phase == "reveal" {
		out["answers"] = st.Answers
	}
	return out
}

// ================================================================ Hikâye Zinciri

type storyState struct {
	Prompt    string          `json:"prompt"`
	Sentences []storySentence `json:"sentences"`
	Turn      int             `json:"turn"`
	Total     int             `json:"total"`
	Order     []uint          `json:"order"`
}

type storySentence struct {
	UserID uint   `json:"userId"`
	Name   string `json:"name"`
	Text   string `json:"text"`
}

type storyKind struct{}

func (storyKind) Meta() Meta {
	return Meta{Key: "story", Name: "Hikâye Zinciri", Tagline: "Müşteri aradı ve...", Icon: "book",
		How:        "Bir açılış cümlesiyle başlar, sırayla herkes bir cümle ekler. Süre dolarsa sıra geçer. Belirlenen cümle sayısına ulaşınca hikâye odaya mesaj olarak düşer. Kazanan yok, herkes kazanır.",
		MinPlayers: 2, MaxPlayers: 0, ItemKind: "story", ItemLabel: "Açılış cümlesi", ItemHint: "Metin: hikâyenin ilk cümlesi.", MinItems: 1,
		DefaultRounds: 10, RoundsLabel: "Toplam cümle", DefaultSeconds: 25, SecondsLabel: "Cümle başına süre (sn)"}
}
func (storyKind) NewState() any { return &storyState{} }

func (k *storyKind) Start(m *Match, s *Service, ctx context.Context) error {
	st := m.Data.(*storyState)
	items := s.pickItems(ctx, "story", 1)
	st.Prompt = "Müşteri aradı ve..."
	if len(items) > 0 {
		st.Prompt = items[0].Text
	}
	st.Sentences = nil
	st.Total = m.Config.Rounds
	st.Order = nil
	for _, p := range m.active() {
		st.Order = append(st.Order, p.UserID)
	}
	s.rnd.Shuffle(len(st.Order), func(i, j int) { st.Order[i], st.Order[j] = st.Order[j], st.Order[i] })
	st.Turn = 0
	m.setDeadline(m.Config.Seconds)
	return nil
}

func (k *storyKind) writer(m *Match) uint {
	st := m.Data.(*storyState)
	if len(st.Order) == 0 {
		return 0
	}
	for i := 0; i < len(st.Order); i++ {
		uid := st.Order[(st.Turn+i)%len(st.Order)]
		if p := m.player(uid); p != nil && !p.Left {
			st.Turn = (st.Turn + i) % len(st.Order)
			return uid
		}
	}
	return 0
}

func (k *storyKind) advance(m *Match, s *Service, ctx context.Context) {
	st := m.Data.(*storyState)
	if len(st.Sentences) >= st.Total {
		var b strings.Builder
		b.WriteString("📖 Hikâye Zinciri: " + st.Prompt)
		for _, sn := range st.Sentences {
			b.WriteString(" " + sn.Text)
		}
		b.WriteString("\n")
		names := []string{}
		for _, p := range m.active() {
			names = append(names, p.Name)
			m.addScore(p.UserID, 1)
		}
		b.WriteString("Yazanlar: " + strings.Join(names, ", "))
		s.rooms.PostSystem(ctx, m.G.GroupID, b.String())
		var all []uint
		for _, p := range m.active() {
			all = append(all, p.UserID)
		}
		s.finish(ctx, m, all, "")
		return
	}
	st.Turn = (st.Turn + 1) % max(1, len(st.Order))
	m.setDeadline(m.Config.Seconds)
}

func (k *storyKind) Act(m *Match, s *Service, ctx context.Context, uid uint, action string, payload json.RawMessage) (bool, error) {
	st := m.Data.(*storyState)
	if action != "add" {
		return false, errs.Invalid("Bilinmeyen hamle.", nil)
	}
	if k.writer(m) != uid {
		return false, errs.Invalid("Sıra sende değil.", nil)
	}
	var in struct{ Text string }
	if err := decode(payload, &in); err != nil {
		return false, err
	}
	text := strings.TrimSpace(in.Text)
	if text == "" || len([]rune(text)) > 200 {
		return false, errs.Invalid("Cümle 1 ile 200 karakter arasında olmalı.", nil)
	}
	name := ""
	if p := m.player(uid); p != nil {
		name = p.Name
	}
	st.Sentences = append(st.Sentences, storySentence{UserID: uid, Name: name, Text: text})
	k.advance(m, s, ctx)
	return true, nil
}

func (k *storyKind) Timeout(m *Match, s *Service, ctx context.Context) {
	k.advance(m, s, ctx)
}

func (k *storyKind) Left(m *Match, s *Service, ctx context.Context, uid uint) {
	if k.writer(m) == uid {
		k.advance(m, s, ctx)
	}
}

func (k *storyKind) View(m *Match, viewer uint) any {
	st := m.Data.(*storyState)
	return map[string]any{"prompt": st.Prompt, "sentences": st.Sentences, "writer": k.writer(m), "count": len(st.Sentences), "total": st.Total}
}

// ================================================================ Kim Söyledi

type whosaidState struct {
	Lines  []whosaidLine `json:"lines"`
	Round  int           `json:"round"`
	Phase  string        `json:"phase"` // guess | reveal
	Votes  map[uint]uint `json:"votes"`
	Scores []whosaidRes  `json:"scores"`
}

type whosaidLine struct {
	Author uint   `json:"author"`
	Text   string `json:"text"`
}

type whosaidRes struct {
	Author  uint          `json:"author"`
	Correct []uint        `json:"correct"`
	Votes   map[uint]uint `json:"votes"`
}

type whosaidKind struct{}

func (whosaidKind) Meta() Meta {
	return Meta{Key: "whosaid", Name: "Kim Söyledi?", Tagline: "Bu odada geçen bir cümle.", Icon: "message-circle-question",
		How:        "Oyuncuların bu odada son bir haftada yazdığı bir mesaj gelir: 'bu müşteri beni bitirdi ya'. Kim yazdı? Yazan izler, diğerleri tahmin eder. Doğru tahmin 10 puan. Katılan herkes mesajlarının kullanılmasını kabul etmiş olur.",
		MinPlayers: 3, MaxPlayers: 0, DefaultRounds: 8, RoundsLabel: "Mesaj sayısı", DefaultSeconds: 25, SecondsLabel: "Tahmin süresi (sn)"}
}
func (whosaidKind) NewState() any { return &whosaidState{} }

func (k *whosaidKind) Start(m *Match, s *Service, ctx context.Context) error {
	st := m.Data.(*whosaidState)
	ids := []uint{}
	for _, p := range m.active() {
		ids = append(ids, p.UserID)
	}
	rows, err := s.repo.RecentLines(ctx, m.G.GroupID, ids, m.Config.Rounds*3)
	if err != nil {
		return errs.Internal(err)
	}
	st.Lines = nil
	seen := map[uint]int{}
	for _, r := range rows {
		if r.SenderID == nil || seen[*r.SenderID] >= 3 {
			continue
		}
		seen[*r.SenderID]++
		st.Lines = append(st.Lines, whosaidLine{Author: *r.SenderID, Text: r.Body})
		if len(st.Lines) >= m.Config.Rounds {
			break
		}
	}
	if len(st.Lines) < 2 {
		return errs.Invalid("Bu odada oyunculara ait yeterli mesaj yok (son 7 gün). Biraz yazışın, sonra deneyin.", nil)
	}
	st.Round = 0
	st.Scores = nil
	k.open(m)
	return nil
}

func (k *whosaidKind) open(m *Match) {
	st := m.Data.(*whosaidState)
	st.Phase = "guess"
	st.Votes = map[uint]uint{}
	m.setDeadline(m.Config.Seconds)
}

func (k *whosaidKind) reveal(m *Match, s *Service, ctx context.Context) {
	st := m.Data.(*whosaidState)
	line := st.Lines[st.Round]
	res := whosaidRes{Author: line.Author, Votes: st.Votes}
	for uid, target := range st.Votes {
		if target == line.Author {
			m.addScore(uid, 10)
			res.Correct = append(res.Correct, uid)
		}
	}
	st.Scores = append(st.Scores, res)
	st.Phase = "reveal"
	m.setDeadline(6)
}

func (k *whosaidKind) Act(m *Match, s *Service, ctx context.Context, uid uint, action string, payload json.RawMessage) (bool, error) {
	st := m.Data.(*whosaidState)
	if action != "guess" || st.Phase != "guess" {
		return false, errs.Invalid("Şu an tahmin edilemez.", nil)
	}
	line := st.Lines[st.Round]
	if uid == line.Author {
		return false, errs.Invalid("Kendi mesajını tahmin edemezsin.", nil)
	}
	var in struct{ Author uint }
	if err := decode(payload, &in); err != nil {
		return false, err
	}
	if p := m.player(in.Author); p == nil {
		return false, errs.Invalid("Oyuncu bulunamadı.", nil)
	}
	st.Votes[uid] = in.Author
	voters := 0
	for _, p := range m.active() {
		if p.UserID != line.Author {
			voters++
		}
	}
	if len(st.Votes) >= voters {
		k.reveal(m, s, ctx)
	}
	return true, nil
}

func (k *whosaidKind) Timeout(m *Match, s *Service, ctx context.Context) {
	st := m.Data.(*whosaidState)
	if st.Phase == "guess" {
		k.reveal(m, s, ctx)
		return
	}
	st.Round++
	if st.Round >= len(st.Lines) {
		s.finish(ctx, m, m.leaders(), "")
		return
	}
	k.open(m)
}

func (k *whosaidKind) Left(m *Match, s *Service, ctx context.Context, uid uint) {}

func (k *whosaidKind) View(m *Match, viewer uint) any {
	st := m.Data.(*whosaidState)
	out := map[string]any{"phase": st.Phase, "round": st.Round + 1, "total": len(st.Lines), "voted": len(st.Votes), "myVote": st.Votes[viewer], "scores": st.Scores}
	if st.Round < len(st.Lines) {
		line := st.Lines[st.Round]
		out["text"] = line.Text
		out["isAuthor"] = line.Author == viewer
		if st.Phase == "reveal" {
			out["author"] = line.Author
			out["votes"] = st.Votes
		}
	}
	return out
}

// ================================================================ Bağlantı Dört

type c4State struct {
	Board  [6][7]int `json:"board"`
	Turn   int       `json:"turn"` // 1 or 2
	Winner int       `json:"winner"`
	Moves  int       `json:"moves"`
	Line   [][2]int  `json:"line"`
}

type connect4Kind struct{}

func (connect4Kind) Meta() Meta {
	return Meta{Key: "connect4", Name: "Bağlantı Dört", Tagline: "Dörtlü diz, kazan.", Icon: "grid-3x3",
		How:        "İki kişi. Sırayla bir sütuna taş bırakılır, taş en alta düşer. Yatay, dikey ya da çapraz dört taşı yan yana getiren kazanır. Hamle süresi dolarsa sıra karşıya geçer.",
		MinPlayers: 2, MaxPlayers: 2, DefaultRounds: 1, DefaultSeconds: 30, SecondsLabel: "Hamle süresi (sn)"}
}
func (connect4Kind) NewState() any { return &c4State{} }

func (k *connect4Kind) Start(m *Match, s *Service, ctx context.Context) error {
	st := m.Data.(*c4State)
	*st = c4State{Turn: 1}
	m.setDeadline(m.Config.Seconds)
	return nil
}

func (k *connect4Kind) seat(m *Match, uid uint) int {
	for i, p := range m.Players {
		if p.UserID == uid {
			return i + 1
		}
	}
	return 0
}

func (k *connect4Kind) uidOf(m *Match, seat int) uint {
	if seat >= 1 && seat <= len(m.Players) {
		return m.Players[seat-1].UserID
	}
	return 0
}

func (k *connect4Kind) winLine(b *[6][7]int, r, c, who int) [][2]int {
	dirs := [][2]int{{0, 1}, {1, 0}, {1, 1}, {1, -1}}
	for _, d := range dirs {
		line := [][2]int{{r, c}}
		for sign := -1; sign <= 1; sign += 2 {
			for step := 1; step < 4; step++ {
				rr, cc := r+d[0]*step*sign, c+d[1]*step*sign
				if rr < 0 || rr >= 6 || cc < 0 || cc >= 7 || b[rr][cc] != who {
					break
				}
				line = append(line, [2]int{rr, cc})
			}
		}
		if len(line) >= 4 {
			return line
		}
	}
	return nil
}

func (k *connect4Kind) Act(m *Match, s *Service, ctx context.Context, uid uint, action string, payload json.RawMessage) (bool, error) {
	st := m.Data.(*c4State)
	if action != "drop" {
		return false, errs.Invalid("Bilinmeyen hamle.", nil)
	}
	seat := k.seat(m, uid)
	if seat != st.Turn {
		return false, errs.Invalid("Sıra sende değil.", nil)
	}
	var in struct{ Col int }
	if err := decode(payload, &in); err != nil {
		return false, err
	}
	if in.Col < 0 || in.Col > 6 || st.Board[0][in.Col] != 0 {
		return false, errs.Invalid("Bu sütun dolu.", nil)
	}
	row := 5
	for row >= 0 && st.Board[row][in.Col] != 0 {
		row--
	}
	st.Board[row][in.Col] = seat
	st.Moves++
	if line := k.winLine(&st.Board, row, in.Col, seat); line != nil {
		st.Winner = seat
		st.Line = line
		m.addScore(uid, 1)
		s.finish(ctx, m, []uint{uid}, "")
		return true, nil
	}
	if st.Moves >= 42 {
		s.finish(ctx, m, nil, "berabere")
		return true, nil
	}
	st.Turn = 3 - st.Turn
	m.setDeadline(m.Config.Seconds)
	return true, nil
}

func (k *connect4Kind) Timeout(m *Match, s *Service, ctx context.Context) {
	st := m.Data.(*c4State)
	st.Turn = 3 - st.Turn
	m.setDeadline(m.Config.Seconds)
}

func (k *connect4Kind) Left(m *Match, s *Service, ctx context.Context, uid uint) {}

func (k *connect4Kind) View(m *Match, viewer uint) any {
	st := m.Data.(*c4State)
	return map[string]any{"board": st.Board, "turn": k.uidOf(m, st.Turn), "winner": k.uidOf(m, st.Winner), "line": st.Line, "mySeat": k.seat(m, viewer)}
}

// ================================================================ Masa Hokeyi

type hockeyState struct {
	Puck   [4]float64    `json:"puck"` // x, y, vx, vy
	Pads   [2][2]float64 `json:"pads"`
	PadV   [2][2]float64 `json:"padV"` // paddle speed, so a hit carries the swing
	Score  [2]int        `json:"score"`
	Target int           `json:"target"`
	Phase  string        `json:"phase"` // play | goal
	GoalAt time.Time     `json:"goalAt"`
	Serve  int           `json:"serve"`
	Scorer int           `json:"scorer"` // seat that just scored, for the banner
	Still  int           `json:"-"`      // ticks the puck has barely moved
	LastX  float64       `json:"-"`
	LastY  float64       `json:"-"`
}

// Table in abstract units; the browser scales. Speeds are per tick at 30 Hz.
const (
	hkW, hkH     = 100.0, 160.0
	hkPad        = 6.0
	hkPuck       = 3.0
	hkGoal       = 34.0
	hkMaxSpeed   = 5.0
	hkMinSpeed   = 0.9
	hkFriction   = 0.992
	hkWallBounce = 0.94
	hkSwing      = 0.55 // how much of the paddle's own speed the puck takes
	hkGoalPause  = 1800 * time.Millisecond
	hkSubSteps   = 3
)

type hockeyKind struct{}

func (hockeyKind) Meta() Meta {
	return Meta{Key: "hockey", Name: "Masa Hokeyi", Tagline: "İki dakikalık refleks düellosu.", Icon: "disc",
		How:        "İki kişi, üstten görünüm. Fareyle raketini kendi yarı sahanda sürükle, pucku karşı kaleye sok. Yedi golde biter. Çağrı gelirse oyun durur, bitince kaldığı yerden devam eder.",
		MinPlayers: 2, MaxPlayers: 2, DefaultRounds: 7, RoundsLabel: "Gol hedefi", DefaultSeconds: 0, Realtime: true}
}
func (hockeyKind) NewState() any { return &hockeyState{} }

func (k *hockeyKind) Start(m *Match, s *Service, ctx context.Context) error {
	st := m.Data.(*hockeyState)
	*st = hockeyState{Target: max(1, m.Config.Rounds), Phase: "play", Serve: 1}
	st.Pads = [2][2]float64{{hkW / 2, hkH - 20}, {hkW / 2, 20}}
	k.serve(st)
	return nil
}

func (k *hockeyKind) serve(st *hockeyState) {
	dir := 1.0
	if st.Serve == 2 {
		dir = -1.0
	}
	st.Puck = [4]float64{hkW / 2, hkH / 2, 0, 1.6 * dir}
	st.PadV = [2][2]float64{}
	st.Phase = "play"
	// A mallet parked on the centre spot must not swallow the serve.
	for i := 0; i < 2; i++ {
		if math.Hypot(st.Puck[0]-st.Pads[i][0], st.Puck[1]-st.Pads[i][1]) < hkPad+hkPuck {
			k.separate(st, i, i)
		}
	}
}

func (k *hockeyKind) seat(m *Match, uid uint) int {
	for i, p := range m.Players {
		if p.UserID == uid {
			return i
		}
	}
	return -1
}

func (k *hockeyKind) Act(m *Match, s *Service, ctx context.Context, uid uint, action string, payload json.RawMessage) (bool, error) {
	st := m.Data.(*hockeyState)
	if action != "move" {
		return false, errs.Invalid("Bilinmeyen hamle.", nil)
	}
	seat := k.seat(m, uid)
	if seat < 0 {
		return false, errs.Forbidden("Bu masada değilsiniz.")
	}
	var in struct{ X, Y float64 }
	if err := decode(payload, &in); err != nil {
		return false, err
	}
	x := math.Max(hkPad, math.Min(hkW-hkPad, in.X))
	var y float64
	if seat == 0 {
		y = math.Max(hkH/2+hkPad, math.Min(hkH-hkPad, in.Y))
	} else {
		y = math.Max(hkPad, math.Min(hkH/2-hkPad, in.Y))
	}
	// The swing: how far the mallet moved since its last report. A jump
	// across the table is capped, and the path it sweeps is checked against
	// the puck, so a fast flick cannot pass through it between two frames.
	ox, oy := st.Pads[seat][0], st.Pads[seat][1]
	vx, vy := x-ox, y-oy
	if sp := math.Hypot(vx, vy); sp > 14 {
		vx, vy = vx/sp*14, vy/sp*14
		x, y = ox+vx, oy+vy
	}
	st.PadV[seat] = [2]float64{vx * 0.7, vy * 0.7}
	if st.Phase == "play" {
		p := &st.Puck
		// Closest point of the puck to the mallet's path.
		l2 := vx*vx + vy*vy
		t := 0.0
		if l2 > 0 {
			t = math.Max(0, math.Min(1, ((p[0]-ox)*vx+(p[1]-oy)*vy)/l2))
		}
		cx, cy := ox+vx*t, oy+vy*t
		nx, ny := p[0]-cx, p[1]-cy
		nd := math.Hypot(nx, ny)
		if nd < hkPad+hkPuck {
			if nd < 0.01 {
				// Dead centre: push along the swing.
				nx, ny, nd = vx, vy, math.Max(0.01, math.Hypot(vx, vy))
			}
			nx, ny = nx/nd, ny/nd
			speed := math.Max(hkMinSpeed*1.5, math.Min(hkMaxSpeed, math.Hypot(vx, vy)*0.9))
			p[2], p[3] = nx*speed+vx*0.2, ny*speed+vy*0.2
			if sp := math.Hypot(p[2], p[3]); sp > hkMaxSpeed {
				p[2], p[3] = p[2]/sp*hkMaxSpeed, p[3]/sp*hkMaxSpeed
			}
			// Leave the puck just outside the mallet's final position, sliding
			// along a board if it is pinned there.
			st.Pads[seat] = [2]float64{x, y}
			if math.Hypot(p[0]-x, p[1]-y) < hkPad+hkPuck {
				ox2, oy2 := k.separate(st, seat, seat)
				if p[2]*ox2+p[3]*oy2 < 0.2 {
					p[2] += ox2 * 0.6
					p[3] += oy2 * 0.6
				}
			}
		}
	}
	st.Pads[seat] = [2]float64{x, y}
	return false, nil
}

// step advances the puck one tick; true when the match just ended.
func (k *hockeyKind) step(m *Match, s *Service, ctx context.Context) bool {
	st := m.Data.(*hockeyState)
	if st.Phase == "goal" {
		if time.Since(st.GoalAt) > hkGoalPause {
			if st.Score[0] >= st.Target || st.Score[1] >= st.Target {
				w := 0
				if st.Score[1] > st.Score[0] {
					w = 1
				}
				if w < len(m.Players) {
					m.addScore(m.Players[w].UserID, st.Score[w])
					s.finish(ctx, m, []uint{m.Players[w].UserID}, fmt.Sprintf("%d - %d", st.Score[0], st.Score[1]))
				}
				return true
			}
			k.serve(st)
		}
		return false
	}
	p := &st.Puck
	for sub := 0; sub < hkSubSteps; sub++ {
		p[0] += p[2] / hkSubSteps
		p[1] += p[3] / hkSubSteps
		// Side boards.
		if p[0] < hkPuck {
			p[0] = hkPuck
			p[2] = -p[2] * hkWallBounce
		}
		if p[0] > hkW-hkPuck {
			p[0] = hkW - hkPuck
			p[2] = -p[2] * hkWallBounce
		}
		inGoal := math.Abs(p[0]-hkW/2) < hkGoal/2
		// End boards, with the goal mouth open.
		if p[1] < hkPuck {
			if inGoal {
				st.Score[0]++
				st.Scorer = 0
				st.Phase = "goal"
				st.GoalAt = time.Now()
				st.Serve = 2
				return false
			}
			p[1] = hkPuck
			p[3] = -p[3] * hkWallBounce
		}
		if p[1] > hkH-hkPuck {
			if inGoal {
				st.Score[1]++
				st.Scorer = 1
				st.Phase = "goal"
				st.GoalAt = time.Now()
				st.Serve = 1
				return false
			}
			p[1] = hkH - hkPuck
			p[3] = -p[3] * hkWallBounce
		}
		// Mallets: reflect off the normal and add the swing. Both are checked
		// twice so a puck squeezed between them still comes out.
		for pass := 0; pass < 2; pass++ {
			for i := 0; i < 2; i++ {
				dx, dy := p[0]-st.Pads[i][0], p[1]-st.Pads[i][1]
				if math.Hypot(dx, dy) >= hkPad+hkPuck {
					continue
				}
				nx, ny := k.separate(st, i, i)
				if pass > 0 {
					// Second contact in the same step: just glance off.
					if p[2]*nx+p[3]*ny < 0 {
						dot := p[2]*nx + p[3]*ny
						p[2] -= 2 * dot * nx
						p[3] -= 2 * dot * ny
					}
					continue
				}
				// Velocity of the puck relative to the mallet.
				rvx, rvy := p[2]-st.PadV[i][0], p[3]-st.PadV[i][1]
				dot := rvx*nx + rvy*ny
				if dot < 0 {
					rvx -= 2 * dot * nx
					rvy -= 2 * dot * ny
				}
				p[2] = rvx + st.PadV[i][0]*hkSwing + nx*0.6
				p[3] = rvy + st.PadV[i][1]*hkSwing + ny*0.6
				sp := math.Hypot(p[2], p[3])
				if sp > hkMaxSpeed {
					p[2], p[3] = p[2]/sp*hkMaxSpeed, p[3]/sp*hkMaxSpeed
				} else if sp < hkMinSpeed {
					p[2], p[3] = nx*hkMinSpeed, ny*hkMinSpeed
				}
				// Never leave the puck moving into the mallet.
				if p[2]*nx+p[3]*ny < 0.2 {
					p[2] += nx * 0.4
					p[3] += ny * 0.4
				}
			}
		}
		// Belt and braces: the puck stays on the table whatever happened.
		if math.IsNaN(p[0]) || math.IsNaN(p[1]) || math.IsNaN(p[2]) || math.IsNaN(p[3]) {
			k.serve(st)
			return false
		}
		p[0] = math.Max(hkPuck, math.Min(hkW-hkPuck, p[0]))
		p[1] = math.Max(hkPuck, math.Min(hkH-hkPuck, p[1]))
	}
	p[2] *= hkFriction
	p[3] *= hkFriction
	// The swing fades between reports.
	for i := 0; i < 2; i++ {
		st.PadV[i][0] *= 0.6
		st.PadV[i][1] *= 0.6
	}
	// Watchdog: a puck that cannot get anywhere for two seconds (pinned in
	// a corner, wedged between mallets) is dropped back at the centre.
	if math.Hypot(p[0]-st.LastX, p[1]-st.LastY) < 0.4 {
		st.Still++
		if st.Still > 60 {
			st.Still = 0
			k.serve(st)
			return false
		}
	} else {
		st.Still = 0
	}
	st.LastX, st.LastY = p[0], p[1]
	if math.Hypot(p[2], p[3]) < 0.25 {
		// A dead puck drifts toward the side that has to serve.
		p[3] = 0.25
		if st.Serve == 2 {
			p[3] = -0.25
		}
	}
	return false
}

// separate moves the puck out of mallet i and returns the contact normal.
// A puck pinned against a board slides along it instead of being pushed
// into the wall and clamped back inside the mallet, which used to freeze
// the game; a puck dead centre is pushed toward the far goal.
func (k *hockeyKind) separate(st *hockeyState, i int, seat int) (float64, float64) {
	p := &st.Puck
	mx, my := st.Pads[i][0], st.Pads[i][1]
	const R = hkPad + hkPuck + 0.2
	inside := func(x, y float64) bool { return x >= hkPuck && x <= hkW-hkPuck && y >= hkPuck && y <= hkH-hkPuck }
	dx, dy := p[0]-mx, p[1]-my
	dist := math.Hypot(dx, dy)
	if dist < 1e-6 {
		dx, dy, dist = 0, -1, 1
		if seat == 1 {
			dy = 1
		}
	}
	nx, ny := dx/dist, dy/dist
	// Candidate spots on the circle around the mallet, best first: the
	// contact normal, then the same edge with the other sign, then the
	// other axis both ways, then straight toward the table centre. The
	// first one on the table wins; in a corner that is never the wall.
	cands := [][2]float64{{mx + nx*R, my + ny*R}}
	if tx := mx + nx*R; tx < hkPuck || tx > hkW-hkPuck {
		cx := math.Max(hkPuck, math.Min(hkW-hkPuck, tx))
		if rest := R*R - (cx-mx)*(cx-mx); rest > 0 {
			sy := math.Sqrt(rest)
			if ny < 0 {
				cands = append(cands, [2]float64{cx, my - sy}, [2]float64{cx, my + sy})
			} else {
				cands = append(cands, [2]float64{cx, my + sy}, [2]float64{cx, my - sy})
			}
		}
	}
	if ty := my + ny*R; ty < hkPuck || ty > hkH-hkPuck {
		cy := math.Max(hkPuck, math.Min(hkH-hkPuck, ty))
		if rest := R*R - (cy-my)*(cy-my); rest > 0 {
			sx := math.Sqrt(rest)
			if nx < 0 || (nx == 0 && mx > hkW/2) {
				cands = append(cands, [2]float64{mx - sx, cy}, [2]float64{mx + sx, cy})
			} else {
				cands = append(cands, [2]float64{mx + sx, cy}, [2]float64{mx - sx, cy})
			}
		}
	}
	// Then sweep the whole circle, nearest angle to the contact first, so a
	// puck squeezed between both mallets slips out sideways.
	base := math.Atan2(ny, nx)
	for step := 1; step <= 8; step++ {
		for _, sign := range []float64{1, -1} {
			a := base + sign*float64(step)*math.Pi/8
			cands = append(cands, [2]float64{mx + math.Cos(a)*R, my + math.Sin(a)*R})
		}
	}
	// A spot inside the other mallet is no escape either: a puck pinned
	// between both mallets on a board must come out into the open ice.
	ox, oy := st.Pads[1-i][0], st.Pads[1-i][1]
	free := func(x, y float64) bool { return inside(x, y) && math.Hypot(x-ox, y-oy) >= R-0.1 }
	chosen := cands[len(cands)-1]
	for _, c := range cands {
		if free(c[0], c[1]) {
			chosen = c
			break
		}
	}
	p[0] = math.Max(hkPuck, math.Min(hkW-hkPuck, chosen[0]))
	p[1] = math.Max(hkPuck, math.Min(hkH-hkPuck, chosen[1]))
	dx, dy = p[0]-mx, p[1]-my
	dist = math.Hypot(dx, dy)
	if dist < 1e-6 {
		return nx, ny
	}
	return dx / dist, dy / dist
}

func (k *hockeyKind) frame(m *Match) any {
	st := m.Data.(*hockeyState)
	return map[string]any{"puck": st.Puck, "pads": st.Pads, "score": st.Score, "phase": st.Phase, "scorer": st.Scorer}
}

func (k *hockeyKind) Timeout(m *Match, s *Service, ctx context.Context) {}

func (k *hockeyKind) Left(m *Match, s *Service, ctx context.Context, uid uint) {}

func (k *hockeyKind) View(m *Match, viewer uint) any {
	st := m.Data.(*hockeyState)
	return map[string]any{"puck": st.Puck, "pads": st.Pads, "score": st.Score, "target": st.Target, "phase": st.Phase, "mySeat": k.seat(m, viewer), "width": hkW, "height": hkH, "pad": hkPad, "puckR": hkPuck, "goal": hkGoal}
}

// ================================================================ Eskalasyon Bingo

type bingoState struct {
	Cards map[uint][]string `json:"cards"`
	Marks map[uint][]bool   `json:"marks"`
	Pool  []string          `json:"pool"`
}

type bingoKind struct{}

func (bingoKind) Meta() Meta {
	return Meta{Key: "bingo", Name: "Eskalasyon Bingo", Tagline: "Günün kendisi oyun.", Icon: "layout-grid",
		How:        "Herkese 5x5 bir kart: 'modem ışığı', 'şifre unuttum', 'diğer operatör'... Gün içinde başına gelen kutuyu işaretlersin; eskalasyon kategorisiyle aynı adlı kutular kayıt girince kendiliğinden işaretlenir. Bir satırı, sütunu ya da çaprazı ilk dolduran BINGO der ve kazanır. Sonradan katılan da kart alır.",
		MinPlayers: 1, MaxPlayers: 0, ItemKind: "bingo", ItemLabel: "Kutu metni", ItemHint: "Metin: karttaki ifade. Eskalasyon kategori adıyla birebir aynıysa kayıt girildiğinde kendiliğinden işaretlenir.", MinItems: 24,
		DefaultRounds: 1, DefaultSeconds: 0, JoinLate: true}
}
func (bingoKind) NewState() any { return &bingoState{} }

func (k *bingoKind) Start(m *Match, s *Service, ctx context.Context) error {
	st := m.Data.(*bingoState)
	items := s.pickItems(ctx, "bingo", 0)
	if len(items) < 24 {
		return errs.Invalid("Bingo için en az 24 kutu metni gerekli.", nil)
	}
	st.Pool = nil
	for _, it := range items {
		st.Pool = append(st.Pool, it.Text)
	}
	st.Cards = map[uint][]string{}
	st.Marks = map[uint][]bool{}
	for _, p := range m.active() {
		k.deal(m, s, p.UserID)
	}
	return nil
}

func (k *bingoKind) deal(m *Match, s *Service, uid uint) {
	st := m.Data.(*bingoState)
	pool := append([]string(nil), st.Pool...)
	s.rnd.Shuffle(len(pool), func(i, j int) { pool[i], pool[j] = pool[j], pool[i] })
	card := make([]string, 25)
	marks := make([]bool, 25)
	n := 0
	for i := 0; i < 25; i++ {
		if i == 12 {
			card[i] = "SERBEST"
			marks[i] = true
			continue
		}
		card[i] = pool[n%len(pool)]
		n++
	}
	st.Cards[uid] = card
	st.Marks[uid] = marks
}

func (k *bingoKind) Joined(m *Match, s *Service, ctx context.Context, uid uint) {
	st := m.Data.(*bingoState)
	if _, ok := st.Cards[uid]; !ok {
		k.deal(m, s, uid)
	}
}

func (k *bingoKind) complete(marks []bool) bool {
	at := func(r, c int) bool { return marks[r*5+c] }
	for i := 0; i < 5; i++ {
		row, col := true, true
		for j := 0; j < 5; j++ {
			row = row && at(i, j)
			col = col && at(j, i)
		}
		if row || col {
			return true
		}
	}
	d1, d2 := true, true
	for i := 0; i < 5; i++ {
		d1 = d1 && at(i, i)
		d2 = d2 && at(i, 4-i)
	}
	return d1 || d2
}

func (k *bingoKind) check(m *Match, s *Service, ctx context.Context, uid uint) {
	st := m.Data.(*bingoState)
	if k.complete(st.Marks[uid]) {
		m.addScore(uid, 5)
		s.finish(ctx, m, []uint{uid}, "BINGO")
	}
}

func (k *bingoKind) autoMark(m *Match, s *Service, ctx context.Context, uid uint, category string) bool {
	st := m.Data.(*bingoState)
	card, ok := st.Cards[uid]
	if !ok {
		return false
	}
	changed := false
	for i, text := range card {
		if !st.Marks[uid][i] && norm(text) == norm(category) {
			st.Marks[uid][i] = true
			changed = true
		}
	}
	if changed {
		k.check(m, s, ctx, uid)
	}
	return changed
}

func (k *bingoKind) Act(m *Match, s *Service, ctx context.Context, uid uint, action string, payload json.RawMessage) (bool, error) {
	st := m.Data.(*bingoState)
	if action != "mark" {
		return false, errs.Invalid("Bilinmeyen hamle.", nil)
	}
	var in struct{ Index int }
	if err := decode(payload, &in); err != nil {
		return false, err
	}
	marks, ok := st.Marks[uid]
	if !ok || in.Index < 0 || in.Index >= 25 || in.Index == 12 {
		return false, errs.Invalid("Geçersiz kutu.", nil)
	}
	marks[in.Index] = !marks[in.Index]
	k.check(m, s, ctx, uid)
	return true, nil
}

func (k *bingoKind) Timeout(m *Match, s *Service, ctx context.Context) {}

func (k *bingoKind) Left(m *Match, s *Service, ctx context.Context, uid uint) {}

func (k *bingoKind) View(m *Match, viewer uint) any {
	st := m.Data.(*bingoState)
	counts := map[uint]int{}
	for uid, marks := range st.Marks {
		n := 0
		for _, b := range marks {
			if b {
				n++
			}
		}
		counts[uid] = n
	}
	return map[string]any{"card": st.Cards[viewer], "marks": st.Marks[viewer], "counts": counts}
}
