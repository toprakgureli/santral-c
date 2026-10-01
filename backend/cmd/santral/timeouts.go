package main

import (
	"bytes"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/valyala/fasthttp"
)

// timeouts bounds how long one connection may take, so a client that
// stops half way (or never sends a whole request) cannot hold a connection
// and its memory for ever. The time a handler works is not counted: the
// write timer starts once the answer is ready.
type timeouts struct {
	// Read is how long a request may take to arrive; Upload replaces it for
	// requests that announce a large body (files up to 100 MB).
	Read, Upload time.Duration
	// Write is how long an ordinary answer may take to go out; Download
	// replaces it for GET requests (files, exports, recordings that stream
	// for minutes) and Stream for the live event streams, which stay open
	// as long as the panel does.
	Write, Download, Stream time.Duration
	// Idle is how long a kept-alive connection may wait for its next
	// request.
	Idle time.Duration
}

// serverTimeouts are the values the server runs with.
var serverTimeouts = timeouts{
	Read:     30 * time.Second,
	Upload:   10 * time.Minute,
	Write:    2 * time.Minute,
	Download: time.Hour,
	Stream:   24 * time.Hour,
	Idle:     75 * time.Second,
}

// uploadBody is the announced body size from which a request counts as an
// upload.
const uploadBody = 1 << 20

// apply sets the server-wide timeouts on the Fiber configuration.
func (t timeouts) apply(cfg *fiber.Config) {
	cfg.ReadTimeout = t.Read
	cfg.WriteTimeout = t.Write
	cfg.IdleTimeout = t.Idle
}

// install gives each request its own limits once its header has arrived.
// The server's write timeout covers the whole answer, so without this a
// live stream or a long download would be cut when it runs out.
func (t timeouts) install(app *fiber.App) {
	app.Server().HeaderReceived = t.forRequest
}

func (t timeouts) forRequest(h *fasthttp.RequestHeader) fasthttp.RequestConfig {
	var rc fasthttp.RequestConfig
	if h.ContentLength() > uploadBody {
		rc.ReadTimeout = t.Upload
	}
	if bytes.Equal(h.Method(), []byte(fiber.MethodGet)) {
		rc.WriteTimeout = t.Download
		if isStream(h.RequestURI()) {
			rc.WriteTimeout = t.Stream
		}
	}
	return rc
}

// isStream reports whether a request opens a live event stream; every one
// of them ends in /stream.
func isStream(uri []byte) bool {
	path, _, _ := strings.Cut(string(uri), "?")
	return strings.HasSuffix(strings.TrimSuffix(path, "/"), "/stream")
}
