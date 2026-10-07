package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/toprakgureli/santral-c/backend/pkg/enums"
)

// TestRatingLinkOpensTheRatingsWithoutSigningIn: only someone with
// whatsapp.rating_link makes a link; whoever holds it reads the ratings
// without a session, with customers' names and numbers cut short and no way
// to search by number; a link altered by one character, cancelled, or whose
// maker can no longer see the ratings stops working; making and cancelling
// are audited.
func TestRatingLinkOpensTheRatingsWithoutSigningIn(t *testing.T) {
	srv, db := testServer(t)
	viewer := person(t, db, "Puanlara bakan", false, customRole(t, db, enums.WAView, enums.WARatings))
	sharerRole := customRole(t, db, enums.WAView, enums.WARatings, enums.WARatingLink)
	sharer := person(t, db, "Link paylaşan", false, sharerRole)

	stamp := time.Now().UnixNano()
	day := time.Date(2025, 4, 10, 10, 0, 0, 0, time.UTC)
	waID := fmt.Sprintf("90555%07d", stamp%10000000)
	var channel, contact, conversation, ticket uint
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(db.Raw(`INSERT INTO wa_channels (name, phone_number_id, waba_id, verify_token, hook_key, access_token_enc, app_secret_enc, active, display_phone)
		VALUES ('Paylaşım hattı', ?, ?, 'vt', ?, 'x', 'x', true, '+90 555 000 00 08') RETURNING id`,
		fmt.Sprintf("rl%d", stamp), fmt.Sprintf("rl%d", stamp), fmt.Sprintf("rl-%d", stamp)).Scan(&channel).Error)
	must(db.Raw(`INSERT INTO wa_contacts (wa_id, peer_key, name) VALUES (?, ?, 'Zeynep Arslan') RETURNING id`,
		waID, waID[2:]).Scan(&contact).Error)
	must(db.Raw(`INSERT INTO wa_conversations (channel_id, contact_id, last_message_at) VALUES (?, ?, ?) RETURNING id`, channel, contact, day).Scan(&conversation).Error)
	must(db.Raw(`INSERT INTO wa_tickets (conversation_id, channel_id, contact_id, status, resolved_at, rating, rating_comment, rated_at)
		VALUES (?, ?, ?, 'resolved', ?, 4, 'Kargo hızlı geldi', ?) RETURNING id`, conversation, channel, contact, day, day).Scan(&ticket).Error)
	t.Cleanup(func() {
		db.Exec("UPDATE wa_conversations SET ticket_id = NULL WHERE id = ?", conversation)
		db.Exec("DELETE FROM wa_tickets WHERE id = ?", ticket)
		db.Exec("DELETE FROM wa_conversations WHERE id = ?", conversation)
		db.Exec("DELETE FROM wa_contacts WHERE id = ?", contact)
		db.Exec("DELETE FROM wa_channels WHERE id = ?", channel)
		db.Exec("DELETE FROM wa_rating_links WHERE created_by = ?", sharer.ID)
		db.Exec("DELETE FROM audit_log WHERE action IN ? AND actor_id = ?", []string{enums.AuditWARatingLinkCreated, enums.AuditWARatingLinkRevoked}, sharer.ID)
	})

	vb, sb := signInAs(t, srv, viewer), signInAs(t, srv, sharer)
	newLink := map[string]any{"label": "Bölge müdürü", "hours": 24}
	if a := vb.do(fiber.MethodPost, "/api/v1/wa/rating-links", newLink); a.status != fiber.StatusForbidden {
		t.Fatalf("someone without whatsapp.rating_link made a link: %d", a.status)
	}
	if a := sb.do(fiber.MethodPost, "/api/v1/wa/rating-links", map[string]any{"label": "x", "hours": 24}); a.status != fiber.StatusBadRequest {
		t.Errorf("a link without a label answered %d, want 400", a.status)
	}
	if a := sb.do(fiber.MethodPost, "/api/v1/wa/rating-links", map[string]any{"label": "Uzun süre", "hours": 31 * 24}); a.status != fiber.StatusBadRequest {
		t.Errorf("a link for 31 days answered %d, want 400", a.status)
	}
	a := sb.do(fiber.MethodPost, "/api/v1/wa/rating-links", newLink)
	if a.status != fiber.StatusOK {
		t.Fatalf("making a link answered %d %s", a.status, a.body)
	}
	var link struct {
		ID     uint   `json:"id"`
		Token  string `json:"token"`
		Active bool   `json:"active"`
	}
	a.json(t, &link)
	if link.Token == "" || !link.Active {
		t.Fatalf("new link %+v carries no token or does not work", link)
	}

	type shared struct {
		Count int64 `json:"count"`
		Items []struct {
			Customer       string `json:"customer"`
			Phone          string `json:"phone"`
			Comment        string `json:"comment"`
			ConversationID *uint  `json:"conversationId"`
			TicketNumber   *int64 `json:"ticketNumber"`
		} `json:"items"`
		Channels []struct {
			ID   uint   `json:"id"`
			Name string `json:"name"`
		} `json:"channels"`
		Label string `json:"label"`
	}
	open := func(token, query string) (int, shared, http.Header) {
		t.Helper()
		path := "/api/v1/wa/shared-ratings/" + token + "?from=2025-04-10&to=2025-04-10&channel=" + strconv.FormatUint(uint64(channel), 10) + query
		res, body := raw(t, srv.app, fiber.MethodGet, path, "", nil, nil)
		var v shared
		if res.StatusCode == fiber.StatusOK {
			if err := json.Unmarshal(body, &v); err != nil {
				t.Fatalf("shared ratings: %v %s", err, body)
			}
		}
		return res.StatusCode, v, res.Header
	}

	// Opened without any session, a customer cannot be told apart.
	status, v, head := open(link.Token, "&first=1")
	if status != fiber.StatusOK || v.Count != 1 || len(v.Items) != 1 {
		t.Fatalf("opening the link: %d, %d scores", status, v.Count)
	}
	it := v.Items[0]
	if it.Customer != "Zeynep A." || it.Phone != "•••• "+waID[len(waID)-4:] || it.ConversationID != nil || it.TicketNumber != nil {
		t.Errorf("a customer shows as %q %q, conversation %v, ticket %v; want the name and number cut short and no way into the chat",
			it.Customer, it.Phone, it.ConversationID, it.TicketNumber)
	}
	if it.Comment != "Kargo hızlı geldi" || v.Label != "Bölge müdürü" {
		t.Errorf("comment %q, label %q", it.Comment, v.Label)
	}
	if len(v.Channels) == 0 {
		t.Error("the shared page gets no devices to filter by")
	}
	if head.Get("Cache-Control") != "no-store" || !strings.Contains(head.Get("X-Robots-Tag"), "noindex") {
		t.Errorf("the answer may be cached or indexed: %q %q", head.Get("Cache-Control"), head.Get("X-Robots-Tag"))
	}
	// The search reads comments, never names or numbers.
	if _, v, _ := open(link.Token, "&q=kargo"); v.Count != 1 {
		t.Errorf("searching a word of the comment found %d, want 1", v.Count)
	}
	if _, v, _ := open(link.Token, "&q="+waID[len(waID)-7:]); v.Count != 0 {
		t.Errorf("searching the customer's number found %d, want nothing", v.Count)
	}
	if _, v, _ := open(link.Token, "&q=Arslan"); v.Count != 0 {
		t.Errorf("searching the customer's surname found %d, want nothing", v.Count)
	}

	// One character changed, or another link's id, opens nothing.
	last := link.Token[len(link.Token)-1]
	other := byte('0')
	if last == '0' {
		other = '1'
	}
	if status, _, _ := open(link.Token[:len(link.Token)-1]+string(other), ""); status != fiber.StatusNotFound {
		t.Errorf("an altered link answered %d, want 404", status)
	}
	_, sig, _ := strings.Cut(link.Token, ".")
	if status, _, _ := open(strconv.FormatUint(uint64(link.ID+1), 10)+"."+sig, ""); status != fiber.StatusNotFound {
		t.Errorf("the signature on another id answered %d, want 404", status)
	}

	// The list counts the visit and still offers the link.
	var links []struct {
		ID        uint   `json:"id"`
		Token     string `json:"token"`
		OpenCount int    `json:"openCount"`
	}
	sb.do(fiber.MethodGet, "/api/v1/wa/rating-links", nil).json(t, &links)
	found := false
	for _, l := range links {
		if l.ID == link.ID {
			found = true
			if l.OpenCount != 1 || l.Token != link.Token {
				t.Errorf("listed link: opened %d times, token kept %v; want 1 and the same token", l.OpenCount, l.Token == link.Token)
			}
		}
	}
	if !found {
		t.Fatal("the new link is not listed")
	}

	// The maker can no longer see the ratings: the link stops too.
	db.Exec(`DELETE FROM role_permissions WHERE role_id = ? AND permission_id = (SELECT id FROM permissions WHERE key = ?)`, sharerRole.ID, string(enums.WARatings))
	srv.actors.ForgetAll()
	if status, _, _ := open(link.Token, ""); status != fiber.StatusNotFound {
		t.Errorf("a link whose maker lost whatsapp.ratings answered %d, want 404", status)
	}
	db.Exec(`INSERT INTO role_permissions (role_id, permission_id) SELECT ?, id FROM permissions WHERE key = ?`, sharerRole.ID, string(enums.WARatings))
	srv.actors.ForgetAll()
	if status, _, _ := open(link.Token, ""); status != fiber.StatusOK {
		t.Fatalf("with the permission back the link answered %d", status)
	}

	// Cancelled, it stops at once; cancelling again finds nothing.
	revoke := fmt.Sprintf("/api/v1/wa/rating-links/%d", link.ID)
	if a := vb.do(fiber.MethodDelete, revoke, nil); a.status != fiber.StatusForbidden {
		t.Errorf("someone without whatsapp.rating_link cancelled a link: %d", a.status)
	}
	if a := sb.do(fiber.MethodDelete, revoke, nil); a.status >= 300 {
		t.Fatalf("cancelling answered %d %s", a.status, a.body)
	}
	if status, _, _ := open(link.Token, ""); status != fiber.StatusNotFound {
		t.Errorf("a cancelled link answered %d, want 404", status)
	}
	if a := sb.do(fiber.MethodDelete, revoke, nil); a.status != fiber.StatusNotFound {
		t.Errorf("cancelling again answered %d, want 404", a.status)
	}

	var audited int64
	db.Raw("SELECT count(*) FROM audit_log WHERE action IN ? AND actor_id = ?",
		[]string{enums.AuditWARatingLinkCreated, enums.AuditWARatingLinkRevoked}, sharer.ID).Scan(&audited)
	if audited != 2 {
		t.Errorf("audit trail holds %d link entries, want 2", audited)
	}
}
