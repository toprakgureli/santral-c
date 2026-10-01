package main

// WhatsApp on the database, under a busy hour: closing a chat while the
// customer writes, a returning customer and their former agent, the call
// checker's queue, reactions, a retry pressed twice, the read mark, and who
// sees which survey answers, callbacks, files and outside systems.
//
// The panel's requests go through the real routes. Meta's notices are
// handed to a second WhatsApp service on the same database, which runs the
// background work and records what would reach the panels live.

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"

	"github.com/toprakgureli/santral-c/backend/internal/audit"
	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
	"github.com/toprakgureli/santral-c/backend/internal/user"
	"github.com/toprakgureli/santral-c/backend/internal/whatsapp"
	"github.com/toprakgureli/santral-c/backend/pkg/crypt"
	"github.com/toprakgureli/santral-c/backend/pkg/enums"
	"github.com/toprakgureli/santral-c/backend/pkg/safe"
)

// ---------------------------------------------------------------- stand-ins

// liveRecorder stands in for the live stream: it keeps every event that
// would reach a panel and says everybody has the panel open.
type liveRecorder struct {
	mu     sync.Mutex
	events []whatsapp.Event
}

func (r *liveRecorder) Push(_ []uint, event any) {
	if e, ok := event.(whatsapp.Event); ok {
		r.mu.Lock()
		r.events = append(r.events, e)
		r.mu.Unlock()
	}
}

func (r *liveRecorder) OnlineUsers(ids []uint) map[uint]bool {
	out := make(map[uint]bool, len(ids))
	for _, id := range ids {
		out[id] = true
	}
	return out
}

func (r *liveRecorder) SubscribeRaw(uint) (chan []byte, func()) {
	return make(chan []byte), func() {}
}

// shown is what was published with a message, as the panel reads it.
type shown struct {
	ConversationID uint
	ID             uint   `json:"id"`
	Kind           string `json:"kind"`
	Reactions      []struct {
		Emoji string `json:"emoji"`
	} `json:"reactions"`
}

// messages lists the messages published since index from.
func (r *liveRecorder) messages(t *testing.T, from int) []shown {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []shown
	for _, e := range r.events[min(from, len(r.events)):] {
		if e.Message == nil {
			continue
		}
		raw, err := json.Marshal(e.Message)
		if err != nil {
			t.Fatal(err)
		}
		var s shown
		if err := json.Unmarshal(raw, &s); err != nil {
			t.Fatal(err)
		}
		s.ConversationID = e.ConversationID
		out = append(out, s)
	}
	return out
}

func (r *liveRecorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.events)
}

// memFiles stands in for the connected file storage.
type memFiles struct{}

func (memFiles) Connected(context.Context) bool { return true }
func (memFiles) Folder(context.Context) (string, error) {
	return "root", nil
}

func (memFiles) EnsureFolder(_ context.Context, name, _ string) (string, error) {
	return name, nil
}

func (memFiles) Put(_ context.Context, _, name, _ string, _ []byte) (string, error) {
	return "mem-" + name, nil
}

func (memFiles) Open(_ context.Context, id, _ string) (*http.Response, error) {
	body := "file " + id
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), ContentLength: int64(len(body)), Header: http.Header{}}, nil
}

// ---------------------------------------------------------------- the set-up

type waRig struct {
	srv   *server
	db    *gorm.DB
	svc   *whatsapp.Service
	live  *liveRecorder
	meta  *fakeMeta
	ch    waChannel
	stats *tally
	area  int
}

// newWARig builds the application, a device with half a year of history and
// the service that receives Meta's notices and does the work behind them.
func newWARig(t *testing.T, area int) *waRig {
	t.Helper()
	fm := newFakeMeta(t)
	srv, db := testServer(t, officeSecurity)
	ch := r3Channel(t, db)
	ring, err := crypt.NewKeyring(strings.Repeat("ab", 32))
	if err != nil {
		t.Fatal(err)
	}
	live := &liveRecorder{}
	svc := whatsapp.NewService(db, user.NewActors(db), live, memFiles{}, audit.NewService(db), ring, "route-test-signing-secret-0123456789abcdef")
	ctx, cancel := context.WithCancel(context.Background())
	var g safe.Group
	svc.Start(ctx, &g)
	t.Cleanup(func() {
		cancel()
		wait, stop := context.WithTimeout(context.Background(), 30*time.Second)
		defer stop()
		_ = g.Wait(wait)
	})
	return &waRig{srv: srv, db: db, svc: svc, live: live, meta: fm, ch: ch, stats: newTally(), area: area}
}

// r3Channel adds an active device with sealed credentials, as the panel
// stores them, and 600 closed conversations of ten messages each on it. The
// database around it already holds the long history of the load tests.
func r3Channel(t *testing.T, db *gorm.DB) waChannel {
	t.Helper()
	ring, err := crypt.NewKeyring(strings.Repeat("ab", 32))
	if err != nil {
		t.Fatal(err)
	}
	stamp := time.Now().UnixNano()
	ch := waChannel{phoneNumberID: fmt.Sprint(stamp % 1_000_000_000_000), hookKey: fmt.Sprintf("r3-%d", stamp), appSecret: "r3-app-secret"}
	token, err := ring.Seal(whatsapp.SealPurpose, "r3-access-token")
	if err != nil {
		t.Fatal(err)
	}
	secret, err := ring.Seal(whatsapp.SealPurpose, ch.appSecret)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Raw(`INSERT INTO wa_channels (name, phone_number_id, waba_id, verify_token, hook_key, access_token_enc, app_secret_enc, active, display_phone)
		VALUES ('Deneme hattı', ?, ?, 'vt', ?, ?, ?, true, '+90 555 000 00 02') RETURNING id`,
		ch.phoneNumberID, ch.phoneNumberID, ch.hookKey, token, secret).Scan(&ch.id).Error; err != nil {
		t.Fatal(err)
	}
	prefix := fmt.Sprintf("r3h%d-", ch.id)
	for _, q := range []string{
		`INSERT INTO wa_contacts (wa_id, peer_key, name)
		 SELECT ? || g, ? || g, 'Geçmiş müşteri ' || g FROM generate_series(1, 600) g`,
		`INSERT INTO wa_conversations (channel_id, contact_id, last_message_at)
		 SELECT ?, c.id, now() - (c.id % 180) * interval '1 day' FROM wa_contacts c WHERE c.wa_id LIKE ? || '%'`,
		`INSERT INTO wa_tickets (conversation_id, channel_id, contact_id, status, resolved_at)
		 SELECT v.id, v.channel_id, v.contact_id, 'resolved', v.last_message_at FROM wa_conversations v WHERE v.channel_id = ?`,
		`UPDATE wa_conversations v SET ticket_id = t.id FROM wa_tickets t WHERE t.conversation_id = v.id AND v.channel_id = ?`,
		`INSERT INTO wa_messages (channel_id, conversation_id, ticket_id, direction, kind, body, created_at)
		 SELECT v.channel_id, v.id, v.ticket_id, CASE WHEN g % 2 = 0 THEN 'in' ELSE 'out' END, 'text', 'Eski yazışma ' || g,
		        v.last_message_at - (10 - g) * interval '1 minute'
		 FROM wa_conversations v, generate_series(1, 10) g WHERE v.channel_id = ?`,
	} {
		var args []any
		switch strings.Count(q, "?") {
		case 1:
			args = []any{ch.id}
		case 2:
			if strings.Contains(q, "INSERT INTO wa_contacts") {
				args = []any{prefix, prefix}
			} else {
				args = []any{ch.id, prefix}
			}
		}
		if err := db.Exec(q, args...).Error; err != nil {
			t.Fatalf("history: %v", err)
		}
	}
	t.Cleanup(func() { db.Exec("UPDATE wa_channels SET active = false WHERE id = ?", ch.id) })
	return ch
}

