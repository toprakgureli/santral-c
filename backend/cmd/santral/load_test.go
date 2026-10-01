package main

// Load tests: what an office does in a busy minute, against the real routes,
// database and Redis. Every request comes from one address, the way a call
// centre behind a single router looks to the server, so a limit that counts
// per address would show up here as refused requests. The phone system is a
// small stand-in on the loopback; nothing leaves the machine.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"

	"github.com/toprakgureli/santral-c/backend/configs"
	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
	"github.com/toprakgureli/santral-c/backend/pkg/enums"
	"github.com/toprakgureli/santral-c/backend/pkg/hash"
)

// loadPassword is every load-test user's password.
const loadPassword = "Load-Test-Pass-1"

// The office's address (from a range set aside for documentation). The
// server sits behind a proxy that reports the client's address, as nginx
// does in production.
const officeIP = "203.0.113.10"

// outsideIP returns an address outside the office, from another
// documentation range, that no earlier run used recently: a ban an earlier
// run left behind lasts fifteen minutes and would answer for this one.
func outsideIP(t *testing.T, db *gorm.DB) string {
	t.Helper()
	ip := fmt.Sprintf("198.51.100.%d", 100+time.Now().UnixNano()/int64(time.Second)%150)
	for _, q := range []string{"DELETE FROM ip_bans WHERE ip = ?", "DELETE FROM login_attempts WHERE ip = ?"} {
		if err := db.Exec(q, ip).Error; err != nil {
			t.Fatal(err)
		}
	}
	return ip
}

// officeSecurity is the production sign-in policy: wrong passwords are
// counted per browser and per account; the office address is trusted, so
// only an address outside it can be banned.
func officeSecurity(c *configs.Config) {
	c.App.TrustedProxies = "0.0.0.0"
	c.Security.TrustedIPs = "203.0.113.0/24"
	c.Security.IPFailureLimit = 100
	c.Security.DeviceFailureLimit = 5
	c.Security.IPBanDuration = 15 * time.Minute
	c.Security.AccountLockDuration = 5 * time.Minute
	c.Security.AttemptWindow = 15 * time.Minute
	c.Security.DistinctIPLimit = 10
}

// budget is how slow a load run's answers may be. The limits sit well
// above what a developer machine and CI measure (see the numbers each run
// logs), so only a real slowdown, five to ten times the usual, fails.
type budget struct {
	p95, max time.Duration
}

// tally counts the answers of a load run.
type tally struct {
	mu     sync.Mutex
	took   []time.Duration
	status map[int]int
}

func newTally() *tally { return &tally{status: map[int]int{}} }

func (s *tally) add(d time.Duration, status int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.took = append(s.took, d)
	s.status[status]++
}

// report logs how many requests ran, how fast, and fails the test when any
// was refused for load (429), broke on the server (5xx), or the answers
// were slower than the budget.
func (s *tally) report(t *testing.T, name string, elapsed time.Duration, b budget) {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	n := len(s.took)
	if n == 0 {
		t.Fatalf("%s: no requests ran", name)
	}
	sorted := slices.Clone(s.took)
	slices.Sort(sorted)
	pct := func(p float64) time.Duration { return sorted[min(n-1, int(float64(n)*p))] }
	perMin := float64(n) / elapsed.Minutes()
	p95, slowest := pct(0.95), sorted[n-1]
	t.Logf("%s: %d requests in %s (%.0f per minute), p50 %s, p95 %s (budget %s), max %s (budget %s), answers %v",
		name, n, elapsed.Round(time.Millisecond), perMin, pct(0.50), p95, b.p95, slowest, b.max, s.status)
	if p95 > b.p95 {
		t.Errorf("%s: p95 %s is over the budget of %s", name, p95, b.p95)
	}
	if slowest > b.max {
		t.Errorf("%s: the slowest answer took %s, over the budget of %s", name, slowest, b.max)
	}
	for code, count := range s.status {
		if code == fiber.StatusTooManyRequests || code >= 500 {
			t.Errorf("%s: %d requests answered %d", name, count, code)
		}
	}
}

// browser is one person's browser: its cookies and the answers it got.
type browser struct {
	t     *testing.T
	app   *fiber.App
	stats *tally
	user  models.User
	// ip is the address the proxy reports for this browser.
	ip  string
	mu  sync.Mutex
	jar map[string]*http.Cookie
}

func newBrowser(t *testing.T, app *fiber.App, stats *tally, u models.User) *browser {
	return &browser{t: t, app: app, stats: stats, user: u, ip: officeIP, jar: map[string]*http.Cookie{}}
}

type answer struct {
	status int
	body   []byte
}

// json decodes the answer into v, failing the test when it cannot.
func (a answer) json(t *testing.T, v any) {
	t.Helper()
	if err := json.Unmarshal(a.body, v); err != nil {
		t.Errorf("answer %d could not be read: %v: %s", a.status, err, a.body)
	}
}

// do sends one request with the browser's cookies and keeps what it sets.
func (b *browser) do(method, path string, body any) answer {
	return b.doWith(method, path, body, nil)
}

