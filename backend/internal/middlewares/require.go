package middlewares

import (
	"context"

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
		for _, p := range perms {
			if actor.Can(p) {
				return c.Next()
			}
		}
		return errs.Forbidden("Bu işlem için yetkin yok.")
	}
}
