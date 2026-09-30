package middlewares

import (
	"github.com/gofiber/fiber/v2"

	"github.com/toprakgureli/santral-c/backend/pkg/errs"
)

// MaxBody rejects a request whose body is larger than limit bytes. The
// server-wide limit is sized for file uploads; endpoints that only take small
// notices use this to refuse anything bigger before it is parsed.
func MaxBody(limit int) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if c.Request().Header.ContentLength() > limit || len(c.Body()) > limit {
			return errs.TooLarge("İstek çok büyük.")
		}
		return c.Next()
	}
}