// doWith is do with extra request headers.
func (b *browser) doWith(method, path string, body any, header http.Header) answer {
	var r io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			b.t.Errorf("%s %s: %v", method, path, err)
			return answer{}
		}
		r = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, path, r)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set(fiber.HeaderXForwardedFor, b.ip)
	for k, v := range header {
		req.Header[k] = v
	}
	b.mu.Lock()
	for _, c := range b.jar {
		req.AddCookie(c)
	}
	b.mu.Unlock()
	start := time.Now()
	res, err := b.app.Test(req, 60_000)
	took := time.Since(start)
	if err != nil {
		b.t.Errorf("%s %s: %v", method, path, err)
		return answer{}
	}
	defer func() { _ = res.Body.Close() }()
	raw, _ := io.ReadAll(res.Body)
	b.mu.Lock()
	for _, c := range res.Cookies() {
		if c.Value == "" || c.MaxAge < 0 || (!c.Expires.IsZero() && c.Expires.Before(time.Now())) {
			delete(b.jar, c.Name)
		} else {
			b.jar[c.Name] = c
		}
	}
	b.mu.Unlock()
	if b.stats != nil {
		b.stats.add(took, res.StatusCode)
	}
	return answer{status: res.StatusCode, body: raw}
}

func (b *browser) login(password string) answer {
	return b.do(fiber.MethodPost, "/api/v1/auth/login", map[string]string{"email": b.user.Email, "password": password})
}

// cookies is a copy of the browser's current cookies.
func (b *browser) cookies() map[string]*http.Cookie {
	b.mu.Lock()
	defer b.mu.Unlock()
	return copyJar(b.jar)
}

func copyJar(m map[string]*http.Cookie) map[string]*http.Cookie {
	out := make(map[string]*http.Cookie, len(m))
	for k, v := range m {
		c := *v
		out[k] = &c
	}
	return out
}

// mustSignIn signs the browser in and fails the test otherwise.
func (b *browser) mustSignIn() {
	b.t.Helper()
	if a := b.login(loadPassword); a.status != fiber.StatusOK {
		b.t.Fatalf("%s could not sign in: %d %s", b.user.Email, a.status, a.body)
	}
}

// together runs fn for 0..n-1 at the same time and waits for all of them.
func together(n int, fn func(i int)) {
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			fn(i)
		}()
	}
	close(start)
	wg.Wait()
}

var loadRun atomic.Int64

// seedPeople adds n active users with the given role. With extensions each
// gets a phone extension of its own. They are switched off again, and their
// extensions freed, when the test ends.
func seedPeople(t *testing.T, db *gorm.DB, n int, role enums.Role, extensions bool) []models.User {
	t.Helper()
	pw, err := hash.Password(loadPassword)
	if err != nil {
		t.Fatal(err)
	}
	var r models.Role
	if err := db.Where("name = ?", string(role)).First(&r).Error; err != nil {
		t.Fatalf("role %s: %v", role, err)
	}
	run := fmt.Sprintf("%d-%d", time.Now().UnixNano(), loadRun.Add(1))
	users := make([]models.User, n)
	for i := range users {
		u := models.User{
			Name:      fmt.Sprintf("Yük %s %d", role, i+1),
			Email:     fmt.Sprintf("load-%s-%d@load-test.local", run, i),
			Password:  pw,
			Active:    true,
			MFAExempt: true,
			Roles:     []models.Role{r},
		}
		if extensions {
			ext := freeExtension(t, db)
			u.SIPExtension = &ext
		}
		if err := db.Omit("Roles.*").Create(&u).Error; err != nil {
			t.Fatal(err)
		}
		users[i] = u
	}
	t.Cleanup(func() {
		db.Exec("UPDATE users SET active = false, sip_extension = NULL WHERE id IN ?", ids(users))
		db.Exec("DELETE FROM sessions WHERE user_id IN ?", ids(users))
	})
	return users
}

// ids lists the users' ids.
func ids(users []models.User) []uint {
	out := make([]uint, len(users))
	for i, u := range users {
		out[i] = u.ID
	}
	return out
}

var extMu sync.Mutex

// freeExtension returns a four-digit extension nobody holds. People are
// seeded one after another, so the next one taken is never handed out twice.
func freeExtension(t *testing.T, db *gorm.DB) string {
	t.Helper()
	extMu.Lock()
	defer extMu.Unlock()
	var taken []string
	if err := db.Model(&models.User{}).Where("sip_extension IS NOT NULL").Pluck("sip_extension", &taken).Error; err != nil {
		t.Fatal(err)
	}
	for n := 2000; n < 9999; n++ {
		if ext := fmt.Sprint(n); !slices.Contains(taken, ext) {
			return ext
		}
	}
	t.Fatal("no free extension")
	return ""
}

// ---------------------------------------------------------------- history

// Sizes of the history the load tests run on: about half a year of a busy
// office, so lists, counts and lookups work on full tables.
const (
	seedStaff     = 60
	seedCalls     = 300_000
	seedRecords   = 300_000
	seedContacts  = 50_000
	seedChatLines = 200_000
)

var (
	seedOnce  sync.Once
	seedErr   error
	seedGroup uint
)