// number is customer i's WhatsApp number on this device.
func (r *waRig) number(i int) string {
	return fmt.Sprintf("9055%d%03d%04d", r.area, r.ch.id%1000, i)
}

// receive hands Meta's signed notice to the service, as the webhook does.
func (r *waRig) receive(t *testing.T, value map[string]any) {
	t.Helper()
	value["messaging_product"] = "whatsapp"
	value["metadata"] = map[string]any{"phone_number_id": r.ch.phoneNumberID}
	body, _ := json.Marshal(map[string]any{
		"object": "whatsapp_business_account",
		"entry":  []any{map[string]any{"id": r.ch.phoneNumberID, "changes": []any{map[string]any{"field": "messages", "value": value}}}},
	})
	mac := hmac.New(sha256.New, []byte(r.ch.appSecret))
	mac.Write(body)
	if err := r.svc.Receive(context.Background(), r.ch.hookKey, "sha256="+hex.EncodeToString(mac.Sum(nil)), body); err != nil {
		t.Errorf("notice refused: %v", err)
	}
}

// writes has customer i write text, stamped at the given moment.
func (r *waRig) writes(t *testing.T, i int, wamid, text string, at time.Time) {
	t.Helper()
	n := r.number(i)
	r.receive(t, map[string]any{
		"contacts": []any{map[string]any{"profile": map[string]any{"name": fmt.Sprintf("Müşteri %d", i)}, "wa_id": n}},
		"messages": []any{map[string]any{"from": n, "id": wamid, "timestamp": fmt.Sprint(at.Unix()), "type": "text", "text": map[string]any{"body": text}}},
	})
}

// settled waits until a customer message is stored and its follow-up work
// is done, and returns its id.
func (r *waRig) settled(t *testing.T, wamid string) uint {
	t.Helper()
	var id uint
	eventually(t, 60*time.Second, "message "+wamid+" handled", func() bool {
		id = 0
		r.db.Raw(`SELECT m.id FROM wa_messages m WHERE m.wamid = ?
			AND NOT EXISTS (SELECT 1 FROM wa_inbound_jobs j WHERE j.message_id = m.id)`, wamid).Scan(&id)
		return id > 0
	})
	return id
}

// conversation is customer i's conversation and its ticket.
func (r *waRig) conversation(t *testing.T, i int) (uint, uint) {
	t.Helper()
	var row struct {
		ID       uint
		TicketID uint
	}
	r.db.Raw(`SELECT v.id, COALESCE(v.ticket_id, 0) AS ticket_id FROM wa_conversations v JOIN wa_contacts c ON c.id = v.contact_id
		WHERE c.wa_id = ? AND v.channel_id = ?`, r.number(i), r.ch.id).Scan(&row)
	if row.ID == 0 {
		t.Fatalf("customer %d has no conversation", i)
	}
	return row.ID, row.TicketID
}

// people adds n people with a role, placed on the device and signed in.
func (r *waRig) people(t *testing.T, n int, role enums.Role) []*browser {
	t.Helper()
	users := seedPeople(t, r.db, n, role, false)
	out := make([]*browser, n)
	for i, u := range users {
		r.place(t, u.ID, r.ch.id)
		out[i] = newBrowser(t, r.srv.app, r.stats, u)
		out[i].mustSignIn()
	}
	return out
}

// place puts a person on a device.
func (r *waRig) place(t *testing.T, userID, channelID uint) {
	t.Helper()
	if err := r.db.Exec("INSERT INTO wa_channel_members (channel_id, user_id) VALUES (?, ?) ON CONFLICT DO NOTHING", channelID, userID).Error; err != nil {
		t.Fatal(err)
	}
}

// withPermissions gives a person a role of their own holding exactly perms.
func withPermissions(t *testing.T, db *gorm.DB, userID uint, perms ...enums.Permission) {
	t.Helper()
	keys := make([]string, len(perms))
	for i, p := range perms {
		keys[i] = string(p)
	}
	var list []models.Permission
	if err := db.Where("key IN ?", keys).Find(&list).Error; err != nil || len(list) != len(perms) {
		t.Fatalf("permissions %v: %v", keys, err)
	}
	role := models.Role{Name: fmt.Sprintf("r3-%d-%d", userID, time.Now().UnixNano()), DisplayName: "Deneme rolü", Permissions: list}
	if err := db.Create(&role).Error; err != nil {
		t.Fatal(err)
	}
	db.Exec("DELETE FROM user_roles WHERE user_id = ?", userID)
	if err := db.Exec("INSERT INTO user_roles (user_id, role_id) VALUES (?, ?)", userID, role.ID).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.Exec("DELETE FROM user_roles WHERE role_id = ?", role.ID)
		db.Exec("DELETE FROM role_permissions WHERE role_id = ?", role.ID)
		db.Exec("DELETE FROM roles WHERE id = ?", role.ID)
	})
}

