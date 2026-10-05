package main

import (
	"strconv"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
	"github.com/toprakgureli/santral-c/backend/pkg/enums"
)

// TestHiddenAdminWhoWorksInARoleCanBeShownInStats: an invisible admin who
// also works in another role stays out of the statistics until that role
// gets performance.show_hidden_admin; then colleagues see their team
// performance row, escalation filter entry and profile. The rest of the
// hiding (audit trail, chat people) does not change, and an invisible admin
// without such a role stays hidden.
func TestHiddenAdminWhoWorksInARoleCanBeShownInStats(t *testing.T) {
	srv, db := testServer(t, officeSecurity)
	admin := systemRole(t, db, enums.RoleInvisibleAdmin)
	work := customRole(t, db, enums.CallViewOwn)
	watcher := customRole(t, db, enums.SystemAuditView, enums.TeamsView, enums.PerformanceViewAll, enums.EscalationListAll)
	worker := person(t, db, "Çalışan sahip", true, admin, work)
	owner := person(t, db, "Sadece sahip", true, admin)
	viewer := person(t, db, "Bakan", false, watcher)

	var escs []models.CallEscalation
	for _, u := range []models.User{worker, owner} {
		id := u.ID
		escs = append(escs, models.CallEscalation{NumberKey: "5550001133", Number: "05550001133", CategoryName: "Deneme", ReasonName: "Deneme", AgentID: &id, AgentName: u.Name, CreatedAt: time.Now()})
	}
	if err := db.Create(&escs).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Exec("DELETE FROM call_escalations WHERE id IN ?", []uint{escs[0].ID, escs[1].ID}) })

	vb := signInAs(t, srv, viewer)
	type look struct {
		name string
		path func(models.User) string
		seen func(a answer, u models.User) bool
	}
	stats := []look{
		{"team performance", func(models.User) string { return "/api/v1/performance/today" }, func(a answer, u models.User) bool { return idsIn(t, a, "userId")[u.ID] }},
		{"escalation agents", func(models.User) string { return "/api/v1/escalations/agents" }, func(a answer, u models.User) bool { return idsIn(t, a, "id")[u.ID] }},
		{"profile", func(u models.User) string { return "/api/v1/profile/" + strconv.FormatUint(uint64(u.ID), 10) }, func(a answer, _ models.User) bool { return a.status == fiber.StatusOK }},
	}
	check := func(stage string, u models.User, want bool) {
		t.Helper()
		for _, l := range stats {
			if got := l.seen(vb.do(fiber.MethodGet, l.path(u), nil), u); got != want {
				t.Errorf("%s: %s of %s seen = %v, want %v", stage, l.name, u.Name, got, want)
			}
		}
	}
	check("before", worker, false)
	check("before", owner, false)

	if err := db.Exec("INSERT INTO role_permissions (role_id, permission_id) SELECT ?, id FROM permissions WHERE key = ?",
		work.ID, string(enums.PerformanceShowHiddenAdmin)).Error; err != nil {
		t.Fatal(err)
	}
	check("after", worker, true)
	check("after", owner, false)

	// Only the statistics open up: the chat's people and the audit trail
	// keep the worker out as before.
	if idsIn(t, vb.do(fiber.MethodGet, "/api/v1/teams/people", nil), "id")[worker.ID] {
		t.Error("the chat's people list shows the invisible admin")
	}
	if a := vb.do(fiber.MethodGet, "/api/v1/audit?query="+worker.Email, nil); totalOf(t, a) > 0 {
		t.Error("the audit trail shows the invisible admin")
	}
}
