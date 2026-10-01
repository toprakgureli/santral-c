package main

// The WhatsApp side of a busy hour: customers write in at once, the chatbot
// greets each with a menu and hands them over, agents take the
// conversations from the pool, answer, leave internal notes and close them,
// while Meta reports every message delivered and read. Meta is a stand-in
// on the loopback that records what the panel sends.

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

	"github.com/toprakgureli/santral-c/backend/internal/whatsapp"
	"github.com/toprakgureli/santral-c/backend/internal/whatsapp/meta"
	"github.com/toprakgureli/santral-c/backend/pkg/crypt"
	"github.com/toprakgureli/santral-c/backend/pkg/enums"
	"github.com/toprakgureli/santral-c/backend/pkg/safe"
)

// metaIPs are the addresses Meta's notices arrive from in the test: one,
// the hardest case for a limit counted per address.
var metaIPs = []string{"198.51.100.21"}

// sentMessage is one message the panel asked Meta to deliver.
type sentMessage struct {
	ID      string
	To      string
	Payload map[string]any
}

// fakeMeta stands in for the Graph API: it accepts messages and read marks
// and remembers them per customer.
type fakeMeta struct {
	srv   *httptest.Server
	mu    sync.Mutex
	next  int
	sent  map[string][]sentMessage // by customer number
	reads atomic.Int64
}

func newFakeMeta(t *testing.T) *fakeMeta {
	t.Helper()
	m := &fakeMeta{sent: map[string][]sentMessage{}}
	m.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/messages") {
			_, _ = w.Write([]byte(`{"data":[]}`))
			return
		}
		var p map[string]any
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &p)
		if p["status"] == "read" {
			m.reads.Add(1)
			_, _ = w.Write([]byte(`{"success":true}`))
			return
		}
		to, _ := p["to"].(string)
		m.mu.Lock()
		m.next++
		id := fmt.Sprintf("wamid.out.%d.%06d", time.Now().UnixNano(), m.next)
		m.sent[to] = append(m.sent[to], sentMessage{ID: id, To: to, Payload: p})
		m.mu.Unlock()
		_, _ = fmt.Fprintf(w, `{"messages":[{"id":%q}]}`, id)
	}))
	t.Cleanup(m.srv.Close)
	old := meta.GraphBase
	meta.GraphBase = m.srv.URL
	t.Cleanup(func() { meta.GraphBase = old })
	return m
}

// to returns a copy of what was sent to a customer.
func (m *fakeMeta) to(number string) []sentMessage {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]sentMessage(nil), m.sent[number]...)
}

// waitFor waits until fn holds for what was sent to number.
func (m *fakeMeta) waitFor(number string, within time.Duration, fn func([]sentMessage) bool) []sentMessage {
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if got := m.to(number); fn(got) {
			return got
		}
		time.Sleep(50 * time.Millisecond)
	}
	return nil
}

// waChannel is the device the test talks through.
type waChannel struct {
	id            uint
	phoneNumberID string
	hookKey       string
	appSecret     string
}

