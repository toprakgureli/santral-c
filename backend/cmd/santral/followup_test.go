package main

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
	"github.com/toprakgureli/santral-c/backend/pkg/enums"
)

// TestUnreachedClosesWhenAnyoneReachesTheCustomer: a call out that does not
// get through puts the number on its caller's list and counts each try; a
// colleague on a call with that number sees it; "I am calling back" holds a
// row for one person; once the customer calls in and talks to someone else,
// the row closes by itself and whoever could not reach them is told who did.
// A row can also be closed by hand, and only call permissions open the list.
func TestUnreachedClosesWhenAnyoneReachesTheCustomer(t *testing.T) {
	srv, db := testServer(t)
	agentRole := customRole(t, db, enums.CallOriginate, enums.CallViewOwn)
	a := person(t, db, "Ulaşamayan", true, agentRole)
	b := person(t, db, "Ulaşan", true, agentRole)
	lead := person(t, db, "Takım lideri", false, customRole(t, db, enums.CallViewAll))
	outsider := person(t, db, "Çağrısı olmayan", false, customRole(t, db, enums.ContactView))
	ab, bb, lb, ob := signInAs(t, srv, a), signInAs(t, srv, b), signInAs(t, srv, lead), signInAs(t, srv, outsider)

	run := time.Now().UnixNano()
	number := fmt.Sprintf("0544%07d", run%10_000_000)
	other := fmt.Sprintf("0533%07d", run%10_000_000)
	t.Cleanup(func() {
		for _, u := range []models.User{a, b} {
			db.Exec("DELETE FROM call_logs WHERE user_id = ?", u.ID)
			db.Exec("DELETE FROM call_unreached WHERE user_id = ?", u.ID)
			db.Exec("DELETE FROM user_notices WHERE user_id = ?", u.ID)
		}
		db.Exec("DELETE FROM pbx_cdrs WHERE call_uuid LIKE ?", fmt.Sprintf("fu-%d-%%", run))
	})

	// call places a call that lasted the given seconds and ended now, with
	// the phone system's record of it, and runs the call checker.
	n := 0
	call := func(br *browser, u models.User, direction, peer, disposition string, seconds int) {
		t.Helper()
		n++
		id := fmt.Sprintf("fu-%d-%d", run, n)
		post := func(phase map[string]any) {
			if r := br.do(fiber.MethodPost, "/api/v1/calls/log/", phase); r.status >= 300 {
				t.Fatalf("call log %v answered %d %s", phase["phase"], r.status, r.body)
			}
		}
		post(map[string]any{"callId": id, "phase": "start", "direction": direction, "peer": peer})
		started := time.Now().Add(-time.Duration(seconds+10) * time.Second)
		answer := ""
		if disposition == "answered" {
			post(map[string]any{"callId": id, "phase": "answer"})
			answer = started.Add(10 * time.Second).Format("2006-01-02 15:04:05 -0700")
		}
		db.Exec("UPDATE call_logs SET started_at = ? WHERE call_id = ?", started, id)
		post(map[string]any{"callId": id, "phase": "end", "disposition": disposition, "durationSeconds": seconds})
		talk := fmt.Sprintf("00:%02d:%02d", seconds/60, seconds%60)
		callerExt, destExt, callerNum, destNum := *u.SIPExtension, "", "", peer
		if direction == "inbound" {
			callerExt, destExt, callerNum, destNum = "", *u.SIPExtension, peer, ""
		}
		if err := db.Exec(`INSERT INTO pbx_cdrs (call_uuid, start_at, start_stamp, direction, caller_id_number, destination_number,
			caller_num, dest_num, caller_ext, dest_ext, duration, talk_duration, answer_stamp)
			VALUES (?, ?, ?, ?, '', '', ?, ?, ?, ?, ?, ?, ?)`,
			"cdr-"+id, started, started.Format(time.DateTime), direction, callerNum, destNum, callerExt, destExt, talk, talk, answer).Error; err != nil {
			t.Fatal(err)
		}
		srv.callLog.VerifyPending(t.Context())
	}
	type row struct {
		ID       uint   `json:"id"`
		Number   string `json:"number"`
		Mine     bool   `json:"mine"`
		Attempts int    `json:"attempts"`
		Reason   string `json:"reason"`
		Status   string `json:"status"`
		User     struct {
			ID uint `json:"id"`
		} `json:"user"`
		Reached *struct {
			By struct {
				ID uint `json:"id"`
			} `json:"by"`
			Direction string `json:"direction"`
			Seconds   int    `json:"seconds"`
		} `json:"reached"`
		Claim *struct {
			By struct {
				ID uint `json:"id"`
			} `json:"by"`
		} `json:"claim"`
	}
	list := func(br *browser, query string) []row {
		t.Helper()
		r := br.do(fiber.MethodGet, "/api/v1/followups/unreached"+query, nil)
		if r.status != fiber.StatusOK {
			t.Fatalf("list %s answered %d %s", query, r.status, r.body)
		}
		var out []row
		r.json(t, &out)
		return out
	}
	find := func(rows []row, number string, user uint) *row {
		for i := range rows {
			if rows[i].Number == number && rows[i].User.ID == user {
				return &rows[i]
			}
		}
		return nil
	}

	// Two tries that do not get through make one row with two attempts.
	call(ab, a, "outbound", number, "no_answer", 0)
	call(ab, a, "outbound", number, "busy", 0)
	// What follows a call runs beside the request.
	waitFor(t, 3*time.Second, "the second try is counted", func() bool {
		r := find(list(ab, ""), number, a.ID)
		return r != nil && r.Attempts == 2
	})
	mine := find(list(ab, ""), number, a.ID)
	if mine == nil || mine.Attempts != 2 || mine.Reason != "busy" || !mine.Mine {
		t.Fatalf("after two tries the list shows %+v, want one row of 2 attempts, last busy", mine)
	}
	if find(list(bb, ""), number, a.ID) != nil {
		t.Error("a colleague without call.view_all sees someone else's row in their list")
	}
	if r := bb.do(fiber.MethodGet, "/api/v1/followups/unreached?scope=all", nil); r.status != fiber.StatusForbidden {
		t.Errorf("everyone's list without call.view_all answered %d, want 403", r.status)
	}
	if r := ob.do(fiber.MethodGet, "/api/v1/followups/unreached", nil); r.status != fiber.StatusForbidden {
		t.Errorf("the list without any call permission answered %d, want 403", r.status)
	}

	// A colleague on a call with the number sees who could not reach it.
	var peer struct {
		InboundBefore int `json:"inboundBefore"`
		Unreached     []struct {
			User struct {
				ID uint `json:"id"`
			} `json:"user"`
			Attempts int `json:"attempts"`
		} `json:"unreached"`
	}
	bb.do(fiber.MethodGet, "/api/v1/followups/peer?number="+number, nil).json(t, &peer)
	if len(peer.Unreached) != 1 || peer.Unreached[0].User.ID != a.ID || peer.Unreached[0].Attempts != 2 {
		t.Errorf("during a call the colleague sees %+v, want the caller's 2 attempts", peer.Unreached)
	}

	// The lead says they are calling back; nobody else can at the same time.
	if r := lb.do(fiber.MethodPost, fmt.Sprintf("/api/v1/followups/unreached/%d/claim", mine.ID), nil); r.status >= 300 {
		t.Fatalf("the lead's claim answered %d %s", r.status, r.body)
	}
	if r := ab.do(fiber.MethodPost, fmt.Sprintf("/api/v1/followups/unreached/%d/claim", mine.ID), nil); r.status != fiber.StatusConflict {
		t.Errorf("a second claim answered %d, want 409", r.status)
	}
	if r := bb.do(fiber.MethodPost, fmt.Sprintf("/api/v1/followups/unreached/%d/drop", mine.ID), nil); r.status != fiber.StatusNotFound {
		t.Errorf("a colleague closed someone else's row: %d", r.status)
	}
	if got := find(list(lb, "?scope=all"), number, a.ID); got == nil || got.Claim == nil || got.Claim.By.ID != lead.ID {
		t.Errorf("everyone's list shows %+v, want the lead's claim on it", got)
	}

	// The customer calls in and talks to the colleague for five minutes.
	call(bb, b, "inbound", number, "answered", 300)
	waitFor(t, 3*time.Second, "the row closes once the customer was reached", func() bool {
		return find(list(ab, ""), number, a.ID) == nil
	})
	closed := find(list(ab, "?state=closed"), number, a.ID)
	if closed == nil || closed.Status != "reached" || closed.Reached == nil || closed.Reached.By.ID != b.ID ||
		closed.Reached.Direction != "inbound" || closed.Reached.Seconds != 300 {
		t.Fatalf("closed row %+v, want reached by the colleague, inbound, 300 s", closed)
	}
	var notes struct {
		Items []struct {
			ID   uint   `json:"id"`
			Kind string `json:"kind"`
			Text string `json:"text"`
		} `json:"items"`
		Unread int `json:"unread"`
	}
	ab.do(fiber.MethodGet, "/api/v1/notices", nil).json(t, &notes)
	if notes.Unread != 1 || len(notes.Items) != 1 || !strings.Contains(notes.Items[0].Text, b.Name) || !strings.Contains(notes.Items[0].Text, "5 dk") {
		t.Fatalf("the caller's notices: %+v, want one telling the colleague talked 5 dk", notes)
	}
	var bnotes struct {
		Unread int `json:"unread"`
	}
	bb.do(fiber.MethodGet, "/api/v1/notices", nil).json(t, &bnotes)
	if bnotes.Unread != 0 {
		t.Errorf("the one who reached the customer got %d notices, want none", bnotes.Unread)
	}
	if r := ab.do(fiber.MethodPost, "/api/v1/notices/read", map[string]any{"all": true}); r.status >= 300 {
		t.Fatalf("marking read answered %d", r.status)
	}
	ab.do(fiber.MethodGet, "/api/v1/notices", nil).json(t, &notes)
	if notes.Unread != 0 {
		t.Errorf("after reading, %d unread", notes.Unread)
	}

	// What a later call with the number shows: today's earlier call and the
	// last conversation.
	var later struct {
		InboundBefore int `json:"inboundBefore"`
		LastTalk      *struct {
			By struct {
				ID uint `json:"id"`
			} `json:"by"`
			Seconds int `json:"seconds"`
		} `json:"lastTalk"`
		Unreached []any `json:"unreached"`
	}
	ab.do(fiber.MethodGet, "/api/v1/followups/peer?number="+number, nil).json(t, &later)
	if later.InboundBefore != 1 || later.LastTalk == nil || later.LastTalk.By.ID != b.ID || later.LastTalk.Seconds != 300 || len(later.Unreached) != 0 {
		t.Errorf("a later call shows %+v, want 1 earlier call today and the colleague's 300 s", later)
	}

	// No call back needed: closed by hand.
	call(ab, a, "outbound", other, "no_answer", 0)
	waitFor(t, 3*time.Second, "the second number is listed", func() bool {
		return find(list(ab, ""), other, a.ID) != nil
	})
	row2 := find(list(ab, ""), other, a.ID)
	if row2 == nil {
		t.Fatal("the second number is not on the list")
	}
	if r := ab.do(fiber.MethodPost, fmt.Sprintf("/api/v1/followups/unreached/%d/drop", row2.ID), nil); r.status >= 300 {
		t.Fatalf("closing by hand answered %d %s", r.status, r.body)
	}
	if got := find(list(ab, "?state=closed"), other, a.ID); got == nil || got.Status != "dropped" {
		t.Errorf("closed by hand: %+v", got)
	}
}
