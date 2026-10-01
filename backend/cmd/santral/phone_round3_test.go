package main

// The phone system under pressure: do-not-disturb changes that the phone
// system refuses for a while, the evening close of every shift, a gap in the
// call-record copy, and calls whose end never arrived.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
	"github.com/toprakgureli/santral-c/backend/pkg/enums"
	"github.com/toprakgureli/santral-c/backend/pkg/tz"
)

// throttleDND makes the stand-in phone system refuse do-not-disturb changes
// beyond perWindow in each window, the way the real one answers 429 when
// its budget runs out. It returns the counts of accepted and refused ones.
func throttleDND(p *fakePBX, perWindow int, window time.Duration) (accepted, refused *atomic.Int64) {
	accepted, refused = &atomic.Int64{}, &atomic.Int64{}
	var mu sync.Mutex
	var start time.Time
	used := 0
	next := p.srv.Config.Handler
	p.srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/dnd/") {
			mu.Lock()
			if time.Since(start) > window {
				start, used = time.Now(), 0
			}
			used++
			over := used > perWindow
			mu.Unlock()
			if over {
				refused.Add(1)
				http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
				return
			}
			accepted.Add(1)
		}
		next.ServeHTTP(w, r)
	})
	return accepted, refused
}

// dndOf reads the stand-in's do-not-disturb state of an extension.
func (p *fakePBX) dndOf(ext string) string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.dnd[ext]
}

// runDND runs the do-not-disturb queue until the returned stop is called.
func runDND(srv *server) (stop func()) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		srv.phone.RunDND(ctx)
		close(done)
	}()
	return func() {
		cancel()
		<-done
	}
}

