// Package sse writes Server-Sent Event streams for the panel's live views
// (the chat, WhatsApp and the agent list). Every stream sends a first
// message, forwards events as they come, keeps the connection alive through
// nginx with a comment line, and closes itself once the caller's session is
// revoked, so a deactivated user stops receiving data right away.
package sse

import (
	"bufio"
	"context"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/toprakgureli/santral-c/backend/internal/middlewares"
)

// Heartbeat is how often an idle stream sends a comment line. It must stay
// under nginx's proxy_read_timeout (30 s) so the connection is not cut, and
// it is also how often the session is re-checked.
const Heartbeat = 20 * time.Second

// Stream is one open event stream.
type Stream struct {
	// First is written as the first event; nil sends nothing.
	First []byte
	// Events delivers the stream's events; a closed channel ends the stream.
	Events <-chan []byte
	// Close runs once when the stream ends, whatever the reason.
	Close func()
}

// Serve sets the event-stream headers and streams s to the client after the
// handler returns. It must be the last thing a handler does.
func Serve(c *fiber.Ctx, s Stream) error {
	session := middlewares.SessionFrom(c)
	c.Set(fiber.HeaderContentType, "text/event-stream")
	c.Set(fiber.HeaderCacheControl, "no-cache")
	c.Set(fiber.HeaderConnection, "keep-alive")
	// nginx must pass events through instead of buffering the response.
	c.Set("X-Accel-Buffering", "no")
	c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
		if s.Close != nil {
			defer s.Close()
		}
		run(w, s, session)
	})
	return nil
}

func run(w *bufio.Writer, s Stream, session *middlewares.Session) {
	if s.First != nil && writeEvent(w, s.First) != nil {
		return
	}
	beat := time.NewTicker(Heartbeat)
	defer beat.Stop()
	for {
		select {
		case msg, ok := <-s.Events:
			if !ok || writeEvent(w, msg) != nil {
				return
			}
		case <-beat.C:
			if !alive(session) {
				return
			}
			if _, err := w.WriteString(": ping\n\n"); err != nil {
				return
			}
			if err := w.Flush(); err != nil {
				return
			}
		}
	}
}

// alive reports whether the stream may go on. A stream opened on a route the
// auth middleware does not guard has no session and is never cut here.
func alive(session *middlewares.Session) bool {
	if session == nil {
		return true
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return session.Check(ctx) == nil
}

func writeEvent(w *bufio.Writer, data []byte) error {
	if _, err := w.WriteString("data: "); err != nil {
		return err
	}
	if _, err := w.Write(data); err != nil {
		return err
	}
	if _, err := w.WriteString("\n\n"); err != nil {
		return err
	}
	return w.Flush()
}
