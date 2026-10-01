package verimor

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

// The do-not-disturb queue. The phone system only routes queue calls to an
// extension whose do-not-disturb is off, so a change the panel makes (a
// shift starts, a break ends) must reach it even when its API is busy. Each
// extension's wanted state is recorded here, the latest wish wins, and it is
// sent until the phone system says yes. A change the agent makes goes out
// at once while the phone system accepts; once it pushes back (too many
// requests, a restart) every change waits its turn and goes out at the pace
// of the API budget, with growing pauses while the pushback lasts.
const (
	dndPace           = 6 * time.Second  // one queued send per pace: ten a minute
	dndPauseMin       = 10 * time.Second // first pause after the phone system pushes back
	dndPauseMax       = 2 * time.Minute  // longest pause
	dndRetryMin       = 5 * time.Second  // first retry of one extension
	dndRetryMax       = 5 * time.Minute  // longest wait between retries of one extension
	dndSendTimeout    = 15 * time.Second // one request to the phone system
	dndDriftSightings = 2                // drift must be seen this often before it is fixed
)

// dndSender changes an extension's do-not-disturb on the phone system.
type dndSender func(ctx context.Context, extension string, on bool) error

// dndWish is the wanted do-not-disturb state of one extension.
type dndWish struct {
	on        bool
	confirmed bool      // the phone system said yes to this wish
	at        time.Time // when the phone system said yes
	seq       uint64    // grows with every new wish, so a late answer for an old one confirms nothing
	attempts  int       // failed sends of this wish
	due       time.Time // the earliest next send
	inflight  bool      // a send is on its way
	drift     int       // consecutive reports from the phone system that disagree
	changed   time.Time // when the panel last asked for a state
}

// dndQueue holds every extension's wanted state and sends it.
type dndQueue struct {
	send dndSender
	now  func() time.Time

	pace     time.Duration
	pauseMin time.Duration
	pauseMax time.Duration
	retryMin time.Duration
	retryMax time.Duration

	mu          sync.Mutex
	wishes      map[string]*dndWish
	pausedUntil time.Time // nothing is sent before this (the phone system pushed back)
	strikes     int       // pushbacks in a row
	lastQueued  time.Time // the last send the worker made
	wake        chan struct{}
}

func newDNDQueue(send dndSender) *dndQueue {
	return &dndQueue{
		send:     send,
		now:      time.Now,
		pace:     dndPace,
		pauseMin: dndPauseMin,
		pauseMax: dndPauseMax,
		retryMin: dndRetryMin,
		retryMax: dndRetryMax,
		wishes:   map[string]*dndWish{},
		wake:     make(chan struct{}, 1),
	}
}

// tune changes the pace and the pauses (tests run them in milliseconds).
func (q *dndQueue) tune(pace, pause time.Duration) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.pace = pace
	q.pauseMin, q.pauseMax = pause, 12*pause
	q.retryMin, q.retryMax = pause/2, 30*pause
}

// want records the wanted state of an extension. With now set it is sent
// at once unless the phone system is pushing back; otherwise it waits for
// the worker. It reports whether the phone system has confirmed the state.
func (q *dndQueue) want(ctx context.Context, ext string, on, now bool) bool {
	q.mu.Lock()
	w := q.wishes[ext]
	if w == nil {
		w = &dndWish{}
		q.wishes[ext] = w
	}
	if w.seq > 0 && w.on == on {
		// Already wanted. A confirmed state is not sent again (the phone
		// system's reports catch drift); a waiting one is tried now when
		// the agent asked for it.
		if w.confirmed || !now {
			ok := w.confirmed
			q.mu.Unlock()
			return ok
		}
		w.due = q.now()
		q.mu.Unlock()
		return q.flush(ctx, ext)
	}
	w.on, w.confirmed, w.attempts, w.drift = on, false, 0, 0
	w.changed = q.now()
	w.seq++
	w.due = q.now()
	q.mu.Unlock()
	if !now {
		q.kick()
		return false
	}
	return q.flush(ctx, ext)
}

// confirmed reports whether the extension's latest wish has been accepted.
// An extension the queue never heard of counts as confirmed.
func (q *dndQueue) confirmed(ext string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	w := q.wishes[ext]
	return w == nil || w.confirmed
}

// adopt queues a state read from the database at asOf, unless the panel
// asked for a state after that read (the read is then already stale).
func (q *dndQueue) adopt(ctx context.Context, ext string, on bool, asOf time.Time) {
	q.mu.Lock()
	w := q.wishes[ext]
	stale := w != nil && (w.on == on || w.changed.After(asOf))
	q.mu.Unlock()
	if !stale {
		q.want(ctx, ext, on, false)
	}
}

