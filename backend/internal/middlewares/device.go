package middlewares

import (
	"crypto/rand"
	"encoding/base64"
	"time"

	"github.com/gofiber/fiber/v2"
)

const (
	// DeviceCookie names the cookie that tells browsers apart, so wrong
	// passwords are counted per browser rather than per office address.
	DeviceCookie = "santral_device"
	// DeviceKey is the Fiber locals key holding the browser's id.
	DeviceKey = "device"
)

// Device gives every browser a random id in a long-lived cookie and puts
// it in the request's locals. It tells browsers apart; it proves nothing
// about who uses them.
func Device(secure bool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id := c.Cookies(DeviceCookie)
		if !validDevice(id) {
			id = newDevice()
			c.Cookie(&fiber.Cookie{
				Name:     DeviceCookie,
				Value:    id,
				Expires:  time.Now().AddDate(1, 0, 0),
				Path:     "/",
				HTTPOnly: true,
				Secure:   secure,
				SameSite: "Lax",
			})
		}
		c.Locals(DeviceKey, id)
		return c.Next()
	}
}

// DeviceFrom returns the browser's id, or "" outside Device.
func DeviceFrom(c *fiber.Ctx) string {
	id, _ := c.Locals(DeviceKey).(string)
	return id
}

func newDevice() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func validDevice(id string) bool {
	if len(id) != 22 {
		return false
	}
	for _, r := range id {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
			return false
		}
	}
	return true
}
