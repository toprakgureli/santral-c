package verimor

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"
)

// fakeLine stands in for the phone system's do-not-disturb endpoint.
type fakeLine struct {
	mu     sync.Mutex
	state  map[string]bool
	sends  []string
	at     []time.Time
	refuse func(ext string, n int) error // answer to the n-th send (1-based)
}

func newFakeLine() *fakeLine { return &fakeLine{state: map[string]bool{}} }

func (f *fakeLine) send(_ context.Context, ext string, on bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sends = append(f.sends, fmt.Sprintf("%s=%v", ext, on))
	f.at = append(f.at, time.Now())
	if f.refuse != nil {
		if err := f.refuse(ext, len(f.sends)); err != nil {
			return err
		}
	}
	f.state[ext] = on
	return nil
}

func (f *fakeLine) get(ext string) (bool, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.state[ext]
	return v, ok
}

func (f *fakeLine) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.sends)
}

var errThrottled = &APIError{Op: "dnd change failed", Status: http.StatusTooManyRequests}

// runQueue starts the worker for the test's lifetime.
func runQueue(t *testing.T, q *dndQueue) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		q.run(ctx)
		close(done)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})
}

// eventually polls cond until it holds or a second passes.
func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("never happened: %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestDNDSendsAtOnceWhenAccepted(t *testing.T) {
	line := newFakeLine()
	q := newDNDQueue(line.send)
	if !q.want(context.Background(), "2001", false, true) {
		t.Fatal("an accepted change is not confirmed")
	}
	if on, ok := line.get("2001"); !ok || on {
		t.Fatalf("phone system state %v %v, want off", on, ok)
	}
	// The same state again is not sent twice.
	q.want(context.Background(), "2001", false, true)
	if line.count() != 1 {
		t.Errorf("%d sends, want 1", line.count())
	}
}

func TestDNDRetriesUntilConfirmedAndPausesOnPushback(t *testing.T) {
	line := newFakeLine()
	line.refuse = func(_ string, n int) error {
		if n <= 3 {
			return errThrottled
		}
		return nil
	}
	q := newDNDQueue(line.send)
	q.tune(time.Millisecond, 20*time.Millisecond)
	if q.want(context.Background(), "2001", false, true) {
		t.Fatal("a refused change reads as confirmed")
	}
	// While the phone system pushes back, another agent's change waits too.
	if q.want(context.Background(), "2002", true, true) {
		t.Fatal("a change sent during a pushback reads as confirmed")
	}
	if line.count() != 1 {
		t.Fatalf("%d sends during the pushback, want 1", line.count())
	}
	runQueue(t, q)
	eventually(t, "both changes confirmed", func() bool { return q.confirmed("2001") && q.confirmed("2002") })
	if on, _ := line.get("2001"); on {
		t.Error("2001 ended with do-not-disturb on")
	}
	if on, _ := line.get("2002"); !on {
		t.Error("2002 ended with do-not-disturb off")
	}
}

func TestDNDLatestWishWins(t *testing.T) {
	line := newFakeLine()
	blocked := true
	line.refuse = func(string, int) error {
		if blocked {
			return errThrottled
		}
		return nil
	}
	q := newDNDQueue(line.send)
	q.tune(time.Millisecond, 10*time.Millisecond)
	ctx := context.Background()
	q.want(ctx, "2001", true, true)
	q.want(ctx, "2001", false, true)
	q.want(ctx, "2001", true, true)
	q.want(ctx, "2001", false, true)
	line.mu.Lock()
	blocked = false
	line.mu.Unlock()
	runQueue(t, q)
	eventually(t, "confirmed", func() bool { return q.confirmed("2001") })
	if on, _ := line.get("2001"); on {
		t.Error("an older wish won over the latest one")
	}
}

func TestDNDOneRefusedExtensionDoesNotHoldUpOthers(t *testing.T) {
	line := newFakeLine()
	line.refuse = func(ext string, _ int) error {
		if ext == "9999" {
			return &APIError{Op: "dnd change failed", Status: http.StatusBadRequest, Body: "no such extension"}
		}
		return nil
	}
	q := newDNDQueue(line.send)
	ctx := context.Background()
	q.want(ctx, "9999", true, true)
	if !q.want(ctx, "2001", true, true) {
		t.Error("a refusal for one extension paused everyone")
	}
}

