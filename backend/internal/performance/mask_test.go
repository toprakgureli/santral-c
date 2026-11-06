package performance

import (
	"context"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
	"github.com/toprakgureli/santral-c/backend/internal/middlewares"
	"github.com/toprakgureli/santral-c/backend/internal/testdb"
	"github.com/toprakgureli/santral-c/backend/pkg/enums"
	"github.com/toprakgureli/santral-c/backend/pkg/tz"
)

func TestMaskNumber(t *testing.T) {
	tests := map[string]string{
		"05551234567":    "•••••••••67",
		"+90 555 123 45": "+•• ••• ••• 45",
		"1014":           "1014",
		"":               "",
	}
	for in, want := range tests {
		if got := MaskNumber(in); got != want {
			t.Errorf("MaskNumber(%q) = %q, want %q", in, got, want)
		}
	}
}

type mapActors map[uint]*models.User

func (m mapActors) GetByID(_ context.Context, id uint) (*models.User, error) {
	return m[id], nil
}

func TestRouterRequiresRecentCallsPermission(t *testing.T) {
	without := &models.User{
		ID:     1,
		Active: true,
		Roles: []models.Role{{
			Name:        "viewer_no_recent",
			Permissions: []models.Permission{{Key: string(enums.PerformanceViewAll)}},
		}},
	}
	with := &models.User{
		ID:     2,
		Active: true,
		Roles: []models.Role{{
			Name: "viewer_with_recent",
			Permissions: []models.Permission{
				{Key: string(enums.PerformanceViewAll)},
				{Key: string(enums.PerformanceRecentCalls)},
			},
		}},
	}
	actors := mapActors{1: without, 2: with}
	svc := NewService(nil, actors, nil)
	need := func(perms ...enums.Permission) fiber.Handler {
		return middlewares.Require(actors, perms...)
	}

	today := time.Now().In(tz.Istanbul).Format("2006-01-02")
	if _, err := svc.AgentCalls(context.Background(), without.ID, 10, today, today); err == nil {
		t.Fatal("AgentCalls without performance.recent_calls succeeded, want forbidden error")
	}

	callRoute := func(uid uint, path string) int {
		app := fiber.New(fiber.Config{ErrorHandler: middlewares.ErrorHandler})
		guard := func(c *fiber.Ctx) error {
			c.Locals(middlewares.UserIDKey, uid)
			return c.Next()
		}
		NewRouter(NewHandler(svc), guard, need).Routes(app)
		res, err := app.Test(httptest.NewRequest(fiber.MethodGet, path, nil))
		if err != nil {
			t.Fatal(err)
		}
		return res.StatusCode
	}

	if got := callRoute(without.ID, "/performance/calls?userId=0"); got != fiber.StatusForbidden {
		t.Fatalf("without performance.recent_calls: status = %d, want 403", got)
	}
	// With performance.recent_calls, the route passes the permission middleware and reaches the handler (userId=0 -> 400).
	if got := callRoute(with.ID, "/performance/calls?userId=0"); got != fiber.StatusBadRequest {
		t.Fatalf("with performance.recent_calls: status = %d, want 400 from handler", got)
	}
}

func TestRecentCallsPermission(t *testing.T) {
	db := testdb.Open(t)
	ctx := context.Background()
	stamp := time.Now().UnixNano()
	ext := fmt.Sprintf("9%03d", stamp%1000)
	agent := models.User{
		Name:         "Temsilci",
		Email:        fmt.Sprintf("perf-rc-%d@test.local", stamp),
		Password:     "x",
		Active:       true,
		SIPExtension: &ext,
	}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.Exec("DELETE FROM call_logs WHERE user_id = ?", agent.ID)
		db.Exec("DELETE FROM users WHERE id = ?", agent.ID)
	})

	now := time.Now().Add(-5 * time.Minute)
	if err := db.Exec(`INSERT INTO call_logs (call_id, user_id, direction, peer_number, peer_key, disposition, started_at, answered_at, ended_at, duration_seconds, hooks_done)
		VALUES (?, ?, 'outbound', '05551234567', '5551234567', 'answered', ?, ?, ?, 40, true)`,
		fmt.Sprintf("perf-rc-%d", stamp), agent.ID, now, now.Add(2*time.Second), now.Add(42*time.Second)).Error; err != nil {
		t.Fatal(err)
	}

	withoutActor := &models.User{
		ID:     agent.ID + 100000,
		Active: true,
		Roles: []models.Role{{
			Name:        "viewer_no_recent",
			Permissions: []models.Permission{{Key: string(enums.PerformanceViewAll)}},
		}},
	}
	withActor := &models.User{
		ID:     agent.ID + 100001,
		Active: true,
		Roles: []models.Role{{
			Name: "viewer_with_recent",
			Permissions: []models.Permission{
				{Key: string(enums.PerformanceViewAll)},
				{Key: string(enums.PerformanceRecentCalls)},
			},
		}},
	}

	svc := NewService(NewRepository(db), mapActors{
		withoutActor.ID: withoutActor,
		withActor.ID:    withActor,
	}, nil)

	today := time.Now().In(tz.Istanbul).Format("2006-01-02")

	teamWithout, err := svc.Range(ctx, withoutActor.ID, today, today)
	if err != nil {
		t.Fatalf("Range without permission: %v", err)
	}
	for _, r := range teamWithout.Items {
		if r.UserID == agent.ID && len(r.Recent) != 0 {
			t.Fatalf("without performance.recent_calls got %d recent calls, want 0", len(r.Recent))
		}
	}
	if _, err := svc.AgentCalls(ctx, withoutActor.ID, agent.ID, today, today); err == nil {
		t.Fatal("AgentCalls without performance.recent_calls succeeded, want forbidden error")
	}

	teamWith, err := svc.Range(ctx, withActor.ID, today, today)
	if err != nil {
		t.Fatalf("Range with permission: %v", err)
	}
	found := false
	for _, r := range teamWith.Items {
		if r.UserID == agent.ID {
			found = len(r.Recent) == 1
		}
	}
	if !found {
		t.Fatal("with performance.recent_calls did not return the agent's recent call")
	}
	calls, err := svc.AgentCalls(ctx, withActor.ID, agent.ID, today, today)
	if err != nil {
		t.Fatalf("AgentCalls with permission: %v", err)
	}
	if len(calls.Items) != 1 {
		t.Fatalf("AgentCalls got %d items, want 1", len(calls.Items))
	}
}