// waitFor polls cond until it holds or the time is up.
func waitFor(t *testing.T, within time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(within)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("not within %s: %s", within, what)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// TestLoadPhoneDNDThrottle: thirty agents start their shift in the same
// second, take a break and come back, against a phone system that takes
// only a few do-not-disturb changes at a time. No request may fail for it,
// the panel says when a change is still on its way, and in the end every
// extension's do-not-disturb matches the panel. At 19:20 the sweep closes
// every shift; the phone system hears about it in turn, not all at once.
func TestLoadPhoneDNDThrottle(t *testing.T) {
	loadTest(t)
	pbx := newFakePBX(t)
	accepted, refused := throttleDND(pbx, 4, 150*time.Millisecond)
	srv, db := testServer(t, officeSecurity, pbx.use)
	bigHistory(t, db)
	const pace = 15 * time.Millisecond
	srv.phone.TuneDND(pace, 60*time.Millisecond)
	stopDND := runDND(srv)

	agents := seedPeople(t, db, 30, enums.RoleSalesTeam, true)
	stats := newTally()
	browsers := make([]*browser, len(agents))
	for i, u := range agents {
		browsers[i] = newBrowser(t, srv.app, stats, u)
		browsers[i].mustSignIn()
	}

	var pendingSeen atomic.Int64
	began := time.Now()
	together(len(browsers), func(i int) {
		b := browsers[i]
		if a := b.do(fiber.MethodPost, "/api/v1/shift/start", nil); a.status != fiber.StatusOK {
			t.Errorf("%s: shift start answered %d %s", b.user.Email, a.status, a.body)
			return
		}
		for _, state := range []string{"break", "available"} {
			a := b.do(fiber.MethodPost, "/api/v1/pbx/status", map[string]string{"state": state})
			if a.status != fiber.StatusOK {
				t.Errorf("%s: status %s answered %d %s", b.user.Email, state, a.status, a.body)
				continue
			}
			var res struct {
				State      string `json:"state"`
				PBXPending bool   `json:"pbxPending"`
			}
			a.json(t, &res)
			if res.State != state {
				t.Errorf("%s: status answered state %q, want %q", b.user.Email, res.State, state)
			}
			if res.PBXPending {
				pendingSeen.Add(1)
			}
		}
	})
	if refused.Load() == 0 {
		t.Fatal("the phone system never pushed back; the test proves nothing")
	}
	if pendingSeen.Load() == 0 {
		t.Error("no agent was told that a change was still on its way")
	}

	waitFor(t, 20*time.Second, "every extension's do-not-disturb off", func() bool {
		for _, a := range agents {
			if pbx.dndOf(*a.SIPExtension) != "off" {
				return false
			}
		}
		return true
	})
	// Once confirmed, the panel stops saying "on its way".
	for _, b := range browsers {
		a := b.do(fiber.MethodGet, "/api/v1/pbx/status", nil)
		var p struct {
			State      string `json:"state"`
			PBXPending bool   `json:"pbxPending"`
		}
		a.json(t, &p)
		if p.State != "available" || p.PBXPending {
			t.Errorf("%s: status %q pending %v after the phone system caught up", b.user.Email, p.State, p.PBXPending)
		}
	}
	t.Logf("do-not-disturb: %d accepted, %d refused and retried", accepted.Load(), refused.Load())
	stats.report(t, "shift start under a throttled phone system", time.Since(began))

	// 19:20: the shifts started "yesterday" are past their cutoff.
	stopDND()
	if err := db.Exec("UPDATE shifts SET started_at = started_at - interval '1 day' WHERE user_id IN ? AND ended_at IS NULL", ids(agents)).Error; err != nil {
		t.Fatal(err)
	}
	before := accepted.Load() + refused.Load()
	srv.shifts.Sweep(t.Context())
	if n := accepted.Load() + refused.Load() - before; n != 0 {
		t.Errorf("the sweep sent %d do-not-disturb changes at once; they must wait their turn", n)
	}
	var open int64
	db.Model(&models.Shift{}).Where("user_id IN ? AND ended_at IS NULL", ids(agents)).Count(&open)
	if open != 0 {
		t.Fatalf("%d shifts still open after the sweep", open)
	}

	stopDND = runDND(srv)
	defer stopDND()
	waitFor(t, 20*time.Second, "every extension's do-not-disturb on after the sweep", func() bool {
		for _, a := range agents {
			if pbx.dndOf(*a.SIPExtension) != "on" {
				return false
			}
		}
		return true
	})
}

// cdrFeed is the stand-in phone system's call-record list: newest first,
// paged the way the real API pages it.
type cdrFeed struct {
	mu       sync.Mutex
	records  []cdrStub
	requests int
}

// cdrStub is one record the stand-in serves.
type cdrStub struct {
	UUID  string
	Start time.Time
}

func (f *cdrFeed) serve(p *fakePBX) {
	next := p.srv.Config.Handler
	p.srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/cdrs" {
			next.ServeHTTP(w, r)
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		f.requests++
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		if page < 1 {
			page = 1
		}
		if limit < 1 {
			limit = 100
		}
		recs := append([]cdrStub(nil), f.records...)
		sort.Slice(recs, func(i, j int) bool { return recs[i].Start.After(recs[j].Start) })
		from := min((page-1)*limit, len(recs))
		to := min(from+limit, len(recs))
		out := make([]map[string]any, 0, to-from)
		for _, c := range recs[from:to] {
			out = append(out, map[string]any{
				"call_uuid": c.UUID, "start_stamp": c.Start.Format("2006-01-02 15:04:05 -0700"),
				"direction": "Giden", "caller_id_number": "1014", "destination_number": "05551234567",
				"duration": "00:01:00", "talk_duration": "00:00:50", "answer_stamp": "x", "result": "Cevaplandı",
			})
		}
		pages := (len(recs) + limit - 1) / limit
		_ = json.NewEncoder(w).Encode(map[string]any{
			"cdrs":       out,
			"pagination": map[string]int{"page": page, "total_count": len(recs), "total_pages": pages, "limit": limit},
		})
	})
}