// bigHistory fills the database once with old calls, phone records,
// contacts and a busy chat room. A database that already holds it is used
// as it is. It returns the busy room's id.
func bigHistory(t *testing.T, db *gorm.DB) uint {
	t.Helper()
	seedOnce.Do(func() {
		began := time.Now()
		var have int64
		if seedErr = db.Raw("SELECT count(*) FROM call_logs WHERE call_id LIKE 'seed-%'").Scan(&have).Error; seedErr != nil {
			return
		}
		if have < seedCalls {
			seedErr = db.Transaction(func(tx *gorm.DB) error {
				for _, q := range []string{
					`INSERT INTO users (name, email, password, active, mfa_exempt, created_at, updated_at)
					 SELECT 'Eski Temsilci ' || g, 'seed-' || g || '@load-test.local', '-', false, true, now(), now()
					 FROM generate_series(1, ` + fmt.Sprint(seedStaff) + `) g ON CONFLICT DO NOTHING`,
					`INSERT INTO call_logs (call_id, user_id, direction, peer_number, peer_key, disposition, started_at, answered_at, ended_at, duration_seconds, hooks_done)
					 SELECT 'seed-' || g, u.id, CASE WHEN g % 3 = 0 THEN 'inbound' ELSE 'outbound' END,
					        '0555' || lpad((g % 4000000)::text, 7, '0'), '555' || lpad((g % 4000000)::text, 7, '0'),
					        CASE WHEN g % 5 = 0 THEN 'no_answer' ELSE 'answered' END,
					        now() - (g % 180) * interval '1 day' - (g % 600) * interval '1 minute',
					        now() - (g % 180) * interval '1 day' - (g % 600) * interval '1 minute' + interval '8 seconds',
					        now() - (g % 180) * interval '1 day' - (g % 600) * interval '1 minute' + interval '3 minutes',
					        g % 400, true
					 FROM generate_series(1, ` + fmt.Sprint(seedCalls) + `) g
					 JOIN users u ON u.email = 'seed-' || (1 + g % ` + fmt.Sprint(seedStaff) + `) || '@load-test.local'
					 ON CONFLICT DO NOTHING`,
					`INSERT INTO pbx_cdrs (call_uuid, start_at, start_stamp, direction, caller_id_number, destination_number, caller_num, dest_num, caller_ext, talk_duration, answer_stamp)
					 SELECT 'seed-cdr-' || g, now() - (g % 180) * interval '1 day', '-', 'outbound', (3000 + g % 60)::text,
					        '0555' || lpad((g % 4000000)::text, 7, '0'), '', '555' || lpad((g % 4000000)::text, 7, '0'),
					        (3000 + g % 60)::text, (g % 400)::text, '-'
					 FROM generate_series(1, ` + fmt.Sprint(seedRecords) + `) g ON CONFLICT DO NOTHING`,
					`INSERT INTO contacts (name, company)
					 SELECT 'Müşteri Kayıt ' || g, 'Firma ' || (g % 900) FROM generate_series(1, ` + fmt.Sprint(seedContacts) + `) g`,
					`INSERT INTO contact_phones (contact_id, label, number_e164, is_primary)
					 SELECT c.id, 'mobile', '+90544' || lpad(c.id::text, 7, '0'), true
					 FROM contacts c WHERE c.name LIKE 'Müşteri Kayıt %' ON CONFLICT DO NOTHING`,
					`INSERT INTO chat_groups (kind, name, post_policy) VALUES ('group', 'Kalabalık oda (yük testi)', 'everyone')`,
					`INSERT INTO chat_messages (group_id, sender_id, kind, body, created_at)
					 SELECT (SELECT max(id) FROM chat_groups WHERE name = 'Kalabalık oda (yük testi)'), u.id, 'text',
					        'Eski mesaj ' || g, now() - (` + fmt.Sprint(seedChatLines) + ` - g) * interval '1 minute'
					 FROM generate_series(1, ` + fmt.Sprint(seedChatLines) + `) g
					 JOIN users u ON u.email = 'seed-' || (1 + g % ` + fmt.Sprint(seedStaff) + `) || '@load-test.local'`,
				} {
					if err := tx.Exec(q).Error; err != nil {
						return err
					}
				}
				return nil
			})
			if seedErr != nil {
				return
			}
			db.Exec("ANALYZE")
		}
		seedErr = db.Raw("SELECT max(id) FROM chat_groups WHERE name = 'Kalabalık oda (yük testi)'").Scan(&seedGroup).Error
		t.Logf("history ready in %s: %d calls, %d phone records, %d contacts, %d chat lines",
			time.Since(began).Round(time.Millisecond), seedCalls, seedRecords, seedContacts, seedChatLines)
	})
	if seedErr != nil {
		t.Fatalf("history: %v", seedErr)
	}
	return seedGroup
}

// ---------------------------------------------------------------- phone system

// fakePBX stands in for the phone system's API: it places calls, takes
// do-not-disturb changes and lists nothing else.
type fakePBX struct {
	srv  *httptest.Server
	mu   sync.Mutex
	next int
	// calls maps a call id to "extension>destination".
	calls map[string]string
	// dnd is the last do-not-disturb state per extension.
	dnd     map[string]string
	refused atomic.Int64
	run     int64
}

const pbxKey = "load-test-key"