// eventually waits until ok holds, failing the test after within.
func eventually(t *testing.T, within time.Duration, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(within)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for: %s", what)
		}
		time.Sleep(40 * time.Millisecond)
	}
}

// newestMessage is the id of the newest message the agent's screen shows.
func newestMessage(t *testing.T, b *browser, conversationID uint) uint {
	t.Helper()
	a := b.do(fiber.MethodGet, fmt.Sprintf("/api/v1/wa/conversations/%d/messages", conversationID), nil)
	if a.status != fiber.StatusOK {
		t.Errorf("%s: messages answered %d %s", b.user.Email, a.status, a.body)
		return 0
	}
	var list []struct {
		ID uint `json:"id"`
	}
	a.json(t, &list)
	var top uint
	for _, m := range list {
		top = max(top, m.ID)
	}
	return top
}

func ticketStatus(db *gorm.DB, ticketID uint) string {
	var s string
	db.Raw("SELECT status FROM wa_tickets WHERE id = ?", ticketID).Scan(&s)
	return s
}

// ---------------------------------------------------------------- B1

// TestWAResolveWaitsForUnseenMessage: thirty customers on ten agents. Each
// customer writes again just before their agent presses "Çöz" with the
// screen they had; the chat stays open with a plain answer, and closes
// once the agent has seen the new message. A panel that sends no marker
// closes as before.
func TestWAResolveWaitsForUnseenMessage(t *testing.T) {
	r := newWARig(t, 1)
	began := time.Now()
	agents := r.people(t, 10, enums.RoleSalesTeam)
	const customers = 30
	together(customers, func(i int) {
		r.writes(t, i, fmt.Sprintf("wamid.r3.%s.1", r.number(i)), "Merhaba, siparişim nerede?", time.Now())
	})
	conv := make([]uint, customers)
	ticket := make([]uint, customers)
	seen := make([]uint, customers)
	for i := range customers {
		r.settled(t, fmt.Sprintf("wamid.r3.%s.1", r.number(i)))
		conv[i], ticket[i] = r.conversation(t, i)
	}
	together(customers, func(i int) {
		b := agents[i%len(agents)]
		if a := b.do(fiber.MethodPost, fmt.Sprintf("/api/v1/wa/conversations/%d/greet", conv[i]), nil); a.status >= 300 {
			t.Errorf("%s: greet answered %d %s", b.user.Email, a.status, a.body)
		}
		seen[i] = newestMessage(t, b, conv[i])
	})
	if t.Failed() {
		t.FailNow()
	}

	// The customer writes again; the agent closes with the screen they had.
	var refused atomic.Int64
	together(customers, func(i int) {
		b := agents[i%len(agents)]
		wamid := fmt.Sprintf("wamid.r3.%s.2", r.number(i))
		r.writes(t, i, wamid, "Bir şey daha soracaktım", time.Now())
		r.settled(t, wamid)
		a := b.do(fiber.MethodPost, fmt.Sprintf("/api/v1/wa/conversations/%d/resolve", conv[i]), map[string]any{"seenMessageId": seen[i]})
		if a.status != fiber.StatusConflict || !strings.Contains(string(a.body), "Müşteri az önce yeni bir mesaj yazdı. Sohbeti kapatmadan önce mesajı gör.") {
			t.Errorf("customer %d: closing over an unseen message answered %d %s", i, a.status, a.body)
			return
		}
		refused.Add(1)
		if s := ticketStatus(r.db, ticket[i]); s == "resolved" {
			t.Errorf("customer %d: the chat closed although the agent had not seen the new message", i)
		}
		// Seen now: it closes.
		newest := newestMessage(t, b, conv[i])
		if a := b.do(fiber.MethodPost, fmt.Sprintf("/api/v1/wa/conversations/%d/resolve", conv[i]), map[string]any{"seenMessageId": newest}); a.status >= 300 {
			t.Errorf("customer %d: closing after seeing answered %d %s", i, a.status, a.body)
		}
		if s := ticketStatus(r.db, ticket[i]); s != "resolved" {
			t.Errorf("customer %d: status %q after closing, want resolved", i, s)
		}
	})
	if refused.Load() != customers {
		t.Fatalf("%d of %d closes over an unseen message were refused", refused.Load(), customers)
	}

	// A panel from before the marker closes as it always did.
	r.writes(t, 0, fmt.Sprintf("wamid.r3.%s.3", r.number(0)), "Pardon, bir şey unuttum", time.Now())
	r.settled(t, fmt.Sprintf("wamid.r3.%s.3", r.number(0)))
	if s := ticketStatus(r.db, ticket[0]); s == "resolved" {
		t.Fatal("the customer's message did not reopen the chat")
	}
	if a := agents[0].do(fiber.MethodPost, fmt.Sprintf("/api/v1/wa/conversations/%d/resolve", conv[0]), nil); a.status >= 300 {
		t.Fatalf("closing without a marker answered %d %s", a.status, a.body)
	}
	if s := ticketStatus(r.db, ticket[0]); s != "resolved" {
		t.Fatalf("status %q after closing without a marker, want resolved", s)
	}
	r.stats.report(t, "whatsapp resolve", time.Since(began))
}

// ---------------------------------------------------------------- B2

