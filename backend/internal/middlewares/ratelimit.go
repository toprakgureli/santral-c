package middlewares

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/limiter"

	"github.com/toprakgureli/santral-c/backend/pkg/errs"
)

// RateLimit limits requests per client IP within a window.
func RateLimit(max int, window time.Duration) fiber.Handler {
	return limiter.New(limiter.Config{
		Max:        max,
		Expiration: window,
		KeyGenerator: func(c *fiber.Ctx) string {
			return c.IP()
		},
		LimitReached: func(c *fiber.Ctx) error {
			return errs.TooMany("Çok fazla istek gönderildi. Lütfen biraz bekleyin.")
		},
	})
}
