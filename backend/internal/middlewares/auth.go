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
	"github.com/toprakgureli/santral-c/backend/pkg/logctx"
)

// IDenylist checks token and per-user revocation.
type IDenylist interface {
	Has(ctx context.Context, id string) (bool, error)
	UserRevokedAt(ctx context.Context, userID uint) (time.Time, error)
}

const (
	// UserIDKey is the Fiber locals key holding the authenticated user id.
	UserIDKey string = "userId"
	// SessionKey is the Fiber locals key holding the request's *Session.
	SessionKey string = "session"
)

var errSessionEnded = errs.Unauthorized("Oturumunuz sonlandırılmış. Lütfen tekrar giriş yapın.")

// Session is the signed-in caller of a request. Responses that stay open
// (event streams) keep checking it, because the session can be revoked
// after the request was accepted.
type Session struct {
	UserID   uint
	tokenID  string
	issuedAt time.Time
	list     IDenylist
}

// Check returns nil while the session is still valid, and an error once its
// token was revoked or the user's sessions were ended.
func (s *Session) Check(ctx context.Context) error {
	revoked, err := s.list.Has(ctx, s.tokenID)
	if err != nil {
		return errs.Internal(err)
	}
	if revoked {
		return errSessionEnded
	}
	cutoff, err := s.list.UserRevokedAt(ctx, s.UserID)
	if err != nil {
		return errs.Internal(err)
	}
	if !cutoff.IsZero() && s.issuedAt.Before(cutoff) {
		return errSessionEnded
	}
	return nil
}

// SessionFrom returns the session the Auth middleware attached, or nil on
// routes it does not guard.
func SessionFrom(c *fiber.Ctx) *Session {
	s, _ := c.Locals(SessionKey).(*Session)
	return s
}

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

		session := &Session{UserID: claims.UserID, tokenID: claims.ID, list: list}
		if claims.IssuedAt != nil {
			session.issuedAt = claims.IssuedAt.Time
		}
		if err := session.Check(c.UserContext()); err != nil {
			return err
		}

		c.Locals(UserIDKey, claims.UserID)
		c.Locals(SessionKey, session)
		c.SetUserContext(logctx.WithUser(c.UserContext(), claims.UserID))
		return c.Next()
	}
}