// TestWAReturningCustomerAndTheirAgent: a customer who writes again after
// their chat was closed goes back to the same agent only when they came
// back within the device's return time (counted to their message) and the
// agent can answer now; otherwise the chat is handed out afresh.
func TestWAReturningCustomerAndTheirAgent(t *testing.T) {
	r := newWARig(t, 2)
	people := r.people(t, 2, enums.RoleSalesTeam)
	former, backup := people[0], people[1]
	shift := func(b *browser) {
		r.db.Exec("UPDATE shifts SET ended_at = now() WHERE user_id = ? AND ended_at IS NULL", b.user.ID)
		if err := r.db.Exec("INSERT INTO shifts (user_id) VALUES (?)", b.user.ID).Error; err != nil {
			t.Fatal(err)
		}
	}
	presence := func(b *browser, state string) {
		if err := r.db.Exec(`INSERT INTO agent_presence (user_id, state) VALUES (?, ?)
			ON CONFLICT (user_id) DO UPDATE SET state = EXCLUDED.state, updated_at = now()`, b.user.ID, state).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, b := range people {
		shift(b)
		presence(b, "available")
	}
	t.Cleanup(func() {
		r.db.Exec("UPDATE shifts SET ended_at = now() WHERE user_id IN ? AND ended_at IS NULL", []uint{former.user.ID, backup.user.ID})
	})
	settings := func(distribute bool) {
		raw := fmt.Sprintf(`{"returnMinutes":30,"distribution":{"enabled":%t,"maxOpen":0},"greeting":{"enabled":false}}`, distribute)
		if err := r.db.Exec("UPDATE wa_channels SET settings = ? WHERE id = ?", raw, r.ch.id).Error; err != nil {
			t.Fatal(err)
		}
	}

	type outcome struct {
		owner     uint
		autoAfter int64
		event     string
	}
	// run: the customer writes, the former agent takes and closes the chat,
	// before() sets the scene, the customer writes again at the given time.
	run := func(i int, before func(ticketID uint), at func() time.Time) outcome {
		t.Helper()
		settings(false)
		wamid := fmt.Sprintf("wamid.r3.%s.1", r.number(i))
		r.writes(t, i, wamid, "Merhaba", time.Now())
		r.settled(t, wamid)
		conv, ticket := r.conversation(t, i)
		if a := former.do(fiber.MethodPost, fmt.Sprintf("/api/v1/wa/conversations/%d/greet", conv), nil); a.status >= 300 {
			t.Fatalf("greet answered %d %s", a.status, a.body)
		}
		if a := former.do(fiber.MethodPost, fmt.Sprintf("/api/v1/wa/conversations/%d/resolve", conv), map[string]any{"seenMessageId": newestMessage(t, former, conv)}); a.status >= 300 {
			t.Fatalf("resolve answered %d %s", a.status, a.body)
		}
		before(ticket)
		var assigned int64
		r.db.Raw("SELECT count(*) FROM wa_assignments WHERE ticket_id = ? AND kind = 'auto'", ticket).Scan(&assigned)
		settings(true)
		wamid = fmt.Sprintf("wamid.r3.%s.2", r.number(i))
		r.writes(t, i, wamid, "Bir sorum daha var", at())
		r.settled(t, wamid)
		var out outcome
		r.db.Raw("SELECT COALESCE(owner_id, 0) FROM wa_tickets WHERE id = ?", ticket).Scan(&out.owner)
		r.db.Raw("SELECT count(*) FROM wa_assignments WHERE ticket_id = ? AND kind = 'auto'", ticket).Scan(&out.autoAfter)
		out.autoAfter -= assigned
		r.db.Raw("SELECT COALESCE(string_agg(body, ' | '), '') FROM wa_messages WHERE ticket_id = ? AND direction = 'event'", ticket).Scan(&out.event)
		presence(former, "available")
		return out
	}
	now := time.Now

	// Back within the time, the agent at their desk: straight to them.
	got := run(0, func(uint) {}, now)
	if got.owner != former.user.ID || got.autoAfter != 0 || !strings.Contains(got.event, "chatbot'a girmeden") {
		t.Errorf("back in time to an available agent: %+v, want the former agent kept", got)
	}

	// The agent is on a break: the chat goes to someone who can answer.
	got = run(1, func(uint) { presence(former, "break") }, now)
	if got.owner != backup.user.ID || got.autoAfter != 1 || !strings.Contains(got.event, "müsait olmadığı için") {
		t.Errorf("former agent on a break: %+v, want it handed to the colleague on duty", got)
	}
	var role string
	_, tk := r.conversation(t, 1)
	r.db.Raw("SELECT role FROM wa_ticket_participants WHERE ticket_id = ? AND user_id = ?", tk, former.user.ID).Scan(&role)
	if role != "helper" {
		t.Errorf("the former agent is %q on the chat, want helper", role)
	}

	// The agent's shift is over: handed out afresh too.
	got = run(2, func(uint) {
		r.db.Exec("UPDATE shifts SET ended_at = now() WHERE user_id = ? AND ended_at IS NULL", former.user.ID)
	}, now)
	shift(former)
	if got.owner != backup.user.ID || got.autoAfter != 1 {
		t.Errorf("former agent off shift: %+v, want it handed to the colleague on duty", got)
	}

	// Two hours after the close: handed out afresh, whoever gets it.
	got = run(3, func(ticket uint) {
		r.db.Exec("UPDATE wa_tickets SET resolved_at = now() - interval '2 hours' WHERE id = ?", ticket)
	}, now)
	if got.autoAfter != 1 || got.owner == 0 || strings.Contains(got.event, "chatbot'a girmeden") {
		t.Errorf("back two hours later: %+v, want it handed out afresh", got)
	}

	// The message was written ten minutes after the close, though it is
	// handled long after: the time counts to the message.
	got = run(4, func(ticket uint) {
		r.db.Exec("UPDATE wa_tickets SET resolved_at = now() - interval '2 hours' WHERE id = ?", ticket)
	}, func() time.Time { return time.Now().Add(-110 * time.Minute) })
	if got.owner != former.user.ID || got.autoAfter != 0 {
		t.Errorf("written ten minutes after the close: %+v, want the former agent kept", got)
	}
}

// ---------------------------------------------------------------- B4

// TestCallChecksSkipMissedRings: a queue call rings twenty agents and one
// answers, ten more calls are answered, and 250 older calls the phone
// records never showed are still waiting. The missed rings never wait for
// a check, and one pass of the checker confirms every answered call.
func TestCallChecksSkipMissedRings(t *testing.T) {
	srv, db := testServer(t, officeSecurity)
	stats := newTally()
	began := time.Now()
	users := seedPeople(t, db, 20, enums.RoleSalesTeam, true)
	browsers := make([]*browser, len(users))
	for i, u := range users {
		browsers[i] = newBrowser(t, srv.app, stats, u)
		browsers[i].mustSignIn()
	}
	run := time.Now().UnixNano()
	peer := func(k int) string { return fmt.Sprintf("0555%03d%04d", run%1000, k) }
	call := func(b *browser, callID, direction, number, disposition string) {
		for _, phase := range []map[string]any{
			{"callId": callID, "phase": "start", "direction": direction, "peer": number},
			{"callId": callID, "phase": "end", "disposition": disposition, "durationSeconds": 40},
		} {
			if a := b.do(fiber.MethodPost, "/api/v1/calls/log/", phase); a.status >= 300 {
				t.Errorf("%s: call log %v answered %d %s", b.user.Email, phase["phase"], a.status, a.body)
			}
		}
	}

	// The old backlog: answered calls looked for once already, never found.
	if err := db.Exec(`INSERT INTO call_logs (call_id, user_id, direction, peer_number, peer_key, disposition, started_at, ended_at, duration_seconds, hooks_done, hooks_next_at, hooks_tries)
		SELECT 'r3-old-' || ? || '-' || g, ?, 'outbound', '0555999' || lpad(g::text, 4, '0'), '555999' || lpad(g::text, 4, '0'), 'answered',
		       now() - interval '70 minutes', now() - interval '65 minutes', 40, false, now() - interval '1 minute', 1
		FROM generate_series(1, 250) g`, fmt.Sprint(run), users[0].ID).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Exec("DELETE FROM call_logs WHERE call_id LIKE ?", fmt.Sprintf("r3-%%-%d%%", run)) })

	// The queue call rings everyone; agent 0 answers.
	queueCall := peer(0)
	together(len(browsers), func(i int) {
		disposition := "no_answer"
		if i == 0 {
			disposition = "answered"
		}
		call(browsers[i], fmt.Sprintf("r3-ring-%d-%d", run, i), "inbound", queueCall, disposition)
	})
	// Ten agents make a call each that is answered.
	together(10, func(i int) {
		call(browsers[i+1], fmt.Sprintf("r3-out-%d-%d", run, i), "outbound", peer(i+1), "answered")
	})
	var waiting int64
	db.Raw("SELECT count(*) FROM call_logs WHERE call_id LIKE ? AND hooks_done = false", fmt.Sprintf("r3-ring-%d-%%", run)).Scan(&waiting)
	if waiting != 1 {
		t.Fatalf("%d rings of the queue call wait for a check, want only the answered one", waiting)
	}

	// The phone system's records arrive for the answered calls.
	at := time.Now().Add(-time.Minute)
	record := func(callID, ext, number string) {
		if err := db.Exec(`INSERT INTO pbx_cdrs (call_uuid, start_at, start_stamp, direction, caller_id_number, destination_number,
			caller_num, dest_num, caller_ext, dest_ext, talk_duration, answer_stamp) VALUES (?, ?, ?, 'outbound', ?, ?, '', ?, ?, ?, '40', ?)`,
			callID, at, at.Format(time.DateTime), ext, number, number, ext, ext, at.Add(2*time.Second).Format(time.DateTime)).Error; err != nil {
			t.Fatal(err)
		}
	}
	cdrs := []string{fmt.Sprintf("r3-cdr-%d-ring", run)}
	record(cdrs[0], *users[0].SIPExtension, queueCall)
	for i := range 10 {
		id := fmt.Sprintf("r3-cdr-%d-%d", run, i)
		cdrs = append(cdrs, id)
		record(id, *users[i+1].SIPExtension, peer(i+1))
	}
	t.Cleanup(func() { db.Exec("DELETE FROM pbx_cdrs WHERE call_uuid IN ?", cdrs) })

	srv.callLog.VerifyPending(t.Context())
	var left int64
	db.Raw("SELECT count(*) FROM call_logs WHERE (call_id LIKE ? OR call_id LIKE ?) AND hooks_done = false",
		fmt.Sprintf("r3-ring-%d-%%", run), fmt.Sprintf("r3-out-%d-%%", run)).Scan(&left)
	if left != 0 {
		t.Fatalf("%d answered calls still wait after one pass behind a backlog of 250", left)
	}

	// The backlog was looked at again and waits longer now; none is lost.
	var later, due int64
	db.Raw("SELECT count(*) FROM call_logs WHERE call_id LIKE ? AND hooks_done = false AND hooks_tries = 2 AND hooks_next_at > now()", fmt.Sprintf("r3-old-%d-%%", run)).Scan(&later)
	db.Raw("SELECT count(*) FROM call_logs WHERE call_id LIKE ? AND hooks_done = false AND hooks_next_at <= now()", fmt.Sprintf("r3-old-%d-%%", run)).Scan(&due)
	if later+due != 250 || later == 0 {
		t.Fatalf("backlog after one pass: %d pushed back, %d still due, want 250 kept and the checked ones pushed back", later, due)
	}
	srv.callLog.VerifyPending(t.Context())
	db.Raw("SELECT count(*) FROM call_logs WHERE call_id LIKE ? AND hooks_done = false AND hooks_next_at <= now()", fmt.Sprintf("r3-old-%d-%%", run)).Scan(&due)
	if due != 0 {
		t.Fatalf("%d backlog calls were never looked at in two passes", due)
	}
	stats.report(t, "call checks", time.Since(began))
}

