package main

// The permission matrix: every route of the running server, called once by
// someone of every system role and once by someone without a role. Which
// permissions a route asks for is read from the server itself (the checks
// placed next to the routes report what they ask), so a new route is in the
// matrix the moment it is mounted.

import (
	"net/http"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"

	"github.com/toprakgureli/santral-c/backend/internal/middlewares"
	"github.com/toprakgureli/santral-c/backend/pkg/enums"
)

// probeHeader names the route a matrix request is for, so the permission
// checks it passes are filed under that route.
const probeHeader = "X-Route-Probe"

// probeID fills every path parameter: no row has it, so a request that is
// let through finds nothing to change.
const probeID = "2000000000"

// notProbed are routes the matrix leaves out, with the reason. Each is
// covered by a test of its own.
var notProbed = map[string]string{
	"POST /api/v1/auth/logout/everywhere": "ends the caller's own sessions; account tests cover it",
	"GET /api/v1/teams/stream":            "an event stream stays open; the deactivation test covers it",
	"GET /api/v1/wa/stream":               "an event stream stays open",
	"GET /api/v1/pbx/stream":              "an event stream stays open",
}

// checkedInsideGroups and checkedInside list the routes without a
// permission check next to the route, and why that is right: every route
// under a group, or one route by its pattern. A route that asks for no
// permission and is not listed fails the matrix: guard it with need() or
// add it here.
var checkedInsideGroups = map[string]string{
	"/api/v1/auth/":     "the caller's own sign-in, password and second step",
	"/api/v1/wa/":       "WhatsApp checks in its service: the module first, then the page or the device",
	"/api/v1/games/":    "reading the setup is open to everyone; playing is checked against games.play in the service",
	"/api/v1/shift/":    "the caller's own shift",
	"/api/v1/profile/":  "profiles every signed-in colleague may see",
	"/api/v1/users/me/": "the caller's own settings",
	"/api/v1/notices/":  "the caller's own notices",
}

var checkedInside = map[string]string{
	"/api/v1/version":                "the running build, for every signed-in person; the panel compares it with its own",
	"/api/v1/users/:id/avatar":       "a colleague's picture",
	"/api/v1/escalations/categories": "the service asks for escalation.view or escalation.manage",
	"/api/v1/escalations/list":       "the service asks for escalation.view, everyone's with escalation.list_all",
	"/api/v1/escalations/agents":     "the service asks for escalation.view",
	"/api/v1/escalations/":           "logging a call's reason; the service asks for escalation.view",
	"/api/v1/escalations/none":       "logging that a call had no reason; the service asks for escalation.view",
	"/api/v1/settings/":              "the service asks for system.settings",
	"/api/v1/settings/break-limit":   "every signed-in person reads the break limit; the break card needs it",
	"/api/v1/settings/real-call":     "every signed-in person reads the real-call threshold; the call screens label their figures with it",
	"/api/v1/webphone":               "the caller's own softphone",
	"/api/v1/sip/credentials":        "the caller's own phone login",
	"/api/v1/pbx/status":             "the caller's own presence",
	"/api/v1/pbx/stats":              "the caller's own numbers for today",
}

// alsoNeeds are routes whose service asks for more than the check next to
// the route: the request passes only when the user also holds one
// permission of every set.
var alsoNeeds = map[string][][]enums.Permission{
	"DELETE /api/v1/security/bans/:id": {anyOf(enums.SystemSettings)},
	"POST /api/v1/teams/groups":        {anyOf(enums.TeamsGroupCreate)},
}

// serviceGates are requests to routes checked inside their service, with
// the permissions that open them: the request passes when the user holds at
// least one permission from every set.
var serviceGates = []struct {
	method, path string
	need         [][]enums.Permission
}{
	{fiber.MethodGet, "/api/v1/settings/", [][]enums.Permission{anyOf(enums.SystemSettings)}},
	{fiber.MethodGet, "/api/v1/escalations/categories", [][]enums.Permission{anyOf(enums.EscalationView, enums.EscalationManage)}},
	{fiber.MethodGet, "/api/v1/wa/ai", [][]enums.Permission{anyOf(enums.WAView), anyOf(enums.WAAIManage)}},
	{fiber.MethodGet, "/api/v1/wa/call-survey", [][]enums.Permission{anyOf(enums.WAView), anyOf(enums.WACallSurvey)}},
	{fiber.MethodGet, "/api/v1/wa/rules", [][]enums.Permission{anyOf(enums.WAView), anyOf(enums.WAAutomation)}},
	{fiber.MethodGet, "/api/v1/wa/callbacks", [][]enums.Permission{anyOf(enums.WAView), anyOf(enums.WACallbacks)}},
	{fiber.MethodGet, "/api/v1/wa/ratings", [][]enums.Permission{anyOf(enums.WAView), anyOf(enums.WARatings)}},
	{fiber.MethodDelete, "/api/v1/wa/ratings/chat/999999999", [][]enums.Permission{anyOf(enums.WAView), anyOf(enums.WARatings), anyOf(enums.WARatingDelete)}},
	{fiber.MethodGet, "/api/v1/wa/rating-links", [][]enums.Permission{anyOf(enums.WAView), anyOf(enums.WARatings), anyOf(enums.WARatingLink)}},
	{fiber.MethodDelete, "/api/v1/wa/rating-links/999999999", [][]enums.Permission{anyOf(enums.WAView), anyOf(enums.WARatings), anyOf(enums.WARatingLink)}},
	{fiber.MethodGet, "/api/v1/wa/reports?from=2026-01-01&to=2026-01-31", [][]enums.Permission{anyOf(enums.WAView), anyOf(enums.WAReports)}},
}