func TestDNDQueuedChangesKeepThePace(t *testing.T) {
	line := newFakeLine()
	q := newDNDQueue(line.send)
	const pace = 30 * time.Millisecond
	q.tune(pace, 50*time.Millisecond)
	ctx := context.Background()
	for i := range 6 {
		// The evening sweep: queued, not sent inline.
		if q.want(ctx, fmt.Sprint(3000+i), true, false) {
			t.Fatal("a queued change was sent inline")
		}
	}
	if line.count() != 0 {
		t.Fatalf("%d sends before the worker ran", line.count())
	}
	runQueue(t, q)
	eventually(t, "all six sent", func() bool { return line.count() == 6 })
	line.mu.Lock()
	defer line.mu.Unlock()
	for i := 1; i < len(line.at); i++ {
		if gap := line.at[i].Sub(line.at[i-1]); gap < pace-5*time.Millisecond {
			t.Errorf("send %d came %s after the one before, faster than the pace", i, gap)
		}
	}
}

func TestDNDDriftIsPutRight(t *testing.T) {
	line := newFakeLine()
	q := newDNDQueue(line.send)
	q.tune(time.Millisecond, 10*time.Millisecond)
	ctx := context.Background()
	q.want(ctx, "2001", true, true)
	// Someone switched it off in the phone system's own panel.
	line.mu.Lock()
	line.state["2001"] = false
	line.mu.Unlock()
	// Reports are fetched after the confirmation (the clock on some systems
	// moves in steps of several milliseconds).
	time.Sleep(20 * time.Millisecond)

	// A report older than the confirmation says nothing.
	q.observe("2001", "AVAILABLE", time.Now().Add(-time.Hour))
	q.observe("2001", "AVAILABLE", time.Now().Add(-time.Hour))
	if !q.confirmed("2001") {
		t.Fatal("an old report undid a confirmation")
	}
	// Talking says nothing about do-not-disturb.
	q.observe("2001", "TALKING", time.Now())
	// One fresh disagreeing report is not enough; two are.
	q.observe("2001", "AVAILABLE", time.Now())
	if !q.confirmed("2001") {
		t.Fatal("one report was taken as drift")
	}
	q.observe("2001", "AVAILABLE", time.Now())
	if q.confirmed("2001") {
		t.Fatal("drift seen twice was not acted on")
	}
	runQueue(t, q)
	eventually(t, "drift fixed", func() bool { on, _ := line.get("2001"); return on && q.confirmed("2001") })
}

func TestDNDAdoptSkipsAStaleRead(t *testing.T) {
	line := newFakeLine()
	q := newDNDQueue(line.send)
	ctx := context.Background()
	read := time.Now()
	time.Sleep(20 * time.Millisecond)
	// The agent started the shift after the database was read.
	q.want(ctx, "2001", false, true)
	q.adopt(ctx, "2001", true, read)
	if on, ok := q.wanted("2001"); !ok || on {
		t.Error("a stale database read overrode the agent's change")
	}
	// A fresh read of a new extension is queued.
	q.adopt(ctx, "2002", true, time.Now())
	if on, ok := q.wanted("2002"); !ok || !on {
		t.Error("a fresh read was not queued")
	}
}

func TestPushback(t *testing.T) {
	cases := []struct {
		err  error
		want bool
	}{
		{errThrottled, true},
		{&APIError{Status: 502}, true},
		{&APIError{Status: 400}, false},
		{errors.New("connection reset"), true},
	}
	for _, c := range cases {
		if got := pushback(c.err); got != c.want {
			t.Errorf("pushback(%v) = %v, want %v", c.err, got, c.want)
		}
	}
}

// wanted returns the extension's wanted state, if one is recorded.
func (q *dndQueue) wanted(ext string) (on, ok bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if w := q.wishes[ext]; w != nil {
		return w.on, true
	}
	return false, false
}