// ---------------------------------------------------------------- B9

// TestWAReactionsAndRetry: a customer's reaction shows on the message it
// belongs to and never as a message of its own, also with many customers
// reacting at once; a retry pressed many times at once sends the message
// once.
func TestWAReactionsAndRetry(t *testing.T) {
	r := newWARig(t, 3)
	agents := r.people(t, 1, enums.RoleSalesTeam)
	const customers = 20
	together(customers, func(i int) {
		r.writes(t, i, fmt.Sprintf("wamid.r3.%s.1", r.number(i)), "Ürün elime ulaştı", time.Now())
	})
	targets := make([]uint, customers)
	for i := range customers {
		targets[i] = r.settled(t, fmt.Sprintf("wamid.r3.%s.1", r.number(i)))
	}
	from := r.live.count()
	together(customers, func(i int) {
		n := r.number(i)
		r.receive(t, map[string]any{
			"contacts": []any{map[string]any{"profile": map[string]any{"name": "x"}, "wa_id": n}},
			"messages": []any{map[string]any{"from": n, "id": fmt.Sprintf("wamid.r3.%s.react", n), "timestamp": fmt.Sprint(time.Now().Unix()), "type": "reaction",
				"reaction": map[string]any{"message_id": fmt.Sprintf("wamid.r3.%s.1", n), "emoji": "👍"}}},
		})
	})
	eventually(t, 30*time.Second, "every reaction published on its message", func() bool {
		got := map[uint]bool{}
		for _, m := range r.live.messages(t, from) {
			if len(m.Reactions) > 0 && m.Reactions[0].Emoji == "👍" {
				got[m.ID] = true
			}
		}
		for _, id := range targets {
			if !got[id] {
				return false
			}
		}
		return true
	})
	for _, m := range r.live.messages(t, from) {
		if m.Kind == "reaction" {
			t.Fatalf("a reaction was published as a message of its own: %+v", m)
		}
	}

	// A reaction pointing at another customer's message shows nothing of it.
	from = r.live.count()
	other := r.number(1)
	r.receive(t, map[string]any{
		"contacts": []any{map[string]any{"profile": map[string]any{"name": "x"}, "wa_id": r.number(0)}},
		"messages": []any{map[string]any{"from": r.number(0), "id": fmt.Sprintf("wamid.r3.%s.cross", r.number(0)), "timestamp": fmt.Sprint(time.Now().Unix()), "type": "reaction",
			"reaction": map[string]any{"message_id": fmt.Sprintf("wamid.r3.%s.1", other), "emoji": "😡"}}},
	})
	eventually(t, 30*time.Second, "the stray reaction stored", func() bool {
		var n int64
		r.db.Raw("SELECT count(*) FROM wa_messages WHERE wamid = ?", fmt.Sprintf("wamid.r3.%s.cross", r.number(0))).Scan(&n)
		return n == 1
	})
	time.Sleep(300 * time.Millisecond)
	for _, m := range r.live.messages(t, from) {
		if m.ID == targets[1] {
			t.Fatalf("customer 0's reaction published customer 1's message: %+v", m)
		}
	}

	// Retry: a failed message, the button pressed eight times at once.
	conv, ticket := r.conversation(t, 2)
	if a := agents[0].do(fiber.MethodPost, fmt.Sprintf("/api/v1/wa/conversations/%d/greet", conv), nil); a.status >= 300 {
		t.Fatalf("greet answered %d %s", a.status, a.body)
	}
	body := fmt.Sprintf("Kargo kodunuz R3-%d", time.Now().UnixNano())
	var failed uint
	if err := r.db.Raw(`INSERT INTO wa_messages (channel_id, conversation_id, ticket_id, direction, kind, body, status, sender_kind, sender_user_id, payload, failed_at, error_text, created_at)
		VALUES (?, ?, ?, 'out', 'text', ?, 'failed', 'agent', ?, ?, now(), 'Mesaj iletilemedi.', now()) RETURNING id`,
		r.ch.id, conv, ticket, body, agents[0].user.ID, fmt.Sprintf(`{"type":"text","text":{"body":%q}}`, body)).Scan(&failed).Error; err != nil {
		t.Fatal(err)
	}
	var ok, busy atomic.Int64
	together(8, func(int) {
		a := agents[0].do(fiber.MethodPost, fmt.Sprintf("/api/v1/wa/messages/%d/retry", failed), nil)
		switch {
		case a.status < 300:
			ok.Add(1)
		case a.status == fiber.StatusConflict && strings.Contains(string(a.body), "zaten yeniden gönderiliyor"):
			busy.Add(1)
		default:
			t.Errorf("retry answered %d %s", a.status, a.body)
		}
	})
	if ok.Load() != 1 || busy.Load() != 7 {
		t.Fatalf("eight retries at once: %d queued, %d told it is on its way; want 1 and 7", ok.Load(), busy.Load())
	}
	sentBody := func(m sentMessage) string {
		txt, _ := m.Payload["text"].(map[string]any)
		s, _ := txt["body"].(string)
		return s
	}
	count := func(s []sentMessage) int {
		n := 0
		for _, m := range s {
			if sentBody(m) == body {
				n++
			}
		}
		return n
	}
	if r.meta.waitFor(r.number(2), 30*time.Second, func(s []sentMessage) bool { return count(s) >= 1 }) == nil {
		t.Fatal("the retried message never went out")
	}
	time.Sleep(4 * time.Second) // one more turn of the sender
	if n := count(r.meta.to(r.number(2))); n != 1 {
		t.Fatalf("the retried message went out %d times, want once", n)
	}
}