func anyOf(p ...enums.Permission) []enums.Permission { return p }

// matrixRoute is one route of the server.
type matrixRoute struct {
	method, pattern string
}

func (r matrixRoute) key() string { return r.method + " " + r.pattern }

// path is a request path that matches the route.
func (r matrixRoute) path() string {
	parts := strings.Split(r.pattern, "/")
	for i, p := range parts {
		if strings.HasPrefix(p, ":") || p == "*" {
			parts[i] = probeID
		}
	}
	return strings.Join(parts, "/")
}

// serverRoutes lists the routes a signed-in person may call: every route
// of the app but the public ones and those left out above.
func serverRoutes(app *fiber.App) []matrixRoute {
	seen := map[string]bool{}
	var out []matrixRoute
	for _, r := range app.GetRoutes(true) {
		if r.Method == fiber.MethodHead || r.Method == fiber.MethodOptions {
			continue
		}
		mr := matrixRoute{r.Method, r.Path}
		if seen[mr.key()] {
			continue
		}
		seen[mr.key()] = true
		if _, ok := public[mr.key()]; ok {
			continue
		}
		if _, ok := notProbed[mr.key()]; ok {
			continue
		}
		out = append(out, mr)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].key() < out[j].key() })
	return out
}

// rolePermissions reads what a role holds from the database, which is what
// the server checks against.
func rolePermissions(t *testing.T, db *gorm.DB, role enums.Role) map[enums.Permission]bool {
	t.Helper()
	var keys []string
	if err := db.Raw(`SELECT p.key FROM permissions p JOIN role_permissions rp ON rp.permission_id = p.id
		JOIN roles r ON r.id = rp.role_id WHERE r.name = ?`, string(role)).Scan(&keys).Error; err != nil {
		t.Fatal(err)
	}
	held := make(map[enums.Permission]bool, len(keys))
	for _, k := range keys {
		held[enums.Permission(k)] = true
	}
	return held
}

// opens reports whether someone holding held passes every set.
func opens(held map[enums.Permission]bool, sets [][]enums.Permission) bool {
	for _, set := range sets {
		if !slices.ContainsFunc(set, func(p enums.Permission) bool { return held[p] }) {
			return false
		}
	}
	return true
}

// requireLog files the permission checks requests pass, by route and role.
type requireLog struct {
	mu   sync.Mutex
	seen map[string][]middlewares.RequireSeen
}

