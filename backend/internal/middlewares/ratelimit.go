package middlewares

import (
	"crypto/sha256"
	"encoding/hex"
	"net"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/limiter"

	"github.com/toprakgureli/santral-c/backend/configs"
	"github.com/toprakgureli/santral-c/backend/pkg/errs"
)

// RateLimit limits requests per client IP within a window. It suits
// addresses outsiders call (webhooks, survey answers); the panel's own
// routes are limited per browser or per session instead, because a whole
// office shares one address.
func RateLimit(max int, window time.Duration) fiber.Handler {
	return RateLimitBy(max, window, func(c *fiber.Ctx) string { return "ip:" + c.IP() })
}

// RateLimitBy limits requests per key within a window.
func RateLimitBy(max int, window time.Duration, key func(c *fiber.Ctx) string) fiber.Handler {
	return limiter.New(limiter.Config{
		Max:          max,
		Expiration:   window,
		KeyGenerator: key,
		LimitReached: func(c *fiber.Ctx) error {
			return errs.TooMany("Çok fazla istek gönderildi. Lütfen biraz bekle.")
		},
	})
}

// RateLimitOutside limits requests per address, leaving the trusted
// addresses (the office) out. IPv6 addresses count per /64, the block one
// connection usually holds, so a sender cannot step around the limit by
// changing the last part of its address.
func RateLimitOutside(max int, window time.Duration, trusted []*net.IPNet) fiber.Handler {
	return limiter.New(limiter.Config{
		Max:        max,
		Expiration: window,
		Next:       func(c *fiber.Ctx) bool { return configs.Contains(trusted, c.IP()) },
		KeyGenerator: func(c *fiber.Ctx) string {
			return "out:" + addressGroup(c.IP())
		},
		LimitReached: func(c *fiber.Ctx) error {
			return errs.TooMany("Çok fazla istek gönderildi. Lütfen biraz bekle.")
		},
	})
}

// addressGroup is an IPv4 address itself, or the /64 an IPv6 address
// belongs to.
func addressGroup(ip string) string {
	parsed := net.ParseIP(ip)
	switch {
	case parsed == nil:
		return ip
	case parsed.To4() != nil:
		return parsed.To4().String()
	}
	return parsed.Mask(net.CIDRMask(64, 128)).String() + "/64"
}

// PerDevice keys a limit by the browser's device cookie (see Device), or
// by address when there is none.
func PerDevice(c *fiber.Ctx) string {
	if d := DeviceFrom(c); d != "" {
		return "dev:" + d
	}
	return "ip:" + c.IP()
}

// PerCookie keys a limit by a cookie's value, hashed, falling back to the
// browser; one session renewing too often is held back, not the office.
func PerCookie(name string) func(c *fiber.Ctx) string {
	return func(c *fiber.Ctx) string {
		if v := c.Cookies(name); v != "" {
			sum := sha256.Sum256([]byte(v))
			return "cookie:" + hex.EncodeToString(sum[:8])
		}
		return PerDevice(c)
	}
}