// TestPhoneMirrorFillsAGap: the phone system's API was down while 350
// calls passed. The regular copy fetches only the newest page; it must keep
// paging back, a page per run, until it reaches what was already stored.
func TestPhoneMirrorFillsAGap(t *testing.T) {
	pbx := newFakePBX(t)
	feed := &cdrFeed{}
	feed.serve(pbx)
	srv, db := testServer(t, pbx.use)

	run := time.Now().UnixNano()
	// The newest record stored before the outage, newer than anything else
	// in the database.
	var latest time.Time
	if err := db.Raw("SELECT COALESCE(max(start_at), now()) FROM pbx_cdrs").Scan(&latest).Error; err != nil {
		t.Fatal(err)
	}
	base := latest.Add(time.Minute).Truncate(time.Second)
	known := cdrStub{UUID: fmt.Sprintf("gap-%d-known", run), Start: base}
	if err := db.Exec(`INSERT INTO pbx_cdrs (call_uuid, start_at, start_stamp, direction) VALUES (?, ?, ?, 'outbound')`,
		known.UUID, known.Start, known.Start.Format("2006-01-02 15:04:05 -0700")).Error; err != nil {
		t.Fatal(err)
	}
	uuids := []string{known.UUID}
	feed.records = append(feed.records, known)
	for i := 1; i <= 350; i++ {
		c := cdrStub{UUID: fmt.Sprintf("gap-%d-%03d", run, i), Start: base.Add(time.Duration(i) * time.Second)}
		feed.records = append(feed.records, c)
		uuids = append(uuids, c.UUID)
	}
	t.Cleanup(func() { db.Exec("DELETE FROM pbx_cdrs WHERE call_uuid IN ?", uuids) })

	for pass := 1; pass <= 6; pass++ {
		before := feed.requests
		srv.phone.MirrorHead(t.Context())
		if n := feed.requests - before; n > 2 {
			t.Errorf("pass %d asked the phone system %d times; the budget allows two", pass, n)
		}
	}
	var stored int64
	db.Model(&models.PBXCDR{}).Where("call_uuid IN ?", uuids).Count(&stored)
	if stored != int64(len(uuids)) {
		t.Errorf("%d of %d records in the copy after the gap was filled", stored, len(uuids))
	}
	// With the gap filled, a run asks for the newest page only.
	before := feed.requests
	srv.phone.MirrorHead(t.Context())
	if n := feed.requests - before; n != 1 {
		t.Errorf("a run without a gap asked %d times, want 1", n)
	}
}

