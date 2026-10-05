package main

import (
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/toprakgureli/santral-c/backend/pkg/enums"
)

// TestRatingDeleteHidesTheScoreAndKeepsIt: only someone with
// whatsapp.rating_delete takes a score out; it leaves the list, the totals
// and the averages, while its original values stay with the record and the
// removal lands in the audit trail.
func TestRatingDeleteHidesTheScoreAndKeepsIt(t *testing.T) {
	srv, db := testServer(t)
	viewer := person(t, db, "Puanlara bakan", false, customRole(t, db, enums.WAView, enums.WARatings))
	remover := person(t, db, "Puan silen", false, customRole(t, db, enums.WAView, enums.WARatings, enums.WARatingDelete))

	stamp := time.Now().UnixNano()
	day := time.Date(2025, 3, 15, 10, 0, 0, 0, time.UTC)
	var channel, contact, conversation, ticket, survey uint
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(db.Raw(`INSERT INTO wa_channels (name, phone_number_id, waba_id, verify_token, hook_key, access_token_enc, app_secret_enc, active, display_phone)
		VALUES ('Puan hattı', ?, ?, 'vt', ?, 'x', 'x', true, '+90 555 000 00 09') RETURNING id`,
		fmt.Sprintf("rd%d", stamp), fmt.Sprintf("rd%d", stamp), fmt.Sprintf("rd-%d", stamp)).Scan(&channel).Error)
	must(db.Raw(`INSERT INTO wa_contacts (wa_id, peer_key, name) VALUES (?, ?, 'Puan veren') RETURNING id`,
		fmt.Sprintf("90555%07d", stamp%10000000), fmt.Sprintf("5%09d", stamp%1000000000)).Scan(&contact).Error)
	must(db.Raw(`INSERT INTO wa_conversations (channel_id, contact_id, last_message_at) VALUES (?, ?, ?) RETURNING id`, channel, contact, day).Scan(&conversation).Error)
	must(db.Raw(`INSERT INTO wa_tickets (conversation_id, channel_id, contact_id, status, resolved_at, rating, rating_comment, rated_at)
		VALUES (?, ?, ?, 'resolved', ?, 5, 'Çok iyiydi', ?) RETURNING id`, conversation, channel, contact, day, day).Scan(&ticket).Error)
	must(db.Raw(`INSERT INTO wa_call_surveys (call_id, peer_key, wa_id, channel_id, status, score, comment, answered_at, created_at)
		VALUES (?, '5550000009', '905550000009', ?, 'answered', 2, 'Beklettiler', ?, ?) RETURNING id`,
		fmt.Sprintf("rd-call-%d", stamp), channel, day, day).Scan(&survey).Error)
	t.Cleanup(func() {
		db.Exec("DELETE FROM wa_call_surveys WHERE id = ?", survey)
		db.Exec("UPDATE wa_conversations SET ticket_id = NULL WHERE id = ?", conversation)
		db.Exec("DELETE FROM wa_tickets WHERE id = ?", ticket)
		db.Exec("DELETE FROM wa_conversations WHERE id = ?", conversation)
		db.Exec("DELETE FROM wa_contacts WHERE id = ?", contact)
		db.Exec("DELETE FROM wa_channels WHERE id = ?", channel)
		db.Exec("DELETE FROM audit_log WHERE action = ? AND actor_id = ?", enums.AuditWARatingDeleted, remover.ID)
	})

	list := "/api/v1/wa/ratings?from=2025-03-15&to=2025-03-15&channel=" + strconv.FormatUint(uint64(channel), 10)
	type view struct {
		Count   int64   `json:"count"`
		Average float64 `json:"average"`
		Items   []struct {
			Source string `json:"source"`
			ID     uint   `json:"id"`
		} `json:"items"`
	}
	read := func(b *browser) view {
		t.Helper()
		a := b.do(fiber.MethodGet, list, nil)
		if a.status != fiber.StatusOK {
			t.Fatalf("ratings answered %d %s", a.status, a.body)
		}
		var v view
		a.json(t, &v)
		return v
	}
	vb, rb := signInAs(t, srv, viewer), signInAs(t, srv, remover)
	before := read(rb)
	if before.Count != 2 || before.Average != 3.5 {
		t.Fatalf("before: count %d average %v, want 2 and 3.5", before.Count, before.Average)
	}
	ids := map[string]uint{}
	for _, it := range before.Items {
		ids[it.Source] = it.ID
	}
	if ids["chat"] != ticket || ids["call"] != survey {
		t.Fatalf("items carry ids %v, want chat %d and call %d", ids, ticket, survey)
	}

	chatPath := fmt.Sprintf("/api/v1/wa/ratings/chat/%d", ticket)
	if a := vb.do(fiber.MethodDelete, chatPath, nil); a.status != fiber.StatusForbidden {
		t.Fatalf("someone without the permission removed a score: %d", a.status)
	}
	if a := rb.do(fiber.MethodDelete, chatPath, nil); a.status >= 300 {
		t.Fatalf("removing the chat score answered %d %s", a.status, a.body)
	}
	after := read(rb)
	if after.Count != 1 || after.Average != 2 {
		t.Fatalf("after one removal: count %d average %v, want 1 and 2", after.Count, after.Average)
	}
	if a := rb.do(fiber.MethodDelete, fmt.Sprintf("/api/v1/wa/ratings/call/%d", survey), nil); a.status >= 300 {
		t.Fatalf("removing the call score answered %d %s", a.status, a.body)
	}
	if v := read(rb); v.Count != 0 {
		t.Fatalf("after both removals the list still counts %d", v.Count)
	}
	if a := rb.do(fiber.MethodDelete, chatPath, nil); a.status != fiber.StatusNotFound {
		t.Errorf("removing it again answered %d, want 404", a.status)
	}
	if a := rb.do(fiber.MethodDelete, fmt.Sprintf("/api/v1/wa/ratings/other/%d", ticket), nil); a.status != fiber.StatusBadRequest {
		t.Errorf("an unknown source answered %d, want 400", a.status)
	}

	// Nothing was erased: the original values sit with the record.
	var kept struct {
		Chat, Call, Status string
	}
	db.Raw(`SELECT
		(SELECT rating_removed->>'score' FROM wa_tickets WHERE id = ?) AS chat,
		(SELECT removed->>'score' FROM wa_call_surveys WHERE id = ?) AS call,
		(SELECT status FROM wa_call_surveys WHERE id = ?) AS status`, ticket, survey, survey).Scan(&kept)
	if kept.Chat != "5" || kept.Call != "2" || kept.Status != "removed" {
		t.Errorf("kept %+v, want the scores 5 and 2 and the survey marked removed", kept)
	}
	var audited int64
	db.Raw("SELECT count(*) FROM audit_log WHERE action = ? AND actor_id = ?", enums.AuditWARatingDeleted, remover.ID).Scan(&audited)
	if audited != 2 {
		t.Errorf("audit trail holds %d removals, want 2", audited)
	}
}
