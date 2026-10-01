package middlewares

import (
	"fmt"
	"strings"

	"github.com/gofiber/fiber/v2"
)

// inlineTypes are the file types the panel may show inside the page:
// pictures, sound, video and PDF. Anything else, whatever type the uploader
// or the sender claimed, is only ever downloaded, so an HTML or SVG file can
// never run as a page on the panel's own address.
var inlineTypes = map[string]bool{
	"image/jpeg": true, "image/png": true, "image/webp": true, "image/gif": true,
	"video/mp4": true, "video/3gpp": true, "video/webm": true, "video/quicktime": true,
	"audio/ogg": true, "audio/mpeg": true, "audio/mp4": true, "audio/aac": true, "audio/amr": true,
	"audio/wav": true, "audio/webm": true, "audio/x-m4a": true,
	"application/pdf": true,
}

// baseType is a media type without its parameters, in lower case.
func baseType(mime string) string {
	return strings.ToLower(strings.TrimSpace(strings.SplitN(mime, ";", 2)[0]))
}

// Inlinable reports whether a file of this type may be shown inside the page.
func Inlinable(mime string) bool {
	return inlineTypes[baseType(mime)]
}

// FileHeaders sets the headers for sending a stored file to the browser. A
// type outside the safe list is sent as plain bytes to save. Everything
// gets nosniff, and what is shown inline gets a policy that lets it run no
// script and load nothing else (PDF excepted, whose viewer would refuse it).
func FileHeaders(c *fiber.Ctx, mime, name string, download bool) {
	t := baseType(mime)
	inline := !download && inlineTypes[t]
	if !inlineTypes[t] {
		t = "application/octet-stream"
	}
	c.Set(fiber.HeaderContentType, t)
	c.Set(fiber.HeaderXContentTypeOptions, "nosniff")
	if t != "application/pdf" {
		c.Set(fiber.HeaderContentSecurityPolicy, "default-src 'none'; img-src 'self'; media-src 'self'; style-src 'unsafe-inline'; sandbox")
	}
	disposition := "attachment"
	if inline {
		disposition = "inline"
	}
	if name == "" {
		c.Set(fiber.HeaderContentDisposition, disposition)
		return
	}
	c.Set(fiber.HeaderContentDisposition, fmt.Sprintf("%s; filename*=UTF-8''%s", disposition, nameEscape(name)))
}

// nameEscape percent-encodes everything but letters, digits and . - _ in a
// file name, as a filename* value requires.
func nameEscape(s string) string {
	const hex = "0123456789ABCDEF"
	out := make([]byte, 0, len(s)*3)
	for i := 0; i < len(s); i++ {
		b := s[i]
		if (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b == '.' || b == '-' || b == '_' {
			out = append(out, b)
			continue
		}
		out = append(out, '%', hex[b>>4], hex[b&15])
	}
	return string(out)
}
