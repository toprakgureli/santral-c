package main

import (
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/toprakgureli/santral-c/backend/pkg/enums"
	"github.com/toprakgureli/santral-c/backend/pkg/tz"
)

// TestRealCallThresholdIsOneSettingEverywhere: how long an answered call
// must last to count as real is a setting only call.real_seconds may change;
// the call history, the team page and the profile all count with it at once.
func TestRealCallThresholdIsOneSettingEverywhere(t *testing.T) {
	srv, db := testServer(t)
	agent := person(t, db, "Çağrı sayan", true, customRole(t, db, enums.CallViewOwn))
	boss := person(t, db, "Eşiği koyan", false, customRole(t, db, enums.CallRealSeconds, enums.PerformanceViewAll))
	plain := person(t, db, "Eşiğe dokunamayan", false, customRole(t, db, enums.PerformanceViewAll))

	var before []string
	db.Raw("SELECT value FROM system_settings WHERE key = 'real_call_seconds'").Scan(&before)
	t.Cleanup(func() {
		if len(before) == 0 {
			db.Exec("DELETE FROM system_settings WHERE key = 'real_call_seconds'")
		} else {
			db.Exec("UPDATE system_settings SET value = ? WHERE key = 'real_call_seconds'", before[0])
		}
	})
	db.Exec("DELETE FROM system_settings WHERE key = 'real_call_seconds'")

	stamp := time.Now().UnixNano()
	start := time.Now().Add(-time.Hour)
	for i, seconds := range []int{40, 70} {
		if err := db.Exec(`INSERT INTO call_logs (call_id, user_id, direction, peer_number, peer_key, disposition, started_at, answered_at, ended_at, duration_seconds, hooks_done)
			VALUES (?, ?, 'outbound', '05550001144', '5550001144', 'answered', ?, ?, ?, ?, true)`,
			fmt.Sprintf("real-%d-%d", stamp, i), agent.ID, start, start.Add(5*time.Second), start.Add(time.Duration(5+seconds)*time.Second), seconds).Error; err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { db.Exec("DELETE FROM call_logs WHERE user_id = ?", agent.ID) })

	ab, bb, pb := signInAs(t, srv, agent), signInAs(t, srv, boss), signInAs(t, srv, plain)
	today := time.Now().In(tz.Istanbul).Format("2006-01-02")
	id := strconv.FormatUint(uint64(agent.ID), 10)

	counts := func(stage string, want int64) {
		t.Helper()
		var history struct {
			Long int64 `json:"long"`
		}
		ab.do(fiber.MethodGet, "/api/v1/calls/log", nil).json(t, &history)
		var team struct {
			Items []struct {
				UserID uint `json:"userId"`
				Calls  struct {
					Long int64 `json:"long"`
				} `json:"calls"`
			} `json:"items"`
		}
		bb.do(fiber.MethodGet, "/api/v1/performance/today", nil).json(t, &team)
		teamLong := int64(-1)
		for _, it := range team.Items {
			if it.UserID == agent.ID {
				teamLong = it.Calls.Long
			}
		}
		var record struct {
			Real int64 `json:"real"`
		}
		bb.do(fiber.MethodGet, "/api/v1/profile/"+id+"/record?from="+today+"&to="+today, nil).json(t, &record)
		if history.Long != want || teamLong != want || record.Real != want {
			t.Errorf("%s: real calls in history %d, team %d, profile %d; want %d everywhere", stage, history.Long, teamLong, record.Real, want)
		}
	}
	counts("default 30 seconds", 2)

	if a := pb.do(fiber.MethodPut, "/api/v1/settings/real-call", map[string]any{"seconds": 60}); a.status != fiber.StatusForbidden {
		t.Fatalf("someone without call.real_seconds changed the threshold: %d", a.status)
	}
	if a := bb.do(fiber.MethodPut, "/api/v1/settings/real-call", map[string]any{"seconds": 3}); a.status != fiber.StatusBadRequest {
		t.Errorf("a threshold of 3 seconds answered %d, want 400", a.status)
	}
	if a := bb.do(fiber.MethodPut, "/api/v1/settings/real-call", map[string]any{"seconds": 60}); a.status != fiber.StatusOK {
		t.Fatalf("setting 60 seconds answered %d %s", a.status, a.body)
	}
	var read struct {
		Seconds int `json:"seconds"`
	}
	ab.do(fiber.MethodGet, "/api/v1/settings/real-call", nil).json(t, &read)
	if read.Seconds != 60 {
		t.Errorf("an agent reads %d seconds, want 60", read.Seconds)
	}
	counts("60 seconds", 1)
}