// ---------------------------------------------------------------- D3

// TestWAReadMarkStaysInsideTheConversation: a read mark past the newest
// message is held to it, so the badge can always be cleared; "Okunmadı
// olarak işaretle" and reading again still work, and a mark spoiled before
// the check is put right by the next read.
func TestWAReadMarkStaysInsideTheConversation(t *testing.T) {
	r := newWARig(t, 4)
	b := r.people(t, 1, enums.RoleSalesTeam)[0]
	for k := 1; k <= 3; k++ {
		wamid := fmt.Sprintf("wamid.r3.%s.%d", r.number(0), k)
		r.writes(t, 0, wamid, fmt.Sprintf("Mesaj %d", k), time.Now())
		r.settled(t, wamid)
	}
	conv, _ := r.conversation(t, 0)
	if a := b.do(fiber.MethodPost, fmt.Sprintf("/api/v1/wa/conversations/%d/greet", conv), nil); a.status >= 300 {
		t.Fatalf("greet answered %d %s", a.status, a.body)
	}
	mark := func() (uint, int) {
		var row struct {
			TeamReadID uint
			Unread     int
		}
		r.db.Raw("SELECT team_read_id, unread FROM wa_conversations WHERE id = ?", conv).Scan(&row)
		return row.TeamReadID, row.Unread
	}
	var newest, lastIn uint
	r.db.Raw("SELECT max(id) FROM wa_messages WHERE conversation_id = ?", conv).Scan(&newest)
	r.db.Raw("SELECT max(id) FROM wa_messages WHERE conversation_id = ? AND direction = 'in'", conv).Scan(&lastIn)
	read := func(id uint) {
		t.Helper()
		if a := b.do(fiber.MethodPost, fmt.Sprintf("/api/v1/wa/conversations/%d/read", conv), map[string]any{"messageId": id}); a.status >= 300 {
			t.Fatalf("read answered %d %s", a.status, a.body)
		}
	}

	read(4_000_000_000)
	if id, unread := mark(); id != newest || unread != 0 {
		t.Fatalf("after a read far past the end: mark %d unread %d, want %d and 0", id, unread, newest)
	}
	// Mark unread, then read again: the badge comes and goes.
	if a := b.do(fiber.MethodPost, fmt.Sprintf("/api/v1/wa/conversations/%d/unread", conv), nil); a.status >= 300 {
		t.Fatalf("unread answered %d %s", a.status, a.body)
	}
	if id, unread := mark(); id >= lastIn || unread < 1 {
		t.Fatalf("after marking unread: mark %d unread %d, want below %d and a badge", id, unread, lastIn)
	}
	read(lastIn)
	if id, unread := mark(); id != lastIn || unread != 0 {
		t.Fatalf("after reading again: mark %d unread %d, want %d and 0", id, unread, lastIn)
	}
	// A new message brings the badge back, and reading it clears it.
	wamid := fmt.Sprintf("wamid.r3.%s.4", r.number(0))
	r.writes(t, 0, wamid, "Mesaj 4", time.Now())
	fresh := r.settled(t, wamid)
	if _, unread := mark(); unread != 1 {
		t.Fatalf("a new message shows %d unread, want 1", unread)
	}
	read(fresh)
	if id, unread := mark(); id != fresh || unread != 0 {
		t.Fatalf("after reading the new message: mark %d unread %d, want %d and 0", id, unread, fresh)
	}
	// A mark spoiled before the check is put right by the next read.
	r.db.Exec("UPDATE wa_conversations SET team_read_id = 4000000000, unread = 2 WHERE id = ?", conv)
	read(fresh)
	if id, unread := mark(); id != fresh || unread != 0 {
		t.Fatalf("a spoiled mark after a read: mark %d unread %d, want %d and 0", id, unread, fresh)
	}
}

