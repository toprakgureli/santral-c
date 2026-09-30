package middlewares

import (
	"github.com/gofiber/fiber/v2"

	"github.com/toprakgureli/santral-c/backend/pkg/logctx"
)

// requestIDKey is where Fiber's requestid middleware leaves the id.
const requestIDKey = "requestid"

// RequestContext puts the request id into the request's context, so every
// log line written while serving the request carries it. It must come
// right after Fiber's requestid middleware, which also sends the id back
// in X-Request-ID for a user to quote when reporting a problem.
func RequestContext() fiber.Handler {
	return func(c *fiber.Ctx) error {
		if id, ok := c.Locals(requestIDKey).(string); ok {
			c.SetUserContext(logctx.WithRequest(c.UserContext(), id))
		}
		return c.Next()
	}
}
