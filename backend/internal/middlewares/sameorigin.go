package middlewares

import (
	"mime"
	"net/http"
	"net/url"
	"path"
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/toprakgureli/santral-c/backend/pkg/errs"
)

// SameOriginConfig says what the panel's own requests look like.
type SameOriginConfig struct {
	// Origins are the addresses the panel is served from (scheme://host),
	// besides the address the request itself was sent to.
	Origins []string
	// Cookies name the session cookies. A request carrying one of them is
	// checked; so is every request under GuardedPrefix.
	Cookies []string
	// GuardedPrefix is checked with or without a session cookie: the sign-in
	// steps, which set the cookies.
	GuardedPrefix string
	// Multipart are the routes (path.Match patterns) that take a file
	// upload as multipart/form-data.
	Multipart []string
}

var (
	errForeignOrigin = errs.Forbidden("Bu istek panelin kendi sayfasından gelmedi; işlem yapılmadı.")
	errBodyType      = errs.New("UNSUPPORTED_MEDIA_TYPE", http.StatusUnsupportedMediaType,
		"İstek gövdesi JSON olmalı.", nil)
)

// SameOrigin turns away a state-changing request that a browser sent from
// another site while the panel's session cookies rode along. Cookies alone
// cannot tell: a page on a neighbouring address counts as the same site,
// so its requests carry them. Two checks close that:
//
//   - The browser says where the request comes from (Origin, or
//     Sec-Fetch-Site). It must be the panel's own address. A client that is
//     not a browser sends neither and passes; it cannot borrow anyone's
//     cookies.
//   - The body is JSON, which a plain form on another site cannot send; a
//     file upload may be multipart, on the routes listed for it.
//
// Requests without the session cookies (the webhooks, a bearer token) are
// left alone, except the sign-in steps under GuardedPrefix.
func SameOrigin(cfg SameOriginConfig) fiber.Handler {
	allowed := make(map[string]bool, len(cfg.Origins))
	for _, o := range cfg.Origins {
		if n := normalOrigin(o); n != "" {
			allowed[n] = true
		}
	}
	return func(c *fiber.Ctx) error {
		switch c.Method() {
		case fiber.MethodGet, fiber.MethodHead, fiber.MethodOptions:
			return c.Next()
		}
		if !hasAnyCookie(c, cfg.Cookies) && (cfg.GuardedPrefix == "" || !strings.HasPrefix(c.Path(), cfg.GuardedPrefix)) {
			return c.Next()
		}
		if !fromPanel(c, allowed) {
			return errForeignOrigin
		}
		if !bodyAccepted(c, cfg.Multipart) {
			return errBodyType
		}
		return c.Next()
	}
}

func hasAnyCookie(c *fiber.Ctx, names []string) bool {
	for _, n := range names {
		if n != "" && c.Cookies(n) != "" {
			return true
		}
	}
	return false
}

// fromPanel reports whether the browser says the request comes from the
// panel's own pages.
func fromPanel(c *fiber.Ctx, allowed map[string]bool) bool {
	origin := strings.TrimSpace(c.Get(fiber.HeaderOrigin))
	site := strings.ToLower(strings.TrimSpace(c.Get("Sec-Fetch-Site")))
	if origin == "" {
		// Older browsers leave Origin out of some same-origin requests but
		// still say where they come from here.
		return site == "" || site == "same-origin"
	}
	n := normalOrigin(origin)
	if n == "" {
		// "null": a sandboxed frame or a local file.
		return false
	}
	if allowed[n] {
		return true
	}
	// The address the browser sent the request to (nginx passes it on).
	u, _ := url.Parse(n)
	host := strings.ToLower(string(c.Request().Host()))
	return u != nil && host != "" && u.Host == host
}

// normalOrigin reduces an address to scheme://host[:port] in lower case, or
// "" when it is not one.
func normalOrigin(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return ""
	}
	host := strings.ToLower(u.Host)
	switch {
	case u.Scheme == "https" && strings.HasSuffix(host, ":443"):
		host = strings.TrimSuffix(host, ":443")
	case u.Scheme == "http" && strings.HasSuffix(host, ":80"):
		host = strings.TrimSuffix(host, ":80")
	}
	return u.Scheme + "://" + host
}

// bodyAccepted reports whether the body type is one a cross-site form
// cannot produce: JSON, nothing at all, or a file upload on a route that
// takes one.
func bodyAccepted(c *fiber.Ctx, multipart []string) bool {
	raw := c.Get(fiber.HeaderContentType)
	if strings.TrimSpace(raw) == "" {
		return len(c.Body()) == 0
	}
	media, _, err := mime.ParseMediaType(raw)
	if err != nil {
		return false
	}
	switch media {
	case fiber.MIMEApplicationJSON:
		return true
	case fiber.MIMEMultipartForm:
		route := strings.TrimSuffix(strings.ToLower(c.Path()), "/")
		for _, p := range multipart {
			if ok, _ := path.Match(p, route); ok {
				return true
			}
		}
	}
	return false
}