// ---------------------------------------------------------------- D2

// lightChannel adds a second device with no history.
func lightChannel(t *testing.T, db *gorm.DB) uint {
	t.Helper()
	stamp := time.Now().UnixNano()
	var id uint
	if err := db.Raw(`INSERT INTO wa_channels (name, phone_number_id, waba_id, verify_token, hook_key, access_token_enc, app_secret_enc, active, display_phone)
		VALUES ('İkinci hat', ?, ?, 'vt', ?, '', '', true, '+90 555 000 00 01') RETURNING id`,
		fmt.Sprint(stamp%1_000_000_000_000+7), fmt.Sprint(stamp), fmt.Sprintf("r3-%d", stamp)).Scan(&id).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Exec("UPDATE wa_channels SET active = false WHERE id = ?", id) })
	return id
}

// TestWAScopeOfReportsCallbacksFilesAndTests: survey answers, callbacks and
// uploaded files show only to those who may see them, a hidden customer
// number shows only its last digits, and testing a chatbot reaches outside
// systems only with the right to change chatbots.
func TestWAScopeOfReportsCallbacksFilesAndTests(t *testing.T) {
	r := newWARig(t, 5)
	other := lightChannel(t, r.db)
	// Each person gets their role before signing in, so no cached copy of
	// them carries an older one.
	roles := [][]enums.Permission{
		{enums.WAView, enums.WAReports, enums.WACallbacks},
		{enums.WAView, enums.WAReports, enums.CallViewPeers},
		{enums.WAView, enums.WAViewAll, enums.WAReports, enums.WACallbacks},
		{enums.WAView, enums.WABotPublish},
		{enums.WAView, enums.WABotManage},
	}
	users := seedPeople(t, r.db, len(roles), enums.RoleSalesTeam, false)
	people := make([]*browser, len(users))
	for i, u := range users {
		withPermissions(t, r.db, u.ID, roles[i]...)
		r.place(t, u.ID, r.ch.id)
		people[i] = newBrowser(t, r.srv.app, r.stats, u)
		people[i].mustSignIn()
	}
	viewer, peers, manager, publisher, builder := people[0], people[1], people[2], people[3], people[4]
	stamp := time.Now().UnixNano()

	// Survey answers: one on this device, one on a device the viewer is not
	// on, one about the viewer's own call.
	type answer struct {
		channel uint
		user    uint
		waID    string
		comment string
	}
	answers := []answer{
		{r.ch.id, manager.user.ID, r.number(1), "Çok ilgiliydi"},
		{other, manager.user.ID, r.number(2), "Başka hattın yorumu"},
		{r.ch.id, viewer.user.ID, r.number(3), "Kendi görüşmem"},
	}
	for i, a := range answers {
		if err := r.db.Exec(`INSERT INTO wa_call_surveys (call_id, user_id, peer_key, wa_id, channel_id, status, score, comment, answered_at)
			VALUES (?, ?, ?, ?, ?, 'answered', 5, ?, now())`, fmt.Sprintf("r3-cs-%d-%d", stamp, i), a.user, a.waID[2:], a.waID, a.channel, a.comment).Error; err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		r.db.Exec("DELETE FROM wa_call_surveys WHERE call_id LIKE ?", fmt.Sprintf("r3-cs-%d-%%", stamp))
	})
	day := time.Now().In(time.FixedZone("TR", 3*3600)).Format("2006-01-02")
	report := func(b *browser) map[string]string {
		t.Helper()
		a := b.do(fiber.MethodGet, "/api/v1/wa/call-survey/report?from="+day+"&to="+day, nil)
		if a.status != fiber.StatusOK {
			t.Fatalf("%s: report answered %d %s", b.user.Email, a.status, a.body)
		}
		var rep struct {
			Answered int64 `json:"answered"`
			Recent   []struct {
				Phone   string `json:"phone"`
				Comment string `json:"comment"`
			} `json:"recent"`
		}
		a.json(t, &rep)
		out := map[string]string{}
		for _, x := range rep.Recent {
			out[x.Comment] = x.Phone
		}
		return out
	}
	seen := report(viewer)
	if _, ok := seen["Başka hattın yorumu"]; ok {
		t.Error("the viewer sees a survey answer from a device they are not on")
	}
	if p := seen["Çok ilgiliydi"]; p == r.number(1) || !strings.HasSuffix(p, r.number(1)[10:]) || !strings.Contains(p, "•") {
		t.Errorf("a colleague's customer shows as %q to the viewer, want all but the last two digits hidden", p)
	}
	if p := seen["Kendi görüşmem"]; p != r.number(3) {
		t.Errorf("the viewer's own customer shows as %q, want %q", p, r.number(3))
	}
	if p := report(peers)["Çok ilgiliydi"]; p != r.number(1) {
		t.Errorf("someone who may see colleagues' numbers gets %q, want %q", p, r.number(1))
	}
	if _, ok := report(manager)["Başka hattın yorumu"]; !ok {
		t.Error("someone who sees every device misses an answer")
	}

	// Callbacks.
	r.writes(t, 1, fmt.Sprintf("wamid.r3.%s.1", r.number(1)), "Beni arar mısınız?", time.Now())
	r.settled(t, fmt.Sprintf("wamid.r3.%s.1", r.number(1)))
	var here, there uint
	r.db.Raw("INSERT INTO wa_callbacks (channel_id, phone, note) VALUES (?, ?, 'Bu hattan') RETURNING id", r.ch.id, "+"+r.number(1)).Scan(&here)
	r.db.Raw("INSERT INTO wa_callbacks (channel_id, phone, note) VALUES (?, ?, 'Öbür hattan') RETURNING id", other, "+"+r.number(2)).Scan(&there)
	t.Cleanup(func() { r.db.Exec("DELETE FROM wa_callbacks WHERE id IN ?", []uint{here, there}) })
	listed := func(b *browser) map[uint]bool {
		t.Helper()
		a := b.do(fiber.MethodGet, "/api/v1/wa/callbacks?all=1", nil)
		if a.status != fiber.StatusOK {
			t.Fatalf("%s: callbacks answered %d %s", b.user.Email, a.status, a.body)
		}
		var list []struct {
			ID uint `json:"id"`
		}
		a.json(t, &list)
		out := map[uint]bool{}
		for _, c := range list {
			out[c.ID] = true
		}
		return out
	}
	if got := listed(viewer); !got[here] || got[there] {
		t.Errorf("the viewer's callbacks: this device %v, the other %v; want only this device's", got[here], got[there])
	}
	if a := viewer.do(fiber.MethodPost, fmt.Sprintf("/api/v1/wa/callbacks/%d/done", there), nil); a.status != fiber.StatusNotFound {
		t.Errorf("closing another device's callback answered %d %s, want 404", a.status, a.body)
	}
	var status string
	r.db.Raw("SELECT status FROM wa_callbacks WHERE id = ?", there).Scan(&status)
	if status != "open" {
		t.Errorf("another device's callback is %q, want open", status)
	}
	if a := viewer.do(fiber.MethodPost, fmt.Sprintf("/api/v1/wa/callbacks/%d/done", here), nil); a.status >= 300 {
		t.Errorf("closing this device's callback answered %d %s", a.status, a.body)
	}
	if got := listed(manager); !got[there] {
		t.Error("someone who sees every device misses a callback")
	}

	// Files: one the viewer uploaded, one a chatbot uses, one nobody uses.
	file := func(by uint, name string) uint {
		var id uint
		r.db.Raw("INSERT INTO wa_files (storage_id, name, mime, size, created_by) VALUES (?, ?, 'image/png', 10, ?) RETURNING id", "mem-"+name, name, by).Scan(&id)
		return id
	}
	mine, botFile, loose := file(viewer.user.ID, "kendi.png"), file(manager.user.ID, "bot.png"), file(manager.user.ID, "baska.png")
	var bot uint
	r.db.Raw(`INSERT INTO wa_bots (name, draft) VALUES (?, ?) RETURNING id`, fmt.Sprintf("Dosyalı bot %d", stamp),
		fmt.Sprintf(`{"nodes":[{"id":"s","type":"start","data":{}},{"id":"m","type":"media","data":{"fileId":%d,"mediaKind":"image"}}],"edges":[]}`, botFile)).Scan(&bot)
	t.Cleanup(func() {
		r.db.Exec("DELETE FROM wa_bots WHERE id = ?", bot)
		r.db.Exec("DELETE FROM wa_files WHERE id IN ?", []uint{mine, botFile, loose})
	})
	opens := func(b *browser, id uint) bool {
		m, err := r.svc.OpenFile(context.Background(), b.user.ID, id, "")
		if err != nil {
			return false
		}
		_ = m.Body.Close()
		return true
	}
	for _, c := range []struct {
		who  *browser
		file uint
		want bool
	}{
		{viewer, mine, true}, {viewer, botFile, false}, {viewer, loose, false},
		{builder, botFile, true}, {builder, loose, false}, {builder, mine, false},
		{publisher, botFile, true},
	} {
		if got := opens(c.who, c.file); got != c.want {
			t.Errorf("%s opening file %d: %v, want %v", c.who.user.Email, c.file, got, c.want)
		}
	}
	if a := viewer.do(fiber.MethodGet, fmt.Sprintf("/api/v1/wa/files/%d", loose), nil); a.status != fiber.StatusNotFound {
		t.Errorf("a guessed file number answered %d, want 404", a.status)
	}

	// Testing a chatbot with an outside system.
	var hits atomic.Int64
	outsideSys := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		_, _ = w.Write([]byte(`{"durum":"kargoda"}`))
	}))
	t.Cleanup(outsideSys.Close)
	var integration uint
	r.db.Raw("INSERT INTO wa_integrations (name, url) VALUES (?, ?) RETURNING id", fmt.Sprintf("Kargo %d", stamp), outsideSys.URL+"/kargo").Scan(&integration)
	t.Cleanup(func() { r.db.Exec("DELETE FROM wa_integrations WHERE id = ?", integration) })
	graph := map[string]any{
		"nodes": []any{
			map[string]any{"id": "s", "type": "start", "data": map[string]any{}},
			map[string]any{"id": "a", "type": "api", "data": map[string]any{"integration": integration}},
			map[string]any{"id": "ok", "type": "message", "data": map[string]any{"text": "Kargonuz yolda"}},
			map[string]any{"id": "no", "type": "message", "data": map[string]any{"text": "Bakamadık"}},
		},
		"edges": []any{
			map[string]any{"from": "s", "to": "a"},
			map[string]any{"from": "a", "port": "ok", "to": "ok"},
			map[string]any{"from": "a", "port": "fail", "to": "no"},
		},
	}
	simulate := func(b *browser) string {
		t.Helper()
		a := b.do(fiber.MethodPost, "/api/v1/wa/bots/simulate", map[string]any{"graph": graph, "start": true, "hoursOpen": true})
		if a.status != fiber.StatusOK {
			t.Fatalf("%s: simulate answered %d %s", b.user.Email, a.status, a.body)
		}
		var res struct {
			Outputs []struct {
				Kind   string `json:"kind"`
				Detail string `json:"detail"`
			} `json:"outputs"`
		}
		a.json(t, &res)
		for _, o := range res.Outputs {
			if o.Kind == "api" {
				return o.Detail
			}
		}
		return ""
	}
	if d := simulate(publisher); !strings.Contains(d, "çağrılmadı") || hits.Load() != 0 {
		t.Errorf("a publish-only test: %q with %d calls out, want the system not called", d, hits.Load())
	}
	if d := simulate(builder); d == "" || strings.Contains(d, "çağrılmadı") {
		t.Errorf("a chatbot builder's test: %q, want the system really asked", d)
	}
}
