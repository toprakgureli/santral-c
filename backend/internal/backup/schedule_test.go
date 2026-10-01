package backup

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
)

func TestDue(t *testing.T) {
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	at := func(d time.Duration) *time.Time { v := now.Add(-d); return &v }
	cases := []struct {
		name string
		h    history
		want bool
	}{
		{"never ran", history{}, true},
		{"good run five hours ago", history{lastOK: at(5 * time.Hour)}, false},
		{"good run six hours ago", history{lastOK: at(6 * time.Hour)}, true},
		{"failed twenty minutes ago", history{lastOK: at(7 * time.Hour), failures: 1, lastTry: at(20 * time.Minute)}, false},
		{"failed half an hour ago", history{lastOK: at(7 * time.Hour), failures: 1, lastTry: at(30 * time.Minute)}, true},
		{"second failure waits an hour", history{lastOK: at(8 * time.Hour), failures: 2, lastTry: at(45 * time.Minute)}, false},
		{"second failure an hour ago", history{lastOK: at(8 * time.Hour), failures: 2, lastTry: at(time.Hour)}, true},
		{"many failures wait at most six hours", history{lastOK: at(48 * time.Hour), failures: 12, lastTry: at(6 * time.Hour)}, true},
		{"many failures, five hours ago", history{lastOK: at(48 * time.Hour), failures: 12, lastTry: at(5 * time.Hour)}, false},
		{"never worked, failed an hour ago", history{failures: 1, lastTry: at(time.Hour)}, true},
		{"never worked, failed ten minutes ago", history{failures: 1, lastTry: at(10 * time.Minute)}, false},
		{"a manual failure does not bring the next one forward", history{lastOK: at(2 * time.Hour), failures: 1, lastTry: at(time.Hour)}, false},
	}
	for _, c := range cases {
		if got := due(now, c.h); got != c.want {
			t.Errorf("%s: due = %v, want %v", c.name, got, c.want)
		}
	}
}

// addRun stores a finished run that started ago.
func addRun(t *testing.T, s *Service, ago time.Duration, ok bool) {
	t.Helper()
	start := time.Now().Add(-ago)
	end := start.Add(time.Minute)
	if err := s.db.Create(&models.BackupRun{StartedAt: start, FinishedAt: &end, OK: ok}).Error; err != nil {
		t.Fatal(err)
	}
}

func enable(t *testing.T, s *Service, sa string) {
	t.Helper()
	if _, err := s.Save(context.Background(), 1, Input{Enabled: true, FolderID: "abc123", Credentials: sa}, ""); err != nil {
		t.Fatal(err)
	}
}

func runCount(t *testing.T, s *Service) int64 {
	t.Helper()
	var n int64
	if err := s.db.Model(&models.BackupRun{}).Count(&n).Error; err != nil {
		t.Fatal(err)
	}
	return n
}

// TestScheduleFollowsTheLastGoodRun: a run that started (and failed) after
// the last good one neither pushes the next backup six hours away nor is
// it retried at once; half an hour later it is.
func TestScheduleFollowsTheLastGoodRun(t *testing.T) {
	f := &fakeDrive{t: t, sharedOK: true}
	s, sa := setup(t, f)
	enable(t, s, sa)
	ctx := context.Background()

	addRun(t, s, 7*time.Hour, true)
	addRun(t, s, 10*time.Minute, false)
	before := runCount(t, s)
	s.dueRun(ctx)
	if runCount(t, s) != before {
		t.Fatal("a failure ten minutes ago was retried at once")
	}

	s.db.Exec("UPDATE backup_runs SET started_at = started_at - interval '30 minutes' WHERE NOT ok")
	s.dueRun(ctx)
	if runCount(t, s) != before+1 || !lastRun(t, s.db).OK {
		t.Fatalf("the failed backup was not retried half an hour later: %+v", lastRun(t, s.db))
	}
}

// TestStuckUploadEnds: an upload that never gets an answer is given up at
// its deadline and recorded as failed.
func TestStuckUploadEnds(t *testing.T) {
	f := &fakeDrive{t: t, sharedOK: true, hang: true}
	s, sa := setup(t, f)
	enable(t, s, sa)
	old := uploadTimeout
	uploadTimeout = 300 * time.Millisecond
	t.Cleanup(func() { uploadTimeout = old })

	done := make(chan struct{})
	go func() { s.run(context.Background(), nil); close(done) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("a stuck upload held the backup")
	}
	r := lastRun(t, s.db)
	if r.OK || r.FinishedAt == nil || !strings.Contains(r.Error, "bitmedi") {
		t.Fatalf("run = %+v, want a failure past the deadline", r)
	}
}

// TestShutdownStillRecordsTheOutcome: when the server stops during an
// upload, the run is still closed with its outcome.
func TestShutdownStillRecordsTheOutcome(t *testing.T) {
	f := &fakeDrive{t: t, sharedOK: true, hang: true, started: make(chan struct{}, 1)}
	s, sa := setup(t, f)
	enable(t, s, sa)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { s.run(ctx, nil); close(done) }()
	select {
	case <-f.started:
	case <-time.After(10 * time.Second):
		t.Fatal("the upload never started")
	}
	cancel()
	<-done
	r := lastRun(t, s.db)
	if r.FinishedAt == nil || r.OK || !strings.Contains(r.Error, "sunucu duruyordu") {
		t.Fatalf("run = %+v, want closed as cut off by the shutdown", r)
	}
}

// TestUnfinishedRunsAreClosedAtStart: a run the previous server left open
// is closed as failed, and counts as a try for the schedule.
func TestUnfinishedRunsAreClosedAtStart(t *testing.T) {
	s, _ := setup(t, &fakeDrive{t: t})
	if err := s.db.Create(&models.BackupRun{StartedAt: time.Now().Add(-time.Hour)}).Error; err != nil {
		t.Fatal(err)
	}
	s.closeUnfinished(context.Background())
	r := lastRun(t, s.db)
	if r.FinishedAt == nil || r.OK || !strings.Contains(r.Error, "yarıda") {
		t.Fatalf("run = %+v", r)
	}
	h, err := s.history(context.Background())
	if err != nil || h.failures != 1 || h.lastOK != nil {
		t.Fatalf("history = %+v %v", h, err)
	}
}