// seedChannel adds an active device with sealed credentials, as the panel
// stores them, and a long resolved history on it.
func seedChannel(t *testing.T, db *gorm.DB) waChannel {
	t.Helper()
	ring, err := crypt.NewKeyring(strings.Repeat("ab", 32))
	if err != nil {
		t.Fatal(err)
	}
	stamp := time.Now().UnixNano()
	ch := waChannel{phoneNumberID: fmt.Sprint(stamp % 1_000_000_000_000), hookKey: fmt.Sprintf("load-%d", stamp), appSecret: "load-app-secret"}
	token, err := ring.Seal(whatsapp.SealPurpose, "load-access-token")
	if err != nil {
		t.Fatal(err)
	}
	secret, err := ring.Seal(whatsapp.SealPurpose, ch.appSecret)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Raw(`INSERT INTO wa_channels (name, phone_number_id, waba_id, verify_token, hook_key, access_token_enc, app_secret_enc, active, display_phone)
		VALUES ('Yük hattı', ?, ?, 'vt', ?, ?, ?, true, '+90 555 000 00 00') RETURNING id`,
		ch.phoneNumberID, ch.phoneNumberID, ch.hookKey, token, secret).Scan(&ch.id).Error; err != nil {
		t.Fatal(err)
	}
	// Half a year of closed conversations on the same device.
	for _, q := range []string{
		`INSERT INTO wa_contacts (wa_id, peer_key, name)
		 SELECT '9054' || lpad(g::text, 9, '0') || ` + fmt.Sprint(ch.id%10) + `, '54' || lpad(g::text, 8, '0'), 'Eski Müşteri ' || g
		 FROM generate_series(1, 5000) g ON CONFLICT DO NOTHING`,
		`INSERT INTO wa_conversations (channel_id, contact_id, last_message_at)
		 SELECT ?, c.id, now() - (c.id % 180) * interval '1 day' FROM wa_contacts c WHERE c.name LIKE 'Eski Müşteri %'
		 ON CONFLICT DO NOTHING`,
		`INSERT INTO wa_tickets (conversation_id, channel_id, contact_id, status, resolved_at)
		 SELECT v.id, v.channel_id, v.contact_id, 'resolved', v.last_message_at FROM wa_conversations v WHERE v.channel_id = ?`,
		`UPDATE wa_conversations v SET ticket_id = t.id FROM wa_tickets t WHERE t.conversation_id = v.id AND v.channel_id = ?`,
		`INSERT INTO wa_messages (channel_id, conversation_id, ticket_id, direction, kind, body, created_at)
		 SELECT v.channel_id, v.id, v.ticket_id, CASE WHEN g % 2 = 0 THEN 'in' ELSE 'out' END, 'text', 'Eski yazışma ' || g,
		        v.last_message_at - (10 - g) * interval '1 minute'
		 FROM wa_conversations v, generate_series(1, 10) g WHERE v.channel_id = ?`,
	} {
		args := []any{}
		if strings.Contains(q, "?") {
			args = append(args, ch.id)
		}
		if err := db.Exec(q, args...).Error; err != nil {
			t.Fatalf("history: %v", err)
		}
	}
	t.Cleanup(func() { db.Exec("UPDATE wa_channels SET active = false WHERE id = ?", ch.id) })
	return ch
}