func newFakePBX(t *testing.T) *fakePBX {
	t.Helper()
	p := &fakePBX{calls: map[string]string{}, dnd: map[string]string{}, run: time.Now().UnixNano()}
	mux := http.NewServeMux()
	keyed := func(fn http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("key") != pbxKey {
				p.refused.Add(1)
				http.Error(w, "bad key", http.StatusUnauthorized)
				return
			}
			fn(w, r)
		}
	}
	mux.HandleFunc("/originate", keyed(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		p.mu.Lock()
		p.next++
		// Call ids are unique for good, like the real system's, so a rerun
		// on the same database never meets an earlier run's calls.
		id := fmt.Sprintf("pbx-%d-%06d", p.run, p.next)
		p.calls[id] = q.Get("extension") + ">" + q.Get("destination")
		p.mu.Unlock()
		_, _ = w.Write([]byte(id))
	}))
	mux.HandleFunc("/dnd/", keyed(func(w http.ResponseWriter, r *http.Request) {
		p.mu.Lock()
		p.dnd[strings.TrimPrefix(r.URL.Path, "/dnd/")] = r.URL.Query().Get("state")
		p.mu.Unlock()
		_, _ = w.Write([]byte(`"ok"`))
	}))
	// A recording: the address to fetch it from, then the sound itself.
	// The key comes in the form here, not in the address.
	mux.HandleFunc("/recording_url/", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil || r.PostForm.Get("key") != pbxKey {
			p.refused.Add(1)
			http.Error(w, "bad key", http.StatusUnauthorized)
			return
		}
		_, _ = fmt.Fprintf(w, "%q", p.srv.URL+"/rec/"+r.PostForm.Get("call_uuid"))
	})
	mux.HandleFunc("/rec/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "audio/mpeg")
		_, _ = w.Write([]byte("ID3-fake-recording"))
	})
	mux.HandleFunc("/user_statuses", keyed(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`[]`)) }))
	mux.HandleFunc("/queues", keyed(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`[]`)) }))
	mux.HandleFunc("/cdrs", keyed(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"cdrs":[],"pagination":{"page":1,"total_count":0,"total_pages":0,"limit":100}}`))
	}))
	p.srv = httptest.NewServer(mux)
	t.Cleanup(p.srv.Close)
	return p
}

func (p *fakePBX) use(c *configs.Config) {
	c.Bulutsantralim = configs.Bulutsantralim{Enabled: true, APIKey: pbxKey, APIBase: p.srv.URL}
}

// ---------------------------------------------------------------- the tests

// loadTest skips a load test in a -short run (the race detector run in CI
// uses one; it would make these minutes long).
func loadTest(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("load test; runs without -short")
	}
}

// TestLoadOfficeSignsIn: forty people sign in at the same minute from one
// address and work through 2,400 requests; some mistype their password,
// two tabs renew one session at once, everybody signs out. Nobody may be
// refused for load and a signed-out session must stop working.
func TestLoadOfficeSignsIn(t *testing.T) {
	loadTest(t)
	srv, db := testServer(t, officeSecurity)
	bigHistory(t, db)
	people := seedPeople(t, db, 40, enums.RoleSalesTeam, false)
	stats := newTally()
	browsers := make([]*browser, len(people))
	for i, u := range people {
		browsers[i] = newBrowser(t, srv.app, stats, u)
	}

	began := time.Now()
	together(len(browsers), func(i int) {
		if a := browsers[i].login(loadPassword); a.status != fiber.StatusOK {
			t.Errorf("%s: sign in answered %d %s", people[i].Email, a.status, a.body)
		}
	})
	if t.Failed() {
		t.FailNow()
	}

	reads := []string{"/api/v1/auth/me", "/api/v1/shift/", "/api/v1/calls/log/", "/api/v1/teams/overview", "/api/v1/contacts/?q=a", "/api/v1/settings/break-limit"}
	together(len(browsers), func(i int) {
		for k := range 60 {
			a := browsers[i].do(fiber.MethodGet, reads[(i+k)%len(reads)], nil)
			if a.status != fiber.StatusOK {
				t.Errorf("%s: GET %s answered %d %s", people[i].Email, reads[(i+k)%len(reads)], a.status, a.body)
				return
			}
		}
	})

	// Five people mistype their password three times, then get it right,
	// each in their own browser; the rest of the office is not affected.
	clumsy := seedPeople(t, db, 5, enums.RoleSalesTeam, false)
	together(len(clumsy), func(i int) {
		b := newBrowser(t, srv.app, stats, clumsy[i])
		for range 3 {
			if a := b.login("Wrong-Pass-123"); a.status != fiber.StatusUnauthorized {
				t.Errorf("wrong password answered %d %s", a.status, a.body)
			}
		}
		if a := b.login(loadPassword); a.status != fiber.StatusOK {
			t.Errorf("%s after three wrong tries: %d %s", clumsy[i].Email, a.status, a.body)
		}
	})

	// One browser keeps guessing: after the browser limit it is held back,
	// while a colleague at the next desk still signs in.
	// Its refusals are the point, so they stay out of the tally.
	guesser := newBrowser(t, srv.app, nil, clumsy[0])
	held := false
	for range 8 {
		if a := guesser.login("Wrong-Pass-123"); a.status != fiber.StatusUnauthorized {
			held = a.status != fiber.StatusOK && a.status < 500
		}
	}
	if !held {
		t.Error("a browser guessing passwords was never held back")
	}
	colleague := newBrowser(t, srv.app, stats, people[0])
	if a := colleague.login(loadPassword); a.status != fiber.StatusOK {
		t.Errorf("after one browser was held back, a colleague could not sign in: %d %s", a.status, a.body)
	}

	// A bad morning: thirty people each mistype their password four times
	// within a few minutes, 120 wrong tries from the office address in all.
	// The office is trusted, so nobody is locked out by address and all of
	// them get in at the next try.
	typos := seedPeople(t, db, 30, enums.RoleSalesTeam, false)
	together(len(typos), func(i int) {
		b := newBrowser(t, srv.app, stats, typos[i])
		for range 4 {
			if a := b.login("Wrong-Pass-123"); a.status != fiber.StatusUnauthorized {
				t.Errorf("%s: wrong password answered %d %s", typos[i].Email, a.status, a.body)
			}
		}
		if a := b.login(loadPassword); a.status != fiber.StatusOK {
			t.Errorf("%s: after 120 wrong tries in the office, sign in answered %d %s", typos[i].Email, a.status, a.body)
		}
	})

	// Someone outside tries passwords against made-up accounts from fresh
	// browsers; past the address limit that address is held back, and the
	// office keeps working. The address starts clean, so the first tries
	// are answered as wrong passwords, not refused for an earlier run.
	outside := outsideIP(t, db)
	held = false
	for k := range 110 {
		b := newBrowser(t, srv.app, nil, models.User{Email: fmt.Sprintf("nobody-%d-%d@load-test.local", time.Now().UnixNano(), k)})
		b.ip = outside
		a := b.login("Guess-Pass-123")
		if a.status == fiber.StatusTooManyRequests {
			if k < 20 {
				t.Errorf("an outside address was held back after only %d tries", k)
			}
			held = true
			break
		}
		if a.status != fiber.StatusUnauthorized {
			t.Errorf("outside guess %d answered %d %s", k, a.status, a.body)
		}
	}
	if !held {
		t.Error("an outside address guessing passwords was never held back")
	}
	if a := newBrowser(t, srv.app, stats, people[1]).login(loadPassword); a.status != fiber.StatusOK {
		t.Errorf("with an outside address banned, the office could not sign in: %d %s", a.status, a.body)
	}

	// Someone outside types wrong passwords for a colleague's account until
	// it locks. The colleague still gets in from their usual browser; a
	// browser that never signed in to that account waits for the lock.
	victim := people[2]
	// These addresses start clean even when the test ran a moment ago.
	db.Exec("DELETE FROM ip_bans WHERE ip LIKE '192.0.2.%'")
	db.Exec("DELETE FROM login_attempts WHERE ip LIKE '192.0.2.%'")
	for k := range 12 {
		b := newBrowser(t, srv.app, nil, victim)
		b.ip = fmt.Sprintf("192.0.2.%d", 10+k)
		if a := b.login("Guess-Pass-123"); a.status != fiber.StatusUnauthorized && a.status != fiber.StatusLocked {
			t.Errorf("guess %d answered %d %s", k, a.status, a.body)
		}
	}
	usual := newBrowser(t, srv.app, stats, victim)
	usual.jar = browsers[2].cookies() // the browser they signed in with this morning
	if a := usual.login(loadPassword); a.status != fiber.StatusOK {
		t.Errorf("after someone else locked the account, its owner could not sign in from their own browser: %d %s", a.status, a.body)
	}
	stranger := newBrowser(t, srv.app, nil, victim)
	if a := stranger.login(loadPassword); a.status != fiber.StatusLocked {
		t.Errorf("a browser new to the locked account answered %d, want 423", a.status)
	}

	// Two tabs of one browser renew the same session at the same moment.
	together(len(browsers), func(i int) {
		b := browsers[i]
		before := b.cookies()
		var wg sync.WaitGroup
		codes := make([]int, 2)
		bodies := make([]string, 2)
		for k := range 2 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				tab := newBrowser(t, srv.app, stats, b.user)
				tab.jar = copyJar(before)
				r := tab.do(fiber.MethodPost, "/api/v1/auth/refresh", nil)
				codes[k], bodies[k] = r.status, string(r.body)
				if k == 0 {
					b.mu.Lock()
					b.jar = tab.cookies()
					b.mu.Unlock()
				}
			}()
		}
		wg.Wait()
		for _, c := range codes {
			if c != fiber.StatusOK {
				t.Errorf("%s: renewing from two tabs answered %v %q", b.user.Email, codes, bodies)
				break
			}
		}
		if a := b.do(fiber.MethodGet, "/api/v1/auth/me", nil); a.status != fiber.StatusOK {
			t.Errorf("%s: after renewing, me answered %d", b.user.Email, a.status)
		}
	})

	// Everybody signs out; the old cookies no longer open anything.
	together(len(browsers), func(i int) {
		b := browsers[i]
		old := b.cookies()
		if a := b.do(fiber.MethodPost, "/api/v1/auth/logout", nil); a.status >= 300 {
			t.Errorf("%s: sign out answered %d", b.user.Email, a.status)
		}
		stale := newBrowser(t, srv.app, nil, b.user)
		stale.jar = old
		if a := stale.do(fiber.MethodGet, "/api/v1/auth/me", nil); a.status != fiber.StatusUnauthorized {
			t.Errorf("%s: a signed-out session still answered %d", b.user.Email, a.status)
		}
	})
	// Every sign-in waits for a deliberately slow password check, so this
	// budget is the widest; a developer machine measures about 0.8 s p95.
	stats.report(t, "office sign-in", time.Since(began), budget{p95: 8 * time.Second, max: 20 * time.Second})
}

// TestLoadCalls: twenty agents open their shift, place five calls each
// through the phone system, log every phase from the softphone, go on a
// break and come back, and hand a call over. Every call must reach the
// phone system from the agent's own extension, land in the agent's call
// history, and be confirmed against the phone system's records.
func TestLoadCalls(t *testing.T) {
	loadTest(t)
	pbx := newFakePBX(t)
	srv, db := testServer(t, officeSecurity, pbx.use)
	bigHistory(t, db)
	agents := seedPeople(t, db, 20, enums.RoleSalesTeam, true)
	// One more without the right to hand calls outside the phone system.
	inside := seedPeople(t, db, 1, enums.RoleSalesTeam, true)
	restrictTransfers(t, db, inside[0].ID)

	stats := newTally()
	browsers := make([]*browser, len(agents))
	for i, u := range agents {
		browsers[i] = newBrowser(t, srv.app, stats, u)
		browsers[i].mustSignIn()
	}

	const perAgent = 5
	type placed struct {
		agent  int
		callID string
		peer   string
		at     time.Time
	}
	var mu sync.Mutex
	var calls []placed

	began := time.Now()
	together(len(browsers), func(i int) {
		b := browsers[i]
		// A call needs an open shift.
		if a := b.do(fiber.MethodPost, "/api/v1/calls/originate", map[string]string{"to": "05550000000"}); a.status != fiber.StatusForbidden {
			t.Errorf("%s: a call before the shift answered %d", b.user.Email, a.status)
		}
		if a := b.do(fiber.MethodPost, "/api/v1/shift/start", nil); a.status != fiber.StatusOK {
			t.Errorf("%s: shift start answered %d %s", b.user.Email, a.status, a.body)
			return
		}
		if a := b.do(fiber.MethodPost, "/api/v1/pbx/status", map[string]string{"state": "available"}); a.status >= 300 {
			t.Errorf("%s: status available answered %d %s", b.user.Email, a.status, a.body)
		}
		for k := range perAgent {
			peer := fmt.Sprintf("0555%03d%04d", i, k)
			a := b.do(fiber.MethodPost, "/api/v1/calls/originate", map[string]string{"to": peer})
			if a.status != fiber.StatusOK {
				t.Errorf("%s: originate answered %d %s", b.user.Email, a.status, a.body)
				continue
			}
			var res struct {
				CallUUID string `json:"callUuid"`
			}
			a.json(t, &res)
			callID := res.CallUUID
			at := time.Now()
			for _, phase := range []map[string]any{
				{"callId": callID, "phase": "start", "direction": "outbound", "peer": peer},
				{"callId": callID, "phase": "answer"},
				{"callId": callID, "phase": "end", "disposition": "answered", "durationSeconds": 42},
				// A second end from a slow tab changes nothing.
				{"callId": callID, "phase": "end", "disposition": "answered", "durationSeconds": 42},
			} {
				if a := b.do(fiber.MethodPost, "/api/v1/calls/log/", phase); a.status >= 300 {
					t.Errorf("%s: call log %v answered %d %s", b.user.Email, phase["phase"], a.status, a.body)
				}
			}
			// The next caller's number is looked up while the phone rings.
			if a := b.do(fiber.MethodGet, "/api/v1/calls/log/lookup?number="+peer, nil); a.status != fiber.StatusOK {
				t.Errorf("%s: lookup answered %d %s", b.user.Email, a.status, a.body)
			}
			mu.Lock()
			calls = append(calls, placed{agent: i, callID: callID, peer: peer, at: at})
			mu.Unlock()
		}
		// A break and back.
		for _, state := range []string{"break", "available", "backoffice", "available"} {
			if a := b.do(fiber.MethodPost, "/api/v1/pbx/status", map[string]string{"state": state}); a.status >= 300 {
				t.Errorf("%s: status %s answered %d %s", b.user.Email, state, a.status, a.body)
			}
		}
		if a := b.do(fiber.MethodGet, "/api/v1/pbx/status", nil); a.status != fiber.StatusOK {
			t.Errorf("%s: status answered %d", b.user.Email, a.status)
		} else {
			var p struct {
				State string `json:"state"`
			}
			a.json(t, &p)
			if p.State != "available" {
				t.Errorf("%s: state %q after coming back, want available", b.user.Email, p.State)
			}
		}
		// Handing a call to a colleague's extension, then to an outside number.
		next := *agents[(i+1)%len(agents)].SIPExtension
		if a := b.do(fiber.MethodPost, "/api/v1/calls/transfer", map[string]string{"callId": "c", "target": next}); a.status >= 300 {
			t.Errorf("%s: transfer to %s answered %d %s", b.user.Email, next, a.status, a.body)
		}
		if a := b.do(fiber.MethodPost, "/api/v1/calls/transfer", map[string]string{"callId": "c", "target": "05321234567"}); a.status >= 300 {
			t.Errorf("%s: outside transfer answered %d %s", b.user.Email, a.status, a.body)
		}
	})
	elapsed := time.Since(began)

	// Someone without the outside-transfer right is held to the phone system.
	in := newBrowser(t, srv.app, stats, inside[0])
	in.mustSignIn()
	if a := in.do(fiber.MethodPost, "/api/v1/calls/transfer", map[string]string{"callId": "c", "target": "2001"}); a.status >= 300 {
		t.Errorf("inside transfer answered %d %s", a.status, a.body)
	}
	if a := in.do(fiber.MethodPost, "/api/v1/calls/transfer", map[string]string{"callId": "c", "target": "05321234567"}); a.status != fiber.StatusForbidden {
		t.Errorf("outside transfer without the right answered %d, want 403", a.status)
	}

	if len(calls) != len(agents)*perAgent {
		t.Fatalf("%d calls placed, want %d", len(calls), len(agents)*perAgent)
	}
	// Every call reached the phone system from the caller's own extension.
	pbx.mu.Lock()
	if len(pbx.calls) != len(calls) {
		t.Errorf("the phone system got %d calls, want %d", len(pbx.calls), len(calls))
	}
	for _, c := range calls {
		want := *agents[c.agent].SIPExtension + ">" + c.peer
		if got := pbx.calls[c.callID]; got != want {
			t.Errorf("call %s reached the phone system as %q, want %q", c.callID, got, want)
		}
	}
	for _, a := range agents {
		if pbx.dnd[*a.SIPExtension] != "off" {
			t.Errorf("extension %s: do-not-disturb %q after coming back, want off", *a.SIPExtension, pbx.dnd[*a.SIPExtension])
		}
	}
	pbx.mu.Unlock()
	if n := pbx.refused.Load(); n > 0 {
		t.Errorf("the phone system refused %d requests", n)
	}

	// Each agent sees exactly their own five calls in the history.
	for i, b := range browsers {
		a := b.do(fiber.MethodGet, "/api/v1/calls/log/", nil)
		var list struct {
			Items []struct {
				ToNumber string `json:"toNumber"`
			} `json:"items"`
		}
		a.json(t, &list)
		got := 0
		for _, it := range list.Items {
			if strings.HasPrefix(it.ToNumber, fmt.Sprintf("0555%03d", i)) {
				got++
			} else {
				t.Errorf("%s sees someone else's call to %s", b.user.Email, it.ToNumber)
			}
		}
		if got != perAgent {
			t.Errorf("%s: %d calls in the history, want %d (%s)", b.user.Email, got, perAgent, a.body)
		}
	}

	// The phone system's records arrive; the checker confirms every call.
	for _, c := range calls {
		ext := *agents[c.agent].SIPExtension
		if err := db.Exec(`INSERT INTO pbx_cdrs (call_uuid, start_at, start_stamp, direction, caller_id_number, destination_number,
			caller_num, dest_num, caller_ext, talk_duration, answer_stamp) VALUES (?, ?, ?, 'outbound', ?, ?, '', ?, ?, '40', ?)`,
			c.callID, c.at, c.at.Format(time.DateTime), ext, c.peer, c.peer, ext, c.at.Add(2*time.Second).Format(time.DateTime)).Error; err != nil {
			t.Fatal(err)
		}
	}
	callIDs := make([]string, len(calls))
	for i, c := range calls {
		callIDs[i] = c.callID
	}
	t.Cleanup(func() { db.Exec("DELETE FROM pbx_cdrs WHERE call_uuid IN ?", callIDs) })
	srv.callLog.VerifyPending(t.Context())
	var pending int64
	db.Model(&models.CallLog{}).Where("call_id IN ? AND hooks_done = false", callIDs).Count(&pending)
	if pending != 0 {
		t.Errorf("%d calls still wait for the phone record after it arrived", pending)
	}
	var talk []int
	db.Model(&models.CallLog{}).Where("call_id IN ?", callIDs).Pluck("duration_seconds", &talk)
	for _, s := range talk {
		if s != 40 {
			t.Errorf("a confirmed call kept %d seconds, want the phone system's 40", s)
			break
		}
	}

	// Shifts close.
	together(len(browsers), func(i int) {
		if a := browsers[i].do(fiber.MethodPost, "/api/v1/shift/end", nil); a.status != fiber.StatusOK {
			t.Errorf("%s: shift end answered %d %s", browsers[i].user.Email, a.status, a.body)
		}
	})
	// About 65 ms p95 and 350 ms at most on a developer machine.
	stats.report(t, "calls", elapsed, budget{p95: time.Second, max: 5 * time.Second})
}

// restrictTransfers gives a user a role that may hand calls on only inside
// the phone system.
func restrictTransfers(t *testing.T, db *gorm.DB, userID uint) {
	t.Helper()
	var perms []models.Permission
	keys := []string{string(enums.CallOriginate), string(enums.CallTransfer), string(enums.CDRViewOwn)}
	if err := db.Where("key IN ?", keys).Find(&perms).Error; err != nil {
		t.Fatal(err)
	}
	role := models.Role{Name: fmt.Sprintf("load-inside-%d", time.Now().UnixNano()), DisplayName: "Yalnız dahili aktarma", Permissions: perms}
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

// TestLoadTeamsBursts: twenty people in one group each send twenty messages
// back to back at the same time (400 lines), while ten pairs write to each
// other directly. Every line must be stored once, in the order each person
// sent it, and nobody may be slowed down by a limit.
func TestLoadTeamsBursts(t *testing.T) {
	loadTest(t)
	srv, db := testServer(t, officeSecurity)
	busy := bigHistory(t, db)
	owner := seedPeople(t, db, 1, enums.RoleManager, false)[0]
	members := seedPeople(t, db, 20, enums.RoleSalesTeam, false)
	stats := newTally()
	// Everyone also sits in a room with 200,000 old lines they never read,
	// so every overview counts a long unread tail.
	for _, m := range members {
		if err := db.Exec("INSERT INTO chat_members (group_id, user_id) VALUES (?, ?)", busy, m.ID).Error; err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		db.Exec("DELETE FROM chat_members WHERE group_id = ? AND user_id IN ?", busy, ids(members))
		db.Exec("DELETE FROM chat_receipts WHERE user_id IN ?", ids(members))
	})

	ob := newBrowser(t, srv.app, stats, owner)
	ob.mustSignIn()
	a := ob.do(fiber.MethodPost, "/api/v1/teams/groups", map[string]any{"name": "Yük testi", "postPolicy": "everyone", "memberIds": ids(members)})
	if a.status >= 300 {
		t.Fatalf("group create answered %d %s", a.status, a.body)
	}
	var group struct {
		ID uint `json:"id"`
	}
	a.json(t, &group)

	browsers := make([]*browser, len(members))
	for i, m := range members {
		browsers[i] = newBrowser(t, srv.app, stats, m)
		browsers[i].mustSignIn()
	}

	const perPerson = 20
	path := fmt.Sprintf("/api/v1/teams/groups/%d/messages", group.ID)
	began := time.Now()
	together(len(browsers), func(i int) {
		b := browsers[i]
		for k := range perPerson {
			if a := b.do(fiber.MethodPost, path, map[string]any{"body": fmt.Sprintf("kişi %02d mesaj %02d", i, k)}); a.status >= 300 {
				t.Errorf("%s: message %d answered %d %s", b.user.Email, k, a.status, a.body)
			}
			// Meanwhile the screen reloads the room, marks it read, refreshes
			// the room list and pages through the busy room.
			if k%5 == 0 {
				for _, r := range []struct{ method, path string }{
					{fiber.MethodGet, path},
					{fiber.MethodPost, fmt.Sprintf("/api/v1/teams/groups/%d/read", group.ID)},
					{fiber.MethodGet, "/api/v1/teams/overview"},
					{fiber.MethodGet, fmt.Sprintf("/api/v1/teams/groups/%d/messages", busy)},
				} {
					var body any
					if r.method == fiber.MethodPost {
						body = map[string]any{}
					}
					if a := b.do(r.method, r.path, body); a.status >= 300 {
						t.Errorf("%s: %s %s answered %d %s", b.user.Email, r.method, r.path, a.status, a.body)
					}
				}
			}
		}
	})

	// Ten pairs write to each other directly, ten lines each way.
	together(10, func(p int) {
		x, y := browsers[2*p], browsers[2*p+1]
		a := x.do(fiber.MethodPost, fmt.Sprintf("/api/v1/teams/dm/%d", y.user.ID), nil)
		if a.status >= 300 {
			t.Errorf("open dm answered %d %s", a.status, a.body)
			return
		}
		var dm struct {
			ID uint `json:"id"`
		}
		a.json(t, &dm)
		dmPath := fmt.Sprintf("/api/v1/teams/groups/%d/messages", dm.ID)
		var wg sync.WaitGroup
		for _, b := range []*browser{x, y} {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for k := range 10 {
					if a := b.do(fiber.MethodPost, dmPath, map[string]any{"body": fmt.Sprintf("dm %d", k)}); a.status >= 300 {
						t.Errorf("%s: dm line answered %d %s", b.user.Email, a.status, a.body)
					}
				}
			}()
		}
		wg.Wait()
		var n int64
		db.Model(&models.ChatMessage{}).Where("group_id = ? AND kind = 'text'", dm.ID).Count(&n)
		if n != 20 {
			t.Errorf("dm %d holds %d lines, want 20", dm.ID, n)
		}
	})
	elapsed := time.Since(began)

	// Every line was stored once, and each person's lines kept their order.
	var rows []struct {
		SenderID uint
		Body     string
	}
	db.Model(&models.ChatMessage{}).Where("group_id = ? AND kind = 'text'", group.ID).Order("id").Select("sender_id, body").Scan(&rows)
	if len(rows) != len(members)*perPerson {
		t.Errorf("the group holds %d lines, want %d", len(rows), len(members)*perPerson)
	}
	last := map[uint]string{}
	for _, r := range rows {
		if prev, ok := last[r.SenderID]; ok && r.Body <= prev {
			t.Errorf("sender %d: %q stored after %q", r.SenderID, r.Body, prev)
		}
		last[r.SenderID] = r.Body
	}
	// Someone added to the busy room later starts at its newest line: the
	// history is there to read but counts as read.
	late := seedPeople(t, db, 1, enums.RoleSalesTeam, false)[0]
	if a := ob.do(fiber.MethodPost, fmt.Sprintf("/api/v1/teams/groups/%d/members", busy), map[string]any{"userIds": []uint{late.ID}}); a.status >= 300 {
		t.Errorf("adding a member answered %d %s", a.status, a.body)
	}
	t.Cleanup(func() { db.Exec("DELETE FROM chat_members WHERE group_id = ? AND user_id = ?", busy, late.ID) })
	lb := newBrowser(t, srv.app, stats, late)
	lb.mustSignIn()
	var ov struct {
		Groups []struct {
			ID     uint  `json:"id"`
			Unread int64 `json:"unread"`
		} `json:"groups"`
	}
	lb.do(fiber.MethodGet, "/api/v1/teams/overview", nil).json(t, &ov)
	found := false
	for _, g := range ov.Groups {
		if g.ID == busy {
			found = true
			// The room's own "added" line may be new to them; the history is not.
			if g.Unread > 1 {
				t.Errorf("a member added later sees %d unread lines of old history", g.Unread)
			}
		}
	}
	if !found {
		t.Error("the member added later does not see the room")
	}

	// What a member's screen loads shows the newest lines.
	var page struct {
		Items []struct {
			Body string `json:"body"`
		} `json:"items"`
	}
	browsers[0].do(fiber.MethodGet, path, nil).json(t, &page)
	if len(page.Items) == 0 {
		t.Error("the room's first page is empty")
	}
	// About 160 ms p95 on a developer machine; the slowest answer, a page
	// of the room with 200,000 lines while the burst runs, takes seconds.
	stats.report(t, "teams", elapsed, budget{p95: 2 * time.Second, max: 20 * time.Second})
}

// TestRenewFromTwoTabs: a hundred people each have two tabs that renew the
// same session at the same moment. Both tabs must stay signed in.
func TestRenewFromTwoTabs(t *testing.T) {
	srv, db := testServer(t, officeSecurity)
	people := seedPeople(t, db, 100, enums.RoleSalesTeam, false)
	browsers := make([]*browser, len(people))
	// Signing in is not what this test is about; eight at a time keeps it
	// quick under the race detector too.
	gate := make(chan struct{}, 8)
	together(len(people), func(i int) {
		gate <- struct{}{}
		defer func() { <-gate }()
		browsers[i] = newBrowser(t, srv.app, nil, people[i])
		if r := browsers[i].login(loadPassword); r.status != fiber.StatusOK {
			t.Errorf("%s: sign in answered %d %s", people[i].Email, r.status, r.body)
		}
	})
	if t.Failed() {
		t.FailNow()
	}
	together(len(browsers)*2, func(i int) {
		b := browsers[i/2]
		tab := newBrowser(t, srv.app, nil, b.user)
		tab.jar = b.cookies()
		if r := tab.do(fiber.MethodPost, "/api/v1/auth/refresh", nil); r.status != fiber.StatusOK {
			t.Errorf("%s tab %d: renewing answered %d %s", b.user.Email, i%2, r.status, r.body)
			return
		}
		if r := tab.do(fiber.MethodGet, "/api/v1/auth/me", nil); r.status != fiber.StatusOK {
			t.Errorf("%s tab %d: after renewing, me answered %d %s", b.user.Email, i%2, r.status, r.body)
		}
	})
}
