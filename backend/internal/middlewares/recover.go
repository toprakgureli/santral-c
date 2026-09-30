package middlewares

import (
	"fmt"
	"log/slog"
	"runtime/debug"

	"github.com/gofiber/fiber/v2"

	"github.com/toprakgureli/santral-c/backend/pkg/errs"
)

// Recover turns a panic into a safe 500 without showing the panic to the
// client, and logs it with its stack so it can be fixed.
func Recover() fiber.Handler {
	return func(c *fiber.Ctx) (err error) {
		defer func() {
			r := recover()
			if r == nil {
				return
			}
			slog.ErrorContext(c.UserContext(), "panic recovered",
				"method", c.Method(),
				"path", c.Path(),
				"panic", fmt.Sprint(r),
				"stack", string(debug.Stack()),
			)
			err = errs.Internal(fmt.Errorf("panic: %v", r))
		}()
		return c.Next()
	}
}
