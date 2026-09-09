package middlewares

import (
	"fmt"
	"log/slog"

	"github.com/gofiber/fiber/v2"

	"github.com/toprakgureli/santral-c/backend/pkg/errs"
)

// Recover turns a panic into a safe 500 without leaking the panic value.
func Recover() fiber.Handler {
	return func(c *fiber.Ctx) (err error) {
		defer func() {
			r := recover()
			if r == nil {
				return
			}
			slog.Error("panic recovered", "method", c.Method(), "path", c.Path())
			err = errs.Internal(fmt.Errorf("panic: %v", r))
		}()
		return c.Next()
	}
}
