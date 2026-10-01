// Package logctx carries who and which request a log line belongs to. The
// HTTP layer puts the request id and the signed-in user into the request's
// context; every slog call made with that context then carries them, so
// the lines of one request can be found together.
package logctx

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel/trace"
)

type key int

const (
	requestKey key = iota
	userKey
)

// WithRequest returns ctx carrying the request id.
func WithRequest(ctx context.Context, id string) context.Context {
	if id == "" {
		return ctx
	}
	return context.WithValue(ctx, requestKey, id)
}

// WithUser returns ctx carrying the signed-in user's id.
func WithUser(ctx context.Context, userID uint) context.Context {
	if userID == 0 {
		return ctx
	}
	return context.WithValue(ctx, userKey, userID)
}

// Handler adds the request id and user id found in a record's context to
// the record, then passes it on.
type Handler struct {
	next slog.Handler
}

var _ slog.Handler = (*Handler)(nil)

// NewHandler wraps next.
func NewHandler(next slog.Handler) *Handler { return &Handler{next: next} }

// Enabled reports whether next logs at level.
func (h *Handler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

// Handle adds the request and user to r.
func (h *Handler) Handle(ctx context.Context, r slog.Record) error {
	if ctx != nil {
		if id, ok := ctx.Value(requestKey).(string); ok {
			r.AddAttrs(slog.String("request_id", id))
		}
		if uid, ok := ctx.Value(userKey).(uint); ok {
			r.AddAttrs(slog.Uint64("user_id", uint64(uid)))
		}
		// With tracing on, a log line leads to its request's trace.
		if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
			r.AddAttrs(slog.String("trace_id", sc.TraceID().String()))
		}
	}
	return h.next.Handle(ctx, r)
}

// WithAttrs returns a handler with attrs added.
func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &Handler{next: h.next.WithAttrs(attrs)}
}

// WithGroup returns a handler that puts later attrs in a group.
func (h *Handler) WithGroup(name string) slog.Handler {
	return &Handler{next: h.next.WithGroup(name)}
}