// hook posts a signed notice to the device's webhook from one of Meta's
// addresses and returns the answer code.
func hook(t *testing.T, app *fiber.App, ch waChannel, ip string, value map[string]any) int {
	t.Helper()
	value["messaging_product"] = "whatsapp"
	value["metadata"] = map[string]any{"phone_number_id": ch.phoneNumberID}
	body, _ := json.Marshal(map[string]any{
		"object": "whatsapp_business_account",
		"entry":  []any{map[string]any{"id": ch.phoneNumberID, "changes": []any{map[string]any{"field": "messages", "value": value}}}},
	})
	mac := hmac.New(sha256.New, []byte(ch.appSecret))
	mac.Write(body)
	req := httptest.NewRequest(fiber.MethodPost, "/api/v1/wa/hook/"+ch.hookKey, strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Hub-Signature-256", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	req.Header.Set(fiber.HeaderXForwardedFor, ip)
	res, err := app.Test(req, 60_000)
	if err != nil {
		t.Errorf("webhook: %v", err)
		return 0
	}
	_ = res.Body.Close()
	return res.StatusCode
}

func customerText(number, name, wamid, text string) map[string]any {
	return map[string]any{
		"contacts": []any{map[string]any{"profile": map[string]any{"name": name}, "wa_id": number}},
		"messages": []any{map[string]any{"from": number, "id": wamid, "timestamp": fmt.Sprint(time.Now().Unix()), "type": "text", "text": map[string]any{"body": text}}},
	}
}

func customerTap(number, wamid, id, title string) map[string]any {
	return map[string]any{
		"contacts": []any{map[string]any{"profile": map[string]any{"name": "x"}, "wa_id": number}},
		"messages": []any{map[string]any{"from": number, "id": wamid, "timestamp": fmt.Sprint(time.Now().Unix()), "type": "interactive",
			"interactive": map[string]any{"type": "button_reply", "button_reply": map[string]any{"id": id, "title": title}}}},
	}
}

func deliveryStatus(number, wamid, status string) map[string]any {
	return map[string]any{
		"statuses": []any{map[string]any{"id": wamid, "status": status, "timestamp": fmt.Sprint(time.Now().Unix()), "recipient_id": number}},
	}
}

// firstButton returns the id and title of the first button of an
// interactive message.
func firstButton(m sentMessage) (string, string) {
	in, _ := m.Payload["interactive"].(map[string]any)
	action, _ := in["action"].(map[string]any)
	buttons, _ := action["buttons"].([]any)
	if len(buttons) == 0 {
		return "", ""
	}
	b, _ := buttons[0].(map[string]any)
	reply, _ := b["reply"].(map[string]any)
	id, _ := reply["id"].(string)
	title, _ := reply["title"].(string)
	return id, title
}

// TestLoadWhatsApp: 150 customers write in within the same minute on a
// device with 5,000 old conversations. Each is greeted by the chatbot with
// a menu, taps a button and is handed to the pool. Ten agents take them,
// answer, add a note and close them, while Meta reports every message
// delivered and read. Every customer must get exactly one menu, the hand-
// over line and the agent's answer, in that order, and nothing in the
// panel may be refused for load.
func TestLoadWhatsApp(t *testing.T) {
	loadTest(t)
	fm := newFakeMeta(t)
	srv, db := testServer(t, officeSecurity)
	bigHistory(t, db)
	ch := seedChannel(t, db)

	// The background work runs as on the server: the webhook worker, the
	// inbound workers, the sender.
	ctx, cancel := context.WithCancel(context.Background())
	var g safe.Group
	srv.start(ctx, &g)
	t.Cleanup(func() {
		cancel()
		wait, stop := context.WithTimeout(context.Background(), 30*time.Second)
		defer stop()
		_ = g.Wait(wait)
	})

	stats := newTally()
	admin := newBrowser(t, srv.app, stats, seedPeople(t, db, 1, enums.RoleInvisibleAdmin, false)[0])
	admin.mustSignIn()
	agents := seedPeople(t, db, 10, enums.RoleSalesTeam, false)
	if a := admin.do(fiber.MethodPut, fmt.Sprintf("/api/v1/wa/channels/%d/members", ch.id), map[string]any{"userIds": ids(agents)}); a.status >= 300 {
		t.Fatalf("channel members answered %d %s", a.status, a.body)
	}

	// The chatbot: a two-button menu, either button hands over to a person.
	graph := map[string]any{
		"nodes": []any{
			map[string]any{"id": "s", "type": "start", "x": 0, "y": 0, "data": map[string]any{}},
			map[string]any{"id": "m", "type": "menu", "x": 0, "y": 0, "data": map[string]any{"text": "Merhaba, nasıl yardımcı olalım?", "style": "buttons",
				"options": []any{map[string]any{"id": "a", "label": "Satış"}, map[string]any{"id": "b", "label": "Destek"}}}},
			map[string]any{"id": "h", "type": "handoff", "x": 0, "y": 0, "data": map[string]any{"text": "Sizi bir temsilciye aktarıyorum."}},
		},
		"edges": []any{
			map[string]any{"from": "s", "to": "m"},
			map[string]any{"from": "m", "port": "a", "to": "h"},
			map[string]any{"from": "m", "port": "b", "to": "h"},
		},
	}
	a := admin.do(fiber.MethodPost, "/api/v1/wa/bots", map[string]any{"name": fmt.Sprintf("Yük botu %d", time.Now().UnixNano()), "trigger": "entry", "channelIds": []uint{ch.id}})
	if a.status >= 300 {
		t.Fatalf("bot create answered %d %s", a.status, a.body)
	}
	var bot struct {
		ID       uint           `json:"id"`
		Name     string         `json:"name"`
		Schedule map[string]any `json:"schedule"`
	}
	a.json(t, &bot)
	if a := admin.do(fiber.MethodPut, fmt.Sprintf("/api/v1/wa/bots/%d/draft", bot.ID), graph); a.status >= 300 {
		t.Fatalf("bot draft answered %d %s", a.status, a.body)
	}
	if a := admin.do(fiber.MethodPost, fmt.Sprintf("/api/v1/wa/bots/%d/publish", bot.ID), nil); a.status >= 300 {
		t.Fatalf("bot publish answered %d %s", a.status, a.body)
	}
	if a := admin.do(fiber.MethodPut, fmt.Sprintf("/api/v1/wa/bots/%d", bot.ID), map[string]any{"name": bot.Name, "description": "", "trigger": "entry",
		"keywords": []string{}, "schedule": bot.Schedule, "channelIds": []uint{ch.id}, "active": true}); a.status >= 300 {
		t.Fatalf("bot switch on answered %d %s", a.status, a.body)
	}
	t.Cleanup(func() { db.Exec("UPDATE wa_bots SET active = false WHERE id = ?", bot.ID) })

	const customers = 150
	numbers := make([]string, customers)
	for i := range numbers {
		numbers[i] = fmt.Sprintf("90553%03d%04d", ch.id%1000, i)
	}
	var refused, notices atomic.Int64
	post := func(i int, value map[string]any) {
		notices.Add(1)
		code := hook(t, srv.app, ch, metaIPs[i%len(metaIPs)], value)
		switch {
		case code == fiber.StatusTooManyRequests:
			refused.Add(1)
		case code >= 300:
			t.Errorf("webhook answered %d", code)
		}
	}
	// acknowledge reports each message sent to a customer as delivered and read.
	acked := map[string]bool{}
	var ackMu sync.Mutex
	acknowledge := func(i int) {
		for _, m := range fm.to(numbers[i]) {
			ackMu.Lock()
			done := acked[m.ID]
			acked[m.ID] = true
			ackMu.Unlock()
			if !done {
				post(i, deliveryStatus(numbers[i], m.ID, "delivered"))
				post(i, deliveryStatus(numbers[i], m.ID, "read"))
			}
		}
	}

	began := time.Now()
	// Customers write in; the chatbot answers each with its menu; each taps
	// the first button and is handed over.
	together(customers, func(i int) {
		post(i, customerText(numbers[i], fmt.Sprintf("Müşteri %d", i), fmt.Sprintf("wamid.in.%s.1", numbers[i]), "Merhaba, siparişim nerede?"))
		menu := fm.waitFor(numbers[i], 90*time.Second, func(s []sentMessage) bool { return len(s) >= 1 })
		if menu == nil {
			t.Errorf("customer %d never got the chatbot's menu", i)
			return
		}
		id, title := firstButton(menu[0])
		if id == "" {
			t.Errorf("customer %d: the first message is not a button menu: %v", i, menu[0].Payload)
			return
		}
		acknowledge(i)
		post(i, customerTap(numbers[i], fmt.Sprintf("wamid.in.%s.2", numbers[i]), id, title))
		if fm.waitFor(numbers[i], 90*time.Second, func(s []sentMessage) bool { return len(s) >= 2 }) == nil {
			t.Errorf("customer %d never got the hand-over line", i)
			return
		}
		acknowledge(i)
	})
	if t.Failed() {
		t.FailNow()
	}

	// Ten agents work the pool at the same time. Two of them may pick the
	// same conversation in the same instant; the first owns it, the second
	// joins as a helper and leaves the answer to the owner, and anyone later
	// is told a colleague took it (409, the only refusal allowed here).
	var handled, conflicts, helpers atomic.Int64
	var doneMu sync.Mutex
	done := map[uint]bool{}
	together(len(agents), func(k int) {
		b := newBrowser(t, srv.app, stats, agents[k])
		b.mustSignIn()
		deadline := time.Now().Add(2 * time.Minute)
		for time.Now().Before(deadline) && handled.Load() < customers {
			a := b.do(fiber.MethodGet, "/api/v1/wa/conversations", nil)
			if a.status != fiber.StatusOK {
				t.Errorf("%s: inbox answered %d %s", b.user.Email, a.status, a.body)
				return
			}
			var list struct {
				Items []struct {
					ID      uint `json:"id"`
					Contact struct {
						WaID string `json:"waId"`
					} `json:"contact"`
					Ticket *struct {
						Status string `json:"status"`
						Owner  *struct {
							ID uint `json:"id"`
						} `json:"owner"`
					} `json:"ticket"`
				} `json:"items"`
			}
			a.json(t, &list)
			took := 0
			for _, c := range list.Items {
				if c.Ticket == nil || c.Ticket.Status != "open" || c.Ticket.Owner != nil || !strings.HasPrefix(c.Contact.WaID, numbers[0][:8]) {
					continue
				}
				// "Karşıla" picks a waiting conversation from the pool. Another
				// agent may be faster; then this one moves on.
				if r := b.do(fiber.MethodPost, fmt.Sprintf("/api/v1/wa/conversations/%d/greet", c.ID), nil); r.status >= 300 {
					if r.status == fiber.StatusConflict {
						conflicts.Add(1)
					} else {
						t.Errorf("%s: greet answered %d, only 409 may refuse it: %s", b.user.Email, r.status, r.body)
					}
					continue
				}
				took++
				path := fmt.Sprintf("/api/v1/wa/conversations/%d", c.ID)
				// Whoever lost the race joined as a helper: the screen shows
				// the colleague as owner, and the owner answers.
				var conv struct {
					Ticket *struct {
						Owner *struct {
							ID uint `json:"id"`
						} `json:"owner"`
					} `json:"ticket"`
				}
				r := b.do(fiber.MethodGet, path, nil)
				if r.status != fiber.StatusOK {
					t.Errorf("%s: conversation answered %d %s", b.user.Email, r.status, r.body)
					continue
				}
				r.json(t, &conv)
				if conv.Ticket == nil || conv.Ticket.Owner == nil {
					t.Errorf("%s: conversation %d has no owner right after greeting it", b.user.Email, c.ID)
					continue
				}
				if conv.Ticket.Owner.ID != b.user.ID {
					helpers.Add(1)
					continue
				}
				for _, step := range []struct {
					path string
					body any
				}{
					{path + "/messages?limit=50", nil},
					{path + "/read", map[string]any{}},
					{path + "/notes", map[string]any{"body": "Sipariş numarası soruldu, kargoya bakılıyor."}},
					{path + "/messages", map[string]any{"kind": "text", "body": "Merhaba, siparişiniz yarın teslim edilecek.", "clientId": fmt.Sprintf("load-%d", c.ID)}},
					{path + "/resolve", map[string]any{}},
				} {
					method := fiber.MethodPost
					if step.body == nil {
						method = fiber.MethodGet
					}
					if r := b.do(method, step.path, step.body); r.status >= 300 {
						t.Errorf("%s: %s %s answered %d %s", b.user.Email, method, step.path, r.status, r.body)
					}
				}
				doneMu.Lock()
				if !done[c.ID] {
					done[c.ID] = true
					handled.Add(1)
				}
				doneMu.Unlock()
			}
			if took == 0 {
				time.Sleep(200 * time.Millisecond)
			}
		}
	})
	if n := handled.Load(); n != customers {
		t.Errorf("agents handled %d different conversations, want %d", n, customers)
	}
	t.Logf("whatsapp pool: %d greets refused with 409, %d joined as helper after losing the race", conflicts.Load(), helpers.Load())

	// Each conversation was claimed once: one owner, one claim recorded,
	// and one answer from an agent, even where two agents greeted it in
	// the same instant.
	var claims []struct {
		ConversationID uint
		Owners         int
		Claims         int
		Answers        int
	}
	db.Raw(`SELECT v.id AS conversation_id,
			(SELECT count(*) FROM wa_ticket_participants p WHERE p.ticket_id = v.ticket_id AND p.role = 'owner') AS owners,
			(SELECT count(*) FROM wa_assignments a WHERE a.ticket_id = v.ticket_id AND a.kind = 'claim') AS claims,
			(SELECT count(*) FROM wa_messages m WHERE m.conversation_id = v.id AND m.direction = 'out' AND m.sender_kind = 'agent') AS answers
		FROM wa_conversations v JOIN wa_contacts c ON c.id = v.contact_id
		WHERE v.channel_id = ? AND c.wa_id LIKE ?`, ch.id, numbers[0][:8]+"%").Scan(&claims)
	if len(claims) != customers {
		t.Errorf("%d conversations of this run found, want %d", len(claims), customers)
	}
	for _, c := range claims {
		if c.Owners != 1 || c.Claims != 1 || c.Answers != 1 {
			t.Errorf("conversation %d: %d owners, %d claims, %d agent answers; want one of each", c.ConversationID, c.Owners, c.Claims, c.Answers)
		}
	}

	// Every customer got the menu, the hand-over line and the agent's answer.
	last := time.Now().Add(time.Minute)
	for i, n := range numbers {
		got := fm.waitFor(n, time.Until(last), func(s []sentMessage) bool { return len(s) >= 3 })
		if got == nil {
			t.Errorf("customer %d got %d messages, want at least 3", i, len(fm.to(n)))
			continue
		}
		acknowledge(i)
		kinds := []string{}
		for _, m := range got[:3] {
			kind, _ := m.Payload["type"].(string)
			kinds = append(kinds, kind)
		}
		if strings.Join(kinds, ",") != "interactive,text,text" {
			t.Errorf("customer %d got %v, want the menu, the hand-over and the answer in that order", i, kinds)
		}
		menus := 0
		for _, m := range fm.to(n) {
			if m.Payload["type"] == "interactive" {
				menus++
			}
		}
		if menus != 1 {
			t.Errorf("customer %d got the menu %d times", i, menus)
		}
	}
	elapsed := time.Since(began)

	// What the panel stored matches.
	var notes, open int64
	db.Raw(`SELECT count(DISTINCT m.conversation_id) FROM wa_messages m JOIN wa_conversations v ON v.id = m.conversation_id
		WHERE v.channel_id = ? AND m.direction = 'note'`, ch.id).Scan(&notes)
	db.Raw(`SELECT count(*) FROM wa_tickets WHERE channel_id = ? AND status <> 'resolved'`, ch.id).Scan(&open)
	if notes != customers {
		t.Errorf("%d conversations hold an internal note, want %d", notes, customers)
	}
	if open != 0 {
		t.Errorf("%d conversations still open after every one was closed", open)
	}
	// Meta's delivery reports are worked through in the background.
	var unconfirmed int64
	for wait := time.Now().Add(time.Minute); ; {
		for i := range numbers {
			acknowledge(i)
		}
		db.Raw(`SELECT count(*) FROM wa_messages m JOIN wa_conversations v ON v.id = m.conversation_id
			WHERE v.channel_id = ? AND m.direction = 'out' AND m.wamid LIKE 'wamid.out.%' AND m.status <> 'read'`, ch.id).Scan(&unconfirmed)
		if unconfirmed == 0 || time.Now().After(wait) {
			break
		}
		time.Sleep(300 * time.Millisecond)
	}
	if unconfirmed != 0 {
		t.Errorf("%d sent messages never showed as read although Meta reported them", unconfirmed)
	}
	t.Logf("whatsapp: %d customers; %d notices from one Meta address in %s, %d refused for load; Meta got %d read marks from agents",
		customers, notices.Load(), time.Since(began).Round(time.Second), refused.Load(), fm.reads.Load())
	if n := refused.Load(); n > 0 {
		t.Errorf("%d of Meta's notices were refused for load (429); Meta retries them late", n)
	}
	// About 60 ms p95 and 450 ms at most on a developer machine.
	stats.report(t, "whatsapp panel", elapsed, budget{p95: time.Second, max: 5 * time.Second})
}
