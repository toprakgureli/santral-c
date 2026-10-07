package main

import (
	"fmt"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/toprakgureli/santral-c/backend/pkg/enums"
)

// TestCallCheckWaitsForTheFinalRecord: the phone system's record of a call is
// copied every half minute, also while the call is still going on, when it
// shows no answer and no talk time yet. A ten-minute conversation checked
// against such a copy must not turn into an unanswered call of 0 seconds; it
// waits until the record is copied after the end, then takes the phone
// system's own length. A record copied after the end that really shows no
// answer (the phone system played an announcement) still wins.
func TestCallCheckWaitsForTheFinalRecord(t *testing.T) {
	srv, db := testServer(t)
	agent := person(t, db, "Uzun görüşen", true, customRole(t, db, enums.CallOriginate, enums.CallViewOwn))
	b := signInAs(t, srv, agent)
	run := time.Now().UnixNano()

	// call places a call that lasted the given seconds, ending now, and
	// returns when it started.
	call := func(callID, number string, seconds int) time.Time {
		t.Helper()
		post := func(phase map[string]any) {
			if a := b.do(fiber.MethodPost, "/api/v1/calls/log/", phase); a.status >= 300 {
				t.Fatalf("call log %v answered %d %s", phase["phase"], a.status, a.body)
			}
		}
		post(map[string]any{"callId": callID, "phase": "start", "direction": "outbound", "peer": number})
		post(map[string]any{"callId": callID, "phase": "answer"})
		started := time.Now().Add(-time.Duration(seconds+10) * time.Second)
		db.Exec("UPDATE call_logs SET started_at = ?, answered_at = ? WHERE call_id = ?", started, started.Add(10*time.Second), callID)
		post(map[string]any{"callId": callID, "phase": "end", "disposition": "answered", "durationSeconds": seconds})
		return started
	}
	record := func(uuid, number string, at, copied time.Time, talk, answerStamp string) {
		t.Helper()
		if err := db.Exec(`INSERT INTO pbx_cdrs (call_uuid, start_at, start_stamp, direction, caller_id_number, destination_number,
			caller_num, dest_num, caller_ext, dest_ext, duration, talk_duration, answer_stamp, fetched_at)
			VALUES (?, ?, ?, 'outbound', ?, ?, '', ?, ?, '', ?, ?, ?, ?)
			ON CONFLICT (call_uuid) DO UPDATE SET talk_duration = excluded.talk_duration, duration = excluded.duration,
				answer_stamp = excluded.answer_stamp, fetched_at = excluded.fetched_at`,
			uuid, at, at.Format(time.DateTime), *agent.SIPExtension, number, number, *agent.SIPExtension,
			talk, talk, answerStamp, copied).Error; err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Exec("DELETE FROM pbx_cdrs WHERE call_uuid = ?", uuid) })
	}
	state := func(callID string) (disposition string, seconds int, done bool) {
		t.Helper()
		var row struct {
			Disposition     string
			DurationSeconds int
			HooksDone       bool
		}
		if err := db.Raw("SELECT disposition, duration_seconds, hooks_done FROM call_logs WHERE call_id = ?", callID).Scan(&row).Error; err != nil {
			t.Fatal(err)
		}
		return row.Disposition, row.DurationSeconds, row.HooksDone
	}
	due := func(callID string) {
		// Time passes: the next look is due now.
		db.Exec("UPDATE call_logs SET hooks_next_at = now() WHERE call_id = ?", callID)
	}
	t.Cleanup(func() { db.Exec("DELETE FROM call_logs WHERE user_id = ?", agent.ID) })

	// A ten-minute call, whose record was copied while it was still ringing.
	long, longNumber := fmt.Sprintf("verify-long-%d", run), fmt.Sprintf("0544%07d", run%10_000_000)
	started := time.Now().Add(-610 * time.Second)
	record("cdr-"+long, longNumber, started, time.Now().Add(-time.Minute), "00:00:00", "")
	call(long, longNumber, 600)
	srv.callLog.VerifyPending(t.Context())
	if d, s, done := state(long); d != "answered" || s != 600 || done {
		t.Fatalf("checked against a copy taken during the call: %s, %d s, done %v; want answered, 600 s, still waiting", d, s, done)
	}

	// The record is copied again after the end, with the real figures.
	record("cdr-"+long, longNumber, started, time.Now().Add(time.Second), "00:08:27", started.Add(10*time.Second).Format("2006-01-02 15:04:05 -0700"))
	due(long)
	srv.callLog.VerifyPending(t.Context())
	if d, s, done := state(long); d != "answered" || s != 507 || !done {
		t.Fatalf("checked against the final record: %s, %d s, done %v; want answered, 507 s, done", d, s, done)
	}

	// A call the phone system never connected (an announcement played): its
	// record, copied after the end, shows no answer, and that stands.
	short, shortNumber := fmt.Sprintf("verify-ann-%d", run), fmt.Sprintf("0533%07d", run%10_000_000)
	at := call(short, shortNumber, 6)
	record("cdr-"+short, shortNumber, at, time.Now().Add(time.Second), "00:00:00", "")
	srv.callLog.VerifyPending(t.Context())
	if d, s, done := state(short); d != "no_answer" || s != 0 || !done {
		t.Fatalf("announcement call: %s, %d s, done %v; want no_answer, 0 s, done", d, s, done)
	}
}
