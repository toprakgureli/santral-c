package middlewares

import (
	"context"
	"sync/atomic"

	"github.com/gofiber/fiber/v2"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
	"github.com/toprakgureli/santral-c/backend/pkg/enums"
	"github.com/toprakgureli/santral-c/backend/pkg/errs"
)

// IActorLoader loads the user behind a request.
type IActorLoader interface {
	GetByID(ctx context.Context, id uint) (*models.User, error)
}

// Requirer builds the check for a route's permissions.
type Requirer func(perms ...enums.Permission) fiber.Handler

// NewRequirer returns a Requirer that loads the acting user with actors.
func NewRequirer(actors IActorLoader) Requirer {
	return func(perms ...enums.Permission) fiber.Handler {
		return Require(actors, perms...)
	}
}

// RequireSeen is what one permission check of a request found: the
// permissions the route asks for (any one of them opens it) and whether the
// user held one.
type RequireSeen struct {
	Perms   []enums.Permission
	Allowed bool
}

// requireWatch is told about every permission check while it is set.
var requireWatch atomic.Pointer[func(c *fiber.Ctx, seen RequireSeen)]

// WatchRequire has fn told about every permission check Require makes,
// until the returned function is called. The route tests use it to read
// which permissions each route asks for straight from the running server.
// It changes nothing about the answers.
func WatchRequire(fn func(c *fiber.Ctx, seen RequireSeen)) (stop func()) {
	requireWatch.Store(&fn)
	return func() { requireWatch.Store(nil) }
}

// Require lets a request through only when the signed-in user holds at
// least one of perms. It shows next to each route what the route needs; the
// service behind it checks again with its finer rules. It must come after
// the Auth middleware.
func Require(actors IActorLoader, perms ...enums.Permission) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id, ok := c.Locals(UserIDKey).(uint)
		if !ok {
			return errs.Unauthorized("Oturum bulunamadı. Lütfen giriş yap.")
		}
		actor, err := actors.GetByID(c.UserContext(), id)
		if err != nil {
			return err
		}
		allowed := false
		for _, p := range perms {
			if actor.Can(p) {
				allowed = true
				break
			}
		}
		if watch := requireWatch.Load(); watch != nil {
			(*watch)(c, RequireSeen{Perms: perms, Allowed: allowed})
		}
		if allowed {
			return c.Next()
		}
		return errs.Forbidden("Bu işlem için yetkin yok.")
	}
}
