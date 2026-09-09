// Package ami is a minimal Asterisk Manager Interface client: it logs in,
// streams events to a handler and sends actions, reconnecting on failure.
package ami

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Event is one AMI message as ordered-insensitive key/value pairs. Keys are
// canonicalized to their original AMI casing (e.g. "Event", "Linkedid").
type Event map[string]string

// Get returns the value for a case-insensitive key.
func (e Event) Get(key string) string {
	if v, ok := e[key]; ok {
		return v
	}
	for k, v := range e {
		if strings.EqualFold(k, key) {
			return v
		}
	}
	return ""
}

// Name returns the event name.
func (e Event) Name() string { return e.Get("Event") }

// Config holds AMI connection settings.
type Config struct {
	Address  string
	Username string
	Secret   string
}

// Client is an AMI connection manager.
type Client struct {
	cfg Config

	mu      sync.Mutex
	conn    net.Conn
	writer  *bufio.Writer
	seq     uint64
	pending map[string]chan Event
}

// New builds an AMI client.
func New(cfg Config) *Client {
	return &Client{cfg: cfg, pending: make(map[string]chan Event)}
}

// Run connects, logs in and delivers events to handler until ctx is done,
// reconnecting with backoff on any failure.
func (c *Client) Run(ctx context.Context, handler func(Event)) {
	backoff := time.Second
	for {
		if ctx.Err() != nil {
			return
		}
		if err := c.session(ctx, handler); err != nil && ctx.Err() == nil {
			slog.Warn("ami session ended", "error", err, "retry_in", backoff.String())
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			if backoff < 30*time.Second {
				backoff *= 2
			}
			continue
		}
		backoff = time.Second
	}
}

func (c *Client) session(ctx context.Context, handler func(Event)) error {
	conn, err := net.DialTimeout("tcp", c.cfg.Address, 10*time.Second)
	if err != nil {
		return fmt.Errorf("ami dial failed: %w", err)
	}
	defer func() { _ = conn.Close() }()

	reader := bufio.NewReader(conn)
	if _, err := reader.ReadString('\n'); err != nil { // banner line
		return fmt.Errorf("ami banner read failed: %w", err)
	}

	c.mu.Lock()
	c.conn = conn
	c.writer = bufio.NewWriter(conn)
	c.mu.Unlock()

	// The read loop must run concurrently with login so the login response,
	// which arrives on the same socket, is actually read.
	errc := make(chan error, 1)
	go func() { errc <- c.readLoop(reader, handler) }()

	if _, err := c.request(ctx, map[string]string{
		"Action":   "Login",
		"Username": c.cfg.Username,
		"Secret":   c.cfg.Secret,
	}); err != nil {
		_ = conn.Close()
		<-errc
		return fmt.Errorf("ami login failed: %w", err)
	}
	slog.Info("ami connected", "address", c.cfg.Address)

	select {
	case <-ctx.Done():
		_ = conn.Close()
		<-errc
		return nil
	case err := <-errc:
		return err
	}
}

func (c *Client) readLoop(reader *bufio.Reader, handler func(Event)) error {
	for {
		msg, err := readMessage(reader)
		if err != nil {
			return err
		}
		if id := msg.Get("ActionID"); id != "" {
			if c.deliver(id, msg) {
				continue
			}
		}
		if msg.Name() != "" {
			handler(msg)
		}
	}
}

// Originate places a call and returns the action response.
func (c *Client) Originate(ctx context.Context, fields map[string]string) (Event, error) {
	action := map[string]string{"Action": "Originate"}
	for k, v := range fields {
		action[k] = v
	}
	return c.request(ctx, action)
}

// Command runs a CLI command through AMI (used for pjsip reload).
func (c *Client) Command(ctx context.Context, command string) (Event, error) {
	return c.request(ctx, map[string]string{"Action": "Command", "Command": command})
}

func (c *Client) request(ctx context.Context, action map[string]string) (Event, error) {
	c.mu.Lock()
	if c.writer == nil {
		c.mu.Unlock()
		return nil, fmt.Errorf("ami not connected")
	}
	c.seq++
	id := strconv.FormatUint(c.seq, 10)
	ch := make(chan Event, 1)
	c.pending[id] = ch
	action["ActionID"] = id
	err := writeAction(c.writer, action)
	c.mu.Unlock()

	if err != nil {
		c.clearPending(id)
		return nil, fmt.Errorf("ami write failed: %w", err)
	}

	select {
	case <-ctx.Done():
		c.clearPending(id)
		return nil, ctx.Err()
	case <-time.After(10 * time.Second):
		c.clearPending(id)
		return nil, fmt.Errorf("ami action %q timed out", action["Action"])
	case resp := <-ch:
		if strings.EqualFold(resp.Get("Response"), "Error") {
			return resp, fmt.Errorf("ami action %q rejected: %s", action["Action"], resp.Get("Message"))
		}
		return resp, nil
	}
}

func (c *Client) deliver(id string, msg Event) bool {
	c.mu.Lock()
	ch, ok := c.pending[id]
	if ok {
		delete(c.pending, id)
	}
	c.mu.Unlock()
	if ok {
		ch <- msg
	}
	return ok
}

func (c *Client) clearPending(id string) {
	c.mu.Lock()
	delete(c.pending, id)
	c.mu.Unlock()
}

func writeAction(w *bufio.Writer, action map[string]string) error {
	var b strings.Builder
	for k, v := range action {
		b.WriteString(k)
		b.WriteString(": ")
		b.WriteString(v)
		b.WriteString("\r\n")
	}
	b.WriteString("\r\n")
	if _, err := w.WriteString(b.String()); err != nil {
		return err
	}
	return w.Flush()
}

func readMessage(r *bufio.Reader) (Event, error) {
	msg := make(Event)
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			if len(msg) == 0 {
				continue // skip stray blank lines between messages
			}
			return msg, nil
		}
		if i := strings.Index(line, ": "); i >= 0 {
			msg[line[:i]] = line[i+2:]
		} else if i := strings.IndexByte(line, ':'); i >= 0 {
			msg[line[:i]] = strings.TrimSpace(line[i+1:])
		}
	}
}
