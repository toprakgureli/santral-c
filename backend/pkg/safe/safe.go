// Package safe runs background work so that a panic in it is logged with its
// stack instead of taking the whole server down, and so that long-running
// loops can be waited for on shutdown.
package safe

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sync"
	"time"
)

// restartPause is how long a loop that panicked waits before it starts again,
// so a panic on every run does not spin the CPU.
const restartPause = 5 * time.Second

// PanicError is a recovered panic turned into an error.
type PanicError struct {
	Value any
	Stack []byte
}

func (e *PanicError) Error() string {
	return fmt.Sprintf("panic: %v", e.Value)
}

// IsPanic reports whether err is, or wraps, a recovered panic.
func IsPanic(err error) bool {
	var p *PanicError
	return errors.As(err, &p)
}

// Call runs fn and returns a panic in it as a *PanicError, so a worker can
// treat one bad item like a failed one and move on.
func Call(fn func() error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = &PanicError{Value: r, Stack: debug.Stack()}
		}
	}()
	return fn()
}

// Run runs fn and logs a panic in it instead of letting it spread. It suits
// one step of a loop whose other steps must keep running.
func Run(ctx context.Context, task string, fn func()) {
	err := Call(func() error {
		fn()
		return nil
	})
	if err != nil {
		logPanic(ctx, task, err)
	}
}

// Go starts fn in its own goroutine and logs a panic in it. It is for short
// tasks that end by themselves.
func Go(ctx context.Context, task string, fn func()) {
	go Run(ctx, task, fn)
}

// Group runs long-lived background loops and lets the caller wait for them
// to stop. The zero value is ready to use.
type Group struct {
	wg sync.WaitGroup
}

// Loop starts fn in its own goroutine. fn is expected to run until ctx ends.
// If it panics, the panic is logged and fn starts again after a short pause.
func (g *Group) Loop(ctx context.Context, task string, fn func(ctx context.Context)) {
	g.wg.Add(1)
	go func() {
		defer g.wg.Done()
		for {
			Run(ctx, task, func() { fn(ctx) })
			if ctx.Err() != nil {
				return
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(restartPause):
			}
		}
	}()
}

// Wait blocks until every loop has returned or ctx ends, whichever comes
// first. It returns ctx's error when it gave up waiting.
func (g *Group) Wait(ctx context.Context) error {
	done := make(chan struct{})
	go func() {
		g.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("background work did not stop in time: %w", ctx.Err())
	}
}

func logPanic(ctx context.Context, task string, err error) {
	var p *PanicError
	if errors.As(err, &p) {
		slog.ErrorContext(ctx, "background task panicked", "task", task, "panic", fmt.Sprint(p.Value), "stack", string(p.Stack))
		return
	}
	slog.ErrorContext(ctx, "background task failed", "task", task, "error", err)
}