// flush sends the extension's latest wish until it is confirmed or a send
// fails. A wish that arrives while a send is on its way is sent right after
// it, so two changes never race each other on the line.
func (q *dndQueue) flush(ctx context.Context, ext string) bool {
	for {
		q.mu.Lock()
		w := q.wishes[ext]
		if w == nil || w.confirmed {
			q.mu.Unlock()
			return w != nil
		}
		if w.inflight || q.now().Before(q.pausedUntil) {
			q.mu.Unlock()
			q.kick()
			return false
		}
		w.inflight = true
		on, seq := w.on, w.seq
		q.mu.Unlock()

		sendCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), dndSendTimeout)
		err := q.send(sendCtx, ext, on)
		cancel()

		if !q.settle(ctx, ext, seq, err) {
			return false
		}
	}
}

// settle records the answer to a send. It returns true when the send went
// through, so the caller may look for a newer wish.
func (q *dndQueue) settle(ctx context.Context, ext string, seq uint64, err error) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	w := q.wishes[ext]
	w.inflight = false
	now := q.now()
	if err == nil {
		q.strikes = 0
		q.pausedUntil = time.Time{}
		if w.seq == seq {
			w.confirmed, w.at, w.attempts, w.drift = true, now, 0, 0
		}
		return true
	}
	if w.seq == seq {
		w.attempts++
		w.due = now.Add(backoff(q.retryMin, q.retryMax, w.attempts))
	}
	if pushback(err) {
		q.strikes++
		q.pausedUntil = now.Add(backoff(q.pauseMin, q.pauseMax, q.strikes))
	}
	slog.WarnContext(ctx, "do-not-disturb change not accepted yet; it is retried", "extension", ext, "attempt", w.attempts, "error", err)
	q.kickLocked()
	return false
}

// pushback reports whether an error means the phone system is busy or away
// (too many requests, a server error, no answer) rather than refusing this
// one change. While it pushes back, nothing is sent.
func pushback(err error) bool {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.Status == http.StatusTooManyRequests || apiErr.Status >= 500
	}
	return true
}

// backoff doubles from lo for every attempt, up to hi.
func backoff(lo, hi time.Duration, attempt int) time.Duration {
	d := lo
	for i := 1; i < attempt && d < hi; i++ {
		d *= 2
	}
	return min(d, hi)
}

func (q *dndQueue) kick() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.kickLocked()
}

func (q *dndQueue) kickLocked() {
	select {
	case q.wake <- struct{}{}:
	default:
	}
}

// next picks the wish to send now and marks it on its way; ok is false when
// nothing may go yet, with how long to wait.
func (q *dndQueue) next() (ext string, on bool, seq uint64, wait time.Duration, ok bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	now := q.now()
	gate := q.pausedUntil
	if paced := q.lastQueued.Add(q.pace); paced.After(gate) {
		gate = paced
	}
	var pick string
	var pickDue time.Time
	for e, w := range q.wishes {
		if w.confirmed || w.inflight {
			continue
		}
		if pick == "" || w.due.Before(pickDue) {
			pick, pickDue = e, w.due
		}
	}
	if pick == "" {
		return "", false, 0, time.Minute, false
	}
	if pickDue.After(gate) {
		gate = pickDue
	}
	if gate.After(now) {
		return "", false, 0, gate.Sub(now), false
	}
	w := q.wishes[pick]
	w.inflight = true
	q.lastQueued = now
	return pick, w.on, w.seq, 0, true
}

// run sends queued wishes one at a time, at the budget's pace, until ctx
// ends.
func (q *dndQueue) run(ctx context.Context) {
	for {
		ext, on, seq, wait, ok := q.next()
		if ok {
			sendCtx, cancel := context.WithTimeout(ctx, dndSendTimeout)
			err := q.send(sendCtx, ext, on)
			cancel()
			if ctx.Err() != nil {
				q.mu.Lock()
				q.wishes[ext].inflight = false
				q.mu.Unlock()
				return
			}
			q.settle(ctx, ext, seq, err)
			continue
		}
		t := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			t.Stop()
			return
		case <-q.wake:
		case <-t.C:
		}
		t.Stop()
	}
}

// observe compares a confirmed wish with what the phone system reports for
// the extension (reported at asOf). A disagreement seen twice in a row on
// fresh reports sends the wish again, so a change made elsewhere (the phone
// system's own panel, a lost request) is put right. Statuses that say
// nothing about do-not-disturb (talking, unregistered) are ignored.
func (q *dndQueue) observe(ext, status string, asOf time.Time) {
	var reportedOn bool
	switch status {
	case "SS_DND":
		reportedOn = true
	case "AVAILABLE":
		reportedOn = false
	default:
		return
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	w := q.wishes[ext]
	if w == nil || !w.confirmed || !asOf.After(w.at) {
		return
	}
	if reportedOn == w.on {
		w.drift = 0
		return
	}
	w.drift++
	if w.drift < dndDriftSightings {
		return
	}
	w.confirmed, w.attempts, w.drift = false, 0, 0
	w.seq++
	w.due = q.now()
	q.kickLocked()
}