// TestPhoneLostCallEnd: calls whose end never reached the panel. One the
// phone system's record matches gets its real length; one nothing matches
// is marked unknown, not two hours, and is left out of the short/long
// counts; once its record arrives, it is sized; a late end from the panel
// still lands.
func TestPhoneLostCallEnd(t *testing.T) {
	pbx := newFakePBX(t)
	srv, db := testServer(t, pbx.use)
	agent := seedPeople(t, db, 1, enums.RoleSalesTeam, true)[0]
	ext := *agent.SIPExtension
	b := newBrowser(t, srv.app, nil, agent)
	b.mustSignIn()

	run := time.Now().UnixNano()
	started := time.Now().Add(-3 * time.Hour).Truncate(time.Second)
	answered := started.Add(5 * time.Second)
	type row struct {
		id, peer string
		answered bool
	}
	rows := []row{
		{fmt.Sprintf("lost-%d-cdr", run), "05551110001", true},
		{fmt.Sprintf("lost-%d-none", run), "05551110002", true},
		{fmt.Sprintf("lost-%d-ring", run), "05551110003", false},
		{fmt.Sprintf("lost-%d-late", run), "05551110004", true},
	}
	var callIDs []string
	for _, r := range rows {
		callIDs = append(callIDs, r.id)
		var ans *time.Time
		if r.answered {
			ans = &answered
		}
		if err := db.Exec(`INSERT INTO call_logs (call_id, user_id, direction, peer_number, peer_key, disposition, started_at, answered_at)
			VALUES (?, ?, 'outbound', ?, ?, 'in_progress', ?, ?)`, r.id, agent.ID, r.peer, r.peer[1:], started, ans).Error; err != nil {
			t.Fatal(err)
		}
	}
	cdrIDs := []string{fmt.Sprintf("lost-%d-cdr1", run), fmt.Sprintf("lost-%d-cdr2", run)}
	t.Cleanup(func() {
		db.Exec("DELETE FROM call_logs WHERE call_id IN ?", callIDs)
		db.Exec("DELETE FROM pbx_cdrs WHERE call_uuid IN ?", cdrIDs)
	})
	addCDR := func(uuid, peer string, talk int) {
		t.Helper()
		if err := db.Exec(`INSERT INTO pbx_cdrs (call_uuid, start_at, start_stamp, direction, caller_id_number, destination_number,
			caller_num, dest_num, caller_ext, talk_duration, answer_stamp) VALUES (?, ?, '-', 'outbound', ?, ?, '', ?, ?, ?, 'x')`,
			uuid, started.Add(time.Second), ext, peer, peer[1:], ext, strconv.Itoa(talk)).Error; err != nil {
			t.Fatal(err)
		}
	}
	addCDR(cdrIDs[0], rows[0].peer, 95)

	srv.phone.FinalizeStaleCalls(t.Context())

	get := func(id string) models.CallLog {
		t.Helper()
		var l models.CallLog
		if err := db.Where("call_id = ?", id).First(&l).Error; err != nil {
			t.Fatal(err)
		}
		return l
	}
	if l := get(rows[0].id); l.EndedAt == nil || l.DurationSeconds != 95 || l.DurationUnknown || l.Disposition != "answered" {
		t.Errorf("matched call: ended %v, %d s, unknown %v, %s; want the record's 95 s", l.EndedAt, l.DurationSeconds, l.DurationUnknown, l.Disposition)
	}
	if l := get(rows[1].id); l.EndedAt == nil || l.DurationSeconds != 0 || !l.DurationUnknown || l.Disposition != "answered" {
		t.Errorf("unmatched call: ended %v, %d s, unknown %v, %s; want an answered call of unknown length", l.EndedAt, l.DurationSeconds, l.DurationUnknown, l.Disposition)
	}
	if l := get(rows[2].id); l.EndedAt == nil || l.DurationUnknown || l.Disposition != "no_answer" {
		t.Errorf("unanswered call: ended %v, unknown %v, %s", l.EndedAt, l.DurationUnknown, l.Disposition)
	}

	// Unknown lengths are neither short nor long calls, and add no talk time.
	assertCounts := func(wantShort, wantLong int64) {
		t.Helper()
		var day struct {
			Short int64 `json:"short"`
			Long  int64 `json:"long"`
		}
		now := time.Now().In(tz.Istanbul)
		if started.Before(time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, tz.Istanbul)) {
			t.Log("the calls fall on yesterday's list; the talk-time check below still covers them")
			return
		}
		b.do(fiber.MethodGet, "/api/v1/calls/log/", nil).json(t, &day)
		if day.Short != wantShort || day.Long != wantLong {
			t.Errorf("today's list counts %d short and %d long, want %d and %d", day.Short, day.Long, wantShort, wantLong)
		}
	}
	assertCounts(0, 1)
	var talk int64
	if err := db.Raw(`SELECT COALESCE(SUM(duration_seconds) FILTER (WHERE disposition = 'answered' AND NOT duration_unknown), 0)
		FROM call_logs WHERE call_id IN ?`, callIDs).Scan(&talk).Error; err != nil {
		t.Fatal(err)
	}
	if talk != 95 {
		t.Errorf("talk time %d s, want 95 (nothing for the unknown ones)", talk)
	}

	// The record of the unknown call arrives late; the next pass sizes it.
	addCDR(cdrIDs[1], rows[1].peer, 42)
	srv.phone.FinalizeStaleCalls(t.Context())
	if l := get(rows[1].id); l.DurationSeconds != 42 || l.DurationUnknown {
		t.Errorf("late record: %d s, unknown %v; want 42 s", l.DurationSeconds, l.DurationUnknown)
	}

	// The panel's own end arrives after the close; it carries the length.
	if l := get(rows[3].id); !l.DurationUnknown {
		t.Fatalf("the fourth call should be unknown before its late end")
	}
	a := b.do(fiber.MethodPost, "/api/v1/calls/log/", map[string]any{"callId": rows[3].id, "phase": "end", "disposition": "answered", "durationSeconds": 300})
	if a.status >= 300 {
		t.Fatalf("late end answered %d %s", a.status, a.body)
	}
	if l := get(rows[3].id); l.DurationSeconds != 300 || l.DurationUnknown {
		t.Errorf("late end: %d s, unknown %v; want 300 s", l.DurationSeconds, l.DurationUnknown)
	}
	// A second end changes nothing now.
	b.do(fiber.MethodPost, "/api/v1/calls/log/", map[string]any{"callId": rows[3].id, "phase": "end", "disposition": "answered", "durationSeconds": 9})
	if l := get(rows[3].id); l.DurationSeconds != 300 {
		t.Errorf("a second end changed the length to %d", l.DurationSeconds)
	}
}
