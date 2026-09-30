package middlewares

import (
	"errors"
	"log/slog"

	"github.com/gofiber/fiber/v2"

	"github.com/toprakgureli/santral-c/backend/pkg/errs"
)

// ErrorHandler renders an error as a typed JSON response and logs it: a
// request the client got wrong (4xx) as a warning, a failure of ours (5xx)
// as an error. The log line carries the request and user from the context.
func ErrorHandler(c *fiber.Ctx, err error) error {
	e := resolve(err)

	level := slog.LevelWarn
	if e.Status >= fiber.StatusInternalServerError {
		level = slog.LevelError
	}
	slog.Log(c.UserContext(), level, "request failed",
		"method", c.Method(),
		"path", c.Path(),
		"status", e.Status,
		"code", string(e.Code),
		"error", e.Err,
	)

	return c.Status(e.Status).JSON(fiber.Map{
		"code":    e.Code,
		"message": e.Message,
	})
}

func resolve(err error) *errs.Error {
	var e *errs.Error
	if errors.As(err, &e) {
		return e
	}
	var fe *fiber.Error
	if errors.As(err, &fe) {
		if fe.Code == fiber.StatusNotFound {
			return errs.NotFound("İstenen kaynak bulunamadı.")
		}
		return errs.New(errs.CodeInvalid, fe.Code, fe.Message, nil)
	}
	return errs.Internal(err)
}