func (l *requireLog) add(c *fiber.Ctx, s middlewares.RequireSeen) {
	key := c.Get(probeHeader)
	if key == "" {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.seen[key] = append(l.seen[key], s)
}

func (l *requireLog) get(key string) []middlewares.RequireSeen {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.seen[key]
}

// TestPermissionMatrix calls every route the server has with every system
// role and with no role. A route with a permission check next to it must
// refuse (403) exactly the roles that lack the permission, and let the
// others through to an answer that is neither a refusal nor a failure of
// ours (5xx); bad input (400) or a missing row (404) is fine, the ids used
// exist nowhere. A route without such a check must belong to a group that
// checks inside its service, and must not fail with 5xx for anyone.
func TestPermissionMatrix(t *testing.T) {
	pbx := newFakePBX(t)
	newFakeMeta(t)
	srv, db := testServer(t, officeSecurity, pbx.use)
	log := &requireLog{seen: map[string][]middlewares.RequireSeen{}}
	t.Cleanup(middlewares.WatchRequire(log.add))
	routes := serverRoutes(srv.app)
	if len(routes) < 200 {
		t.Fatalf("only %d routes found; the route table was not read", len(routes))
	}

	// The invisible admin goes first: holding every permission, it passes
	// every check of a route, so the whole chain of checks is recorded.
	roles := []enums.Role{enums.RoleInvisibleAdmin, enums.RoleManager, enums.RoleTechnicalTeam, enums.RoleSalesTeam, ""}
	chains := map[string][][]enums.Permission{}
	var guarded, inside int
	for _, role := range roles {
		name := string(role)
		if name == "" {
			name = "no-role"
		}
		var b *browser
		held := map[enums.Permission]bool{}
		if role == "" {
			u := seedPeople(t, db, 1, enums.RoleSalesTeam, false)[0]
			db.Exec("DELETE FROM user_roles WHERE user_id = ?", u.ID)
			b = newBrowser(t, srv.app, nil, u)
		} else {
			b = newBrowser(t, srv.app, nil, seedPeople(t, db, 1, role, false)[0])
			held = rolePermissions(t, db, role)
		}
		b.mustSignIn()
		for _, r := range routes {
			probe := name + " " + r.key()
			var body any
			if r.method != fiber.MethodGet && r.method != fiber.MethodDelete {
				body = map[string]any{}
			}
			a := b.doWith(r.method, r.path(), body, http.Header{probeHeader: {probe}})
			seen := log.get(probe)
			if role == enums.RoleInvisibleAdmin {
				var chain [][]enums.Permission
				for _, s := range seen {
					chain = append(chain, s.Perms)
				}
				chains[r.key()] = chain
			}
			chain, hasChain := chains[r.key()]
			switch {
			case a.status == fiber.StatusUnauthorized:
				t.Errorf("%s: %s %s signed the caller out (401): %s", name, r.method, r.pattern, a.body)
			case a.status >= 500:
				t.Errorf("%s: %s %s failed with %d: %s", name, r.method, r.pattern, a.status, a.body)
			}
			if !hasChain || len(chain) == 0 {
				if role == enums.RoleInvisibleAdmin {
					inside++
					if !insideGroup(r.pattern) {
						t.Errorf("%s %s asks for no permission and is in no group that checks inside; guard it with need() or list it in checkedInside", r.method, r.pattern)
					}
				}
				continue
			}
			if role == enums.RoleInvisibleAdmin {
				guarded++
			}
			want := opens(held, chain)
			refused := slices.ContainsFunc(seen, func(s middlewares.RequireSeen) bool { return !s.Allowed })
			if refused != !want {
				t.Errorf("%s: %s %s: the route's check refused=%v, but the role %s %v", name, r.method, r.pattern, refused, holdsWord(want), chain)
			}
			if extra, ok := alsoNeeds[r.key()]; ok {
				want = want && opens(held, extra)
			}
			switch {
			case want && a.status == fiber.StatusForbidden:
				t.Errorf("%s: %s %s refused (403) although the role holds %v: %s", name, r.method, r.pattern, append(chain, alsoNeeds[r.key()]...), a.body)
			case !want && a.status != fiber.StatusForbidden:
				t.Errorf("%s: %s %s answered %d although the role lacks %v", name, r.method, r.pattern, a.status, append(chain, alsoNeeds[r.key()]...))
			}
		}

		// Requests outside every route pass the server-wide check for
		// webhooks registered in Meta untouched: not found, never a
		// refusal or a failure.
		for _, path := range []string{"/", "/webhook/whatsapp", "/some/page"} {
			for _, method := range []string{fiber.MethodGet, fiber.MethodPost} {
				if a := b.do(method, path, nil); a.status != fiber.StatusNotFound {
					t.Errorf("%s: %s %s outside every route answered %d, want 404", name, method, path, a.status)
				}
			}
		}

		// Routes checked inside their service.
		for _, g := range serviceGates {
			a := b.do(g.method, g.path, nil)
			want := opens(held, g.need)
			switch {
			case want && (a.status == fiber.StatusForbidden || a.status == fiber.StatusUnauthorized || a.status >= 500):
				t.Errorf("%s: %s %s answered %d although the role holds %v: %s", name, g.method, g.path, a.status, g.need, a.body)
			case !want && a.status != fiber.StatusForbidden:
				t.Errorf("%s: %s %s answered %d although the role lacks %v", name, g.method, g.path, a.status, g.need)
			}
		}
		if a := b.do(fiber.MethodGet, "/api/v1/auth/me", nil); a.status != fiber.StatusOK {
			t.Fatalf("%s: signed out by the matrix itself: %d", name, a.status)
		}
	}
	t.Logf("permission matrix: %d routes x %d roles; %d with a check next to the route, %d checked inside their service",
		len(routes), len(roles), guarded, inside)
	for key, why := range notProbed {
		if !slices.ContainsFunc(srv.app.GetRoutes(true), func(r fiber.Route) bool { return r.Method+" "+r.Path == key }) {
			t.Errorf("route %s (left out: %s) no longer exists; remove it from notProbed", key, why)
		}
	}
	for key := range alsoNeeds {
		if _, ok := chains[key]; !ok {
			t.Errorf("route %s in alsoNeeds no longer exists", key)
		}
	}
}

func insideGroup(pattern string) bool {
	if _, ok := checkedInside[pattern]; ok {
		return true
	}
	for prefix := range checkedInsideGroups {
		if strings.HasPrefix(pattern, prefix) || pattern == strings.TrimSuffix(prefix, "/") {
			return true
		}
	}
	return false
}

func holdsWord(holds bool) string {
	if holds {
		return "holds"
	}
	return "lacks"
}
