// Package sse writes Server-Sent Event streams for the panel's live views
// (the chat, WhatsApp and the agent list). Every stream sends a first
// message, forwards events as they come, keeps the connection alive through
// nginx with a comment line, and closes itself once the caller's session is
// revoked, so a deactivated user stops receiving data right away.
package sse

import (
	"bufio"
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/toprakgureli/santral-c/backend/internal/middlewares"
)

// Heartbeat is how often an idle stream sends a comment line. It must stay
// under nginx's proxy_read_timeout (30 s) so the connection is not cut, and
// it is also how often the session is re-checked.
const Heartbeat = 20 * time.Second

// beat is the heartbeat in use: Heartbeat unless a test set a shorter one.
var beat atomic.Int64

// SetHeartbeat changes how often idle streams send a comment line and check
// the session again, and returns a function that puts the old value back.
// Tests use it to see a revoked session end a stream without waiting
// twenty seconds; streams opened afterwards use the new value.
func SetHeartbeat(d time.Duration) (restore func()) {
	old := beat.Swap(int64(d))
	return func() { beat.Store(old) }
}

func heartbeat() time.Duration {
	if d := time.Duration(beat.Load()); d > 0 {
		return d
	}
	return Heartbeat
}

// streams counts the event streams open right now, for the metrics page.
var streams atomic.Int64

// closing ends every open stream when the server stops, so shutdown does
// not wait for browsers that would keep them open forever.
var (
	closing     = make(chan struct{})
	closingOnce sync.Once
)

// CloseAll ends every open stream and refuses new ones; the panel
// reconnects to the next server.
func CloseAll() {
	closingOnce.Do(func() { close(closing) })
}

// Open returns how many event streams are open right now.
func Open() int64 { return streams.Load() }

// Stream is one open event stream.
type Stream struct {
	// First is written as the first event; nil sends nothing.
	First []byte
	// Events delivers the stream's events; a closed channel ends the stream.
	Events <-chan []byte
	// Close runs once when the stream ends, whatever the reason.
	Close func()
	// Allowed is asked again with every heartbeat; an error ends the
	// stream, so someone who lost the permission stops receiving. Nil
	// checks only the session.
	Allowed func(ctx context.Context) error
}

// Serve sets the event-stream headers and streams s to the client after the
// handler returns. It must be the last thing a handler does.
func Serve(c *fiber.Ctx, s Stream) error {
	select {
	case <-closing:
		if s.Close != nil {
			s.Close()
		}
		return fiber.ErrServiceUnavailable
	default:
	}
	session := middlewares.SessionFrom(c)
	c.Set(fiber.HeaderContentType, "text/event-stream")
	c.Set(fiber.HeaderCacheControl, "no-cache")
	c.Set(fiber.HeaderConnection, "keep-alive")
	// nginx must pass events through instead of buffering the response.
	c.Set("X-Accel-Buffering", "no")
	c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
		streams.Add(1)
		defer streams.Add(-1)
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
	ticker := time.NewTicker(heartbeat())
	defer ticker.Stop()
	for {
		select {
		case <-closing:
			return
		case msg, ok := <-s.Events:
			if !ok || writeEvent(w, msg) != nil {
				return
			}
		case <-ticker.C:
			if !alive(session, s.Allowed) {
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
func alive(session *middlewares.Session, allowed func(ctx context.Context) error) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if session != nil && session.Check(ctx) != nil {
		return false
	}
	return allowed == nil || allowed(ctx) == nil
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
