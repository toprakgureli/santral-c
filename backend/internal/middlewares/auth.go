// Package middlewares holds shared Fiber middleware.
package middlewares

import (
	"context"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/toprakgureli/santral-c/backend/configs"
	"github.com/toprakgureli/santral-c/backend/pkg/errs"
	"github.com/toprakgureli/santral-c/backend/pkg/jwt"
)

// IDenylist checks token and per-user revocation.
type IDenylist interface {
	Has(ctx context.Context, id string) (bool, error)
	UserRevokedAt(ctx context.Context, userID uint) (time.Time, error)
}

// UserIDKey is the Fiber locals key holding the authenticated user id.
const UserIDKey string = "userId"

// Auth authenticates a request from the access cookie or bearer header.
func Auth(cfg configs.Auth, list IDenylist) fiber.Handler {
	return func(c *fiber.Ctx) error {
		token := c.Cookies(cfg.CookieName)
		if token == "" {
			header := c.Get(fiber.HeaderAuthorization)
			if strings.HasPrefix(header, "Bearer ") {
				token = strings.TrimPrefix(header, "Bearer ")
			}
		}
		if token == "" {
			return errs.Unauthorized("Oturum bulunamadı. Lütfen giriş yapın.")
		}

		claims, err := jwt.Parse(cfg, token)
		if err != nil {
			return errs.Unauthorized("Oturumunuz geçersiz veya süresi dolmuş.")
		}
		if claims.Purpose != jwt.PurposeAccess {
			return errs.Unauthorized("Bu token API erişimi için geçerli değil.")
		}

		revoked, err := list.Has(c.UserContext(), claims.ID)
		if err != nil {
			return errs.Internal(err)
		}
		if revoked {
			return errs.Unauthorized("Oturumunuz sonlandırılmış. Lütfen tekrar giriş yapın.")
		}

		revokedAt, err := list.UserRevokedAt(c.UserContext(), claims.UserID)
		if err != nil {
			return errs.Internal(err)
		}
		if !revokedAt.IsZero() && claims.IssuedAt != nil && claims.IssuedAt.Before(revokedAt) {
			return errs.Unauthorized("Oturumunuz sonlandırılmış. Lütfen tekrar giriş yapın.")
		}

		c.Locals(UserIDKey, claims.UserID)
		return c.Next()
	}
}
