package main

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/toprakgureli/santral-c/backend/pkg/enums"
)

// TestLiveAlertsForTheLead: only performance.live_alerts sees them; a
// conversation running past twenty minutes, someone on break past the
// day's limit, incoming calls left unanswered in a short time, a number
// nobody called back for two hours and a planned call back past its time
// each raise one alert.
func TestLiveAlertsForTheLead(t *testing.T) {
	srv, db := testServer(t)
	agentRole := customRole(t, db, enums.CallOriginate, enums.CallViewOwn)
	talker := person(t, db, "Uzun konuşan", true, agentRole)
	rester := person(t, db, "Molada kalan", true, agentRole)
	lead := person(t, db, "Canlı izleyen", false, customRole(t, db, enums.PerformanceLiveAlerts))
	plain := person(t, db, "İzleyemeyen", false, customRole(t, db, enums.PerformanceViewRole))
	lb, pb := signInAs(t, srv, lead), signInAs(t, srv, plain)

	var limit []string
	db.Raw("SELECT value FROM system_settings WHERE key = 'break_limit_minutes'").Scan(&limit)
	stamp := time.Now().UnixNano()
	t.Cleanup(func() {
		db.Exec("DELETE FROM system_settings WHERE key = 'break_limit_minutes'")
		if len(limit) > 0 {
			db.Exec("INSERT INTO system_settings (key, value, updated_at) VALUES ('break_limit_minutes', ?, now())", limit[0])
		}
		db.Exec("DELETE FROM call_logs WHERE user_id = ?", talker.ID)
		db.Exec("DELETE FROM agent_presence WHERE user_id = ?", rester.ID)
		db.Exec("DELETE FROM presence_events WHERE user_id = ?", rester.ID)
		db.Exec("DELETE FROM pbx_cdrs WHERE call_uuid LIKE ?", fmt.Sprintf("la-%d-%%", stamp))
		db.Exec("DELETE FROM call_unreached WHERE user_id = ?", talker.ID)
		db.Exec("DELETE FROM call_reminders WHERE user_id = ?", talker.ID)
	})

	if r := pb.do(fiber.MethodGet, "/api/v1/performance/alerts", nil); r.status != fiber.StatusForbidden {
		t.Fatalf("someone without performance.live_alerts read them: %d", r.status)
	}

	now := time.Now()
	db.Exec(`INSERT INTO call_logs (call_id, user_id, direction, peer_number, peer_key, disposition, started_at, answered_at, hooks_done)
		VALUES (?, ?, 'inbound', '05550009911', '5550009911', 'in_progress', ?, ?, true)`,
		fmt.Sprintf("la-%d-call", stamp), talker.ID, now.Add(-26*time.Minute), now.Add(-25*time.Minute))
	db.Exec("DELETE FROM system_settings WHERE key = 'break_limit_minutes'")
	db.Exec("INSERT INTO system_settings (key, value, updated_at) VALUES ('break_limit_minutes', '5', now())")
	db.Exec(`INSERT INTO agent_presence (user_id, state, updated_at) VALUES (?, 'break', ?)
		ON CONFLICT (user_id) DO UPDATE SET state = 'break', updated_at = excluded.updated_at`, rester.ID, now.Add(-3*time.Minute))
	db.Exec(`INSERT INTO presence_events (user_id, state, started_at) VALUES (?, 'break', ?)`, rester.ID, now.Add(-3*time.Minute))
	db.Exec(`INSERT INTO presence_events (user_id, state, started_at, ended_at) VALUES (?, 'break', ?, ?)`, rester.ID, now.Add(-2*time.Hour), now.Add(-2*time.Hour+10*time.Minute))
	for i := range 3 {
		db.Exec(`INSERT INTO pbx_cdrs (call_uuid, start_at, start_stamp, direction, caller_num, dest_num, caller_ext, dest_ext, answer_stamp)
			VALUES (?, ?, '', 'inbound', '05550008800', '', '', '', '')`, fmt.Sprintf("la-%d-%d", stamp, i), now.Add(-time.Duration(2+i)*time.Minute))
	}
	db.Exec(`INSERT INTO call_unreached (peer_key, peer_number, user_id, first_at, last_at, status) VALUES ('5550007700', '05550007700', ?, ?, ?, 'open')`,
		talker.ID, now.Add(-3*time.Hour), now.Add(-3*time.Hour))
	db.Exec(`INSERT INTO call_reminders (user_id, peer_number, peer_key, due_at) VALUES (?, '05550006600', '5550006600', ?)`, talker.ID, now.Add(-time.Hour))

	var alerts []struct {
		Key  string `json:"key"`
		Kind string `json:"kind"`
		Text string `json:"text"`
	}
	r := lb.do(fiber.MethodGet, "/api/v1/performance/alerts", nil)
	if r.status != fiber.StatusOK {
		t.Fatalf("alerts answered %d %s", r.status, r.body)
	}
	r.json(t, &alerts)
	found := map[string]string{}
	for _, a := range alerts {
		if _, seen := found[a.Kind]; !seen || strings.Contains(a.Text, talker.Name) || strings.Contains(a.Text, rester.Name) {
			found[a.Kind] = a.Text
		}
	}
	checks := []struct{ kind, needs string }{
		{"long_call", talker.Name},
		{"break_over", rester.Name},
		{"missed", "cevaplanmadı"},
		{"unreached", "geri dönmedi"},
		{"reminder_late", talker.Name},
	}
	for _, c := range checks {
		if !strings.Contains(found[c.kind], c.needs) {
			t.Errorf("no %s alert naming %q; got %+v", c.kind, c.needs, alerts)
		}
	}
}
