package middlewares

import (
	"context"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
	"github.com/toprakgureli/santral-c/backend/pkg/enums"
)

type oneActor struct{ perms []enums.Permission }

func (a oneActor) GetByID(_ context.Context, id uint) (*models.User, error) {
	role := models.Role{Name: "test"}
	for _, p := range a.perms {
		role.Permissions = append(role.Permissions, models.Permission{Key: string(p)})
	}
	return &models.User{ID: id, Roles: []models.Role{role}}, nil
}

func TestRequire(t *testing.T) {
	tests := []struct {
		name string
		held []enums.Permission
		need []enums.Permission
		want int
	}{
		{"holds the one asked for", []enums.Permission{enums.UserView}, []enums.Permission{enums.UserView}, fiber.StatusOK},
		{"holds one of several", []enums.Permission{enums.RoleAssign}, []enums.Permission{enums.RoleView, enums.RoleAssign}, fiber.StatusOK},
		{"holds another", []enums.Permission{enums.ContactView}, []enums.Permission{enums.UserView}, fiber.StatusForbidden},
		{"holds nothing", nil, []enums.Permission{enums.UserView}, fiber.StatusForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var seen []RequireSeen
			stop := WatchRequire(func(_ *fiber.Ctx, s RequireSeen) { seen = append(seen, s) })
			defer stop()

			app := fiber.New(fiber.Config{ErrorHandler: ErrorHandler})
			app.Get("/", func(c *fiber.Ctx) error {
				c.Locals(UserIDKey, uint(7))
				return c.Next()
			}, Require(oneActor{perms: tt.held}, tt.need...), func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) })
			res, err := app.Test(httptest.NewRequest(fiber.MethodGet, "/", nil))
			if err != nil {
				t.Fatal(err)
			}
			if res.StatusCode != tt.want {
				t.Fatalf("status = %d, want %d", res.StatusCode, tt.want)
			}
			if len(seen) != 1 || !slices.Equal(seen[0].Perms, tt.need) || seen[0].Allowed != (tt.want == fiber.StatusOK) {
				t.Fatalf("watch saw %+v", seen)
			}
		})
	}
}

func TestRequireWithoutSession(t *testing.T) {
	app := fiber.New(fiber.Config{ErrorHandler: ErrorHandler})
	app.Get("/", Require(oneActor{}, enums.UserView), func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) })
	res, err := app.Test(httptest.NewRequest(fiber.MethodGet, "/", nil))
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", res.StatusCode)
	}
}

func TestWatchRequireStops(t *testing.T) {
	calls := 0
	stop := WatchRequire(func(*fiber.Ctx, RequireSeen) { calls++ })
	stop()
	app := fiber.New()
	app.Get("/", func(c *fiber.Ctx) error {
		c.Locals(UserIDKey, uint(1))
		return c.Next()
	}, Require(oneActor{perms: []enums.Permission{enums.UserView}}, enums.UserView), func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) })
	if _, err := app.Test(httptest.NewRequest(fiber.MethodGet, "/", nil)); err != nil {
		t.Fatal(err)
	}
	if calls != 0 {
		t.Fatalf("a stopped watch was told %d times", calls)
	}
}
