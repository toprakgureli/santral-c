package main

import (
	"fmt"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/toprakgureli/santral-c/backend/pkg/enums"
	"github.com/toprakgureli/santral-c/backend/pkg/tz"
)

// TestWorkdayEndAndDaySummary: only agent.workday sets when the working day
// ends and each role's daily target; the shift then reminds at that time,
// brings the summary five minutes before and closes fifty minutes after;
// the day's summary counts today's real calls against the person's target
// and their own last week, with the day's follow-ups.
func TestWorkdayEndAndDaySummary(t *testing.T) {
	srv, db := testServer(t)
	agentRole := customRole(t, db, enums.CallOriginate, enums.CallViewOwn)
	agent := person(t, db, "Hedefli", true, agentRole)
	boss := person(t, db, "Mesaiyi ayarlayan", false, customRole(t, db, enums.AgentWorkday))
	plain := person(t, db, "Ayarlayamayan", false, customRole(t, db, enums.CallViewOwn))
	ab, bb, pb := signInAs(t, srv, agent), signInAs(t, srv, boss), signInAs(t, srv, plain)

	var before []struct{ Key, Value string }
	db.Raw("SELECT key, value FROM system_settings WHERE key = 'shift_end' OR key LIKE 'daily_target_%'").Scan(&before)
	t.Cleanup(func() {
		db.Exec("DELETE FROM system_settings WHERE key = 'shift_end' OR key LIKE 'daily_target_%'")
		for _, r := range before {
			db.Exec("INSERT INTO system_settings (key, value, updated_at) VALUES (?, ?, now())", r.Key, r.Value)
		}
		db.Exec("DELETE FROM call_logs WHERE user_id = ?", agent.ID)
		db.Exec("DELETE FROM call_unreached WHERE user_id = ?", agent.ID)
		db.Exec("DELETE FROM call_reminders WHERE user_id = ?", agent.ID)
		db.Exec("DELETE FROM shifts WHERE user_id = ?", agent.ID)
	})

	set := func(br *browser, body map[string]any) int {
		t.Helper()
		return br.do(fiber.MethodPut, "/api/v1/settings/workday", body).status
	}
	good := map[string]any{"shiftEnd": "17:30", "targets": map[string]int{fmt.Sprint(agentRole.ID): 25}}
	if st := set(pb, good); st != fiber.StatusForbidden {
		t.Fatalf("someone without agent.workday changed the working day: %d", st)
	}
	if st := set(bb, map[string]any{"shiftEnd": "09:00"}); st != fiber.StatusBadRequest {
		t.Errorf("a day ending at 09:00 answered %d, want 400", st)
	}
	if st := set(bb, map[string]any{"shiftEnd": "17:30", "targets": map[string]int{fmt.Sprint(agentRole.ID): 600}}); st != fiber.StatusBadRequest {
		t.Errorf("a target of 600 answered %d, want 400", st)
	}
	if st := set(bb, map[string]any{"shiftEnd": "17:30", "targets": map[string]int{"999999999": 10}}); st != fiber.StatusBadRequest {
		t.Errorf("a target for an unknown role answered %d, want 400", st)
	}
	if st := set(bb, good); st != fiber.StatusOK {
		t.Fatalf("setting the working day answered %d", st)
	}

	// The shift follows the set end.
	var status struct {
		ReminderAt time.Time `json:"reminderAt"`
		AutoEndAt  time.Time `json:"autoEndAt"`
		SummaryAt  time.Time `json:"summaryAt"`
	}
	ab.do(fiber.MethodGet, "/api/v1/shift/", nil).json(t, &status)
	clock := func(v time.Time) string { return v.In(tz.Istanbul).Format("15:04") }
	if clock(status.ReminderAt) != "17:30" || clock(status.SummaryAt) != "17:25" || clock(status.AutoEndAt) != "18:20" {
		t.Errorf("shift times %s / summary %s / auto end %s, want 17:30, 17:25 and 18:20",
			clock(status.ReminderAt), clock(status.SummaryAt), clock(status.AutoEndAt))
	}

	// Today: one real call and one too short; the week before: 3 and 1.
	now := time.Now().In(tz.Istanbul)
	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, tz.Istanbul)
	stamp := time.Now().UnixNano()
	n := 0
	logCall := func(at time.Time, seconds int) {
		t.Helper()
		n++
		if err := db.Exec(`INSERT INTO call_logs (call_id, user_id, direction, peer_number, peer_key, disposition, started_at, answered_at, ended_at, duration_seconds, hooks_done)
			VALUES (?, ?, 'outbound', '05550001122', '5550001122', 'answered', ?, ?, ?, ?, true)`,
			fmt.Sprintf("wd-%d-%d", stamp, n), agent.ID, at, at, at.Add(time.Duration(seconds)*time.Second), seconds).Error; err != nil {
			t.Fatal(err)
		}
	}
	today := midnight.Add(time.Minute)
	if now.Sub(midnight) > 2*time.Minute {
		today = now.Add(-time.Minute)
	}
	logCall(today, 70)
	logCall(today, 10)
	for i := range 3 {
		logCall(midnight.AddDate(0, 0, -2).Add(time.Duration(10+i)*time.Hour), 90)
	}
	logCall(midnight.AddDate(0, 0, -4).Add(11*time.Hour), 90)
	db.Exec(`INSERT INTO call_unreached (peer_key, peer_number, user_id, first_at, last_at, status) VALUES
		('5550003344', '05550003344', ?, now(), now(), 'open'), ('5550005566', '05550005566', ?, now(), now(), 'reached')`, agent.ID, agent.ID)
	db.Exec(`INSERT INTO call_reminders (user_id, peer_number, peer_key, due_at) VALUES (?, '05550007788', '5550007788', now() + interval '1 hour')`, agent.ID)

	var sum struct {
		Today struct {
			Real  int64 `json:"real"`
			Short int64 `json:"short"`
		} `json:"today"`
		Target      int     `json:"target"`
		WeekAverage float64 `json:"weekAverage"`
		WeekBest    int64   `json:"weekBest"`
		WeekDays    int     `json:"weekDays"`
		Followups   struct {
			Unreached     int64 `json:"unreached"`
			Reached       int64 `json:"reached"`
			RemindersOpen int64 `json:"remindersOpen"`
		} `json:"followups"`
	}
	r := ab.do(fiber.MethodGet, "/api/v1/profile/me/summary", nil)
	if r.status != fiber.StatusOK {
		t.Fatalf("the day's summary answered %d %s", r.status, r.body)
	}
	r.json(t, &sum)
	if sum.Today.Real != 1 || sum.Today.Short != 1 || sum.Target != 25 {
		t.Errorf("today %d real, %d short, target %d; want 1, 1 and 25", sum.Today.Real, sum.Today.Short, sum.Target)
	}
	if sum.WeekDays != 2 || sum.WeekAverage != 2 || sum.WeekBest != 3 {
		t.Errorf("week: %d days, average %v, best %d; want 2 days, 2 and 3", sum.WeekDays, sum.WeekAverage, sum.WeekBest)
	}
	if sum.Followups.Unreached != 2 || sum.Followups.Reached != 1 || sum.Followups.RemindersOpen != 1 {
		t.Errorf("follow-ups %+v, want 2 unreached, 1 reached since, 1 call back open", sum.Followups)
	}
}
