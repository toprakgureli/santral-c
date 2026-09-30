package safe

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestCallTurnsPanicIntoError(t *testing.T) {
	err := Call(func() error { panic("boom") })
	if !IsPanic(err) {
		t.Fatalf("Call() = %v, want a panic error", err)
	}
	want := errors.New("plain")
	if got := Call(func() error { return want }); !errors.Is(got, want) || IsPanic(got) {
		t.Fatalf("Call() = %v, want %v", got, want)
	}
}

func TestLoopRestartsAfterPanicAndStops(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var runs atomic.Int32
	var g Group
	g.Loop(ctx, "test", func(ctx context.Context) {
		if runs.Add(1) == 1 {
			panic("first run fails")
		}
		<-ctx.Done()
	})

	deadline := time.Now().Add(restartPause + 3*time.Second)
	for runs.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if runs.Load() < 2 {
		t.Fatal("loop did not restart after a panic")
	}
	cancel()

	waitCtx, waitCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer waitCancel()
	if err := g.Wait(waitCtx); err != nil {
		t.Fatalf("Wait() = %v", err)
	}
}
