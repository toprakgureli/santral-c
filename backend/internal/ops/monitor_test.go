package ops

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // the driver for a database that is not there
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	goredis "github.com/redis/go-redis/v9"
)

const gb = 1 << 30

func TestDiskWarningLevels(t *testing.T) {
	if w := diskWarning(Disk{Used: 80 * gb, Free: 20 * gb}); w != nil {
		t.Fatalf("80%% full warned: %+v", w)
	}
	w := diskWarning(Disk{Used: 86 * gb, Free: 14 * gb})
	if w == nil || w.Level != LevelWarning || !strings.Contains(w.Text, "yüzde 86") || !strings.Contains(w.Text, "14,0 GB") {
		t.Fatalf("86%% full = %+v", w)
	}
	worse := diskWarning(Disk{Used: 91 * gb, Free: 9 * gb})
	if worse == nil || worse.Fingerprint == w.Fingerprint {
		t.Fatal("a fuller disk must show a closed warning again")
	}
	same := diskWarning(Disk{Used: 87 * gb, Free: 13 * gb})
	if same.Fingerprint != w.Fingerprint {
		t.Fatal("one more percent must not show a closed warning again")
	}
	if c := diskWarning(Disk{Used: 96 * gb, Free: 4 * gb}); c == nil || c.Level != LevelCritical {
		t.Fatalf("96%% full = %+v", c)
	}
}

func TestBackupWarning(t *testing.T) {
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	ago := func(d time.Duration) *time.Time { at := now.Add(-d); return &at }
	if w := backupWarning(now, now.Add(-time.Hour*48), ago(5*time.Hour)); w != nil {
		t.Fatalf("a backup five hours old warned: %+v", w)
	}
	w := backupWarning(now, now.Add(-48*time.Hour), ago(9*time.Hour))
	if w == nil || w.Level != LevelWarning || !strings.Contains(w.Text, "9 saat") || w.Link != "/settings" {
		t.Fatalf("nine hours = %+v", w)
	}
	if w := backupWarning(now, now.Add(-72*time.Hour), ago(50*time.Hour)); w == nil || w.Level != LevelCritical || !strings.Contains(w.Text, "2 gün") {
		t.Fatalf("two days = %+v", w)
	}
	if w := backupWarning(now, now.Add(-30*time.Minute), nil); w != nil {
		t.Fatalf("switched on half an hour ago and warned: %+v", w)
	}
	if w := backupWarning(now, now.Add(-2*time.Hour), nil); w == nil || !strings.Contains(w.Text, "henüz") {
		t.Fatalf("never worked = %+v", w)
	}
}

func TestQueueWarnings(t *testing.T) {
	if webhookWarning(0, 0) != nil {
		t.Error("no failed notice warned")
	}
	a, b := webhookWarning(2, 1000), webhookWarning(3, 2000)
	if a == nil || !strings.Contains(a.Text, "2 bildirim") || a.Fingerprint == b.Fingerprint {
		t.Errorf("failed notices = %+v / %+v", a, b)
	}
	if outboxWarning(5, 5*time.Minute) != nil {
		t.Error("a five minute wait warned")
	}
	if w := outboxWarning(5, 14*time.Minute); w == nil || w.Level != LevelWarning || !strings.Contains(w.Text, "14 dakika") {
		t.Errorf("fourteen minutes = %+v", w)
	}
	if w := outboxWarning(5, 40*time.Minute); w == nil || w.Level != LevelCritical {
		t.Errorf("forty minutes = %+v", w)
	}
	if inboundWarning(200) != nil {
		t.Error("200 waiting jobs warned")
	}
	if w := inboundWarning(340); w == nil || !strings.Contains(w.Text, "340") {
		t.Errorf("340 jobs = %+v", w)
	}
}

func TestHumanBytes(t *testing.T) {
	cases := map[uint64]string{512: "512 B", 1536: "1,5 KB", 12*gb + gb/2: "12,5 GB"}
	for in, want := range cases {
		if got := humanBytes(in); got != want {
			t.Errorf("humanBytes(%d) = %q, want %q", in, got, want)
		}
	}
}

// deadDeps are a database and a Redis that are not there.
func deadDeps(t *testing.T) (*sql.DB, *goredis.Client) {
	t.Helper()
	db, err := sql.Open("pgx", "host=127.0.0.1 port=1 user=x dbname=x sslmode=disable connect_timeout=1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	rdb := goredis.NewClient(&goredis.Options{Addr: "127.0.0.1:1", DialTimeout: 500 * time.Millisecond, MaxRetries: -1})
	t.Cleanup(func() { _ = rdb.Close() })
	return db, rdb
}

// TestDependencyGaugeWhenDown: with PostgreSQL and Redis gone, the gauge
// reads 0 for both instead of going missing, so the alert on it fires.
func TestDependencyGaugeWhenDown(t *testing.T) {
	db, rdb := deadDeps(t)
	reg := prometheus.NewRegistry()
	NewHandler(db, rdb, Build{}, reg)
	want := `
# HELP santral_dependency_up 1 when the server reaches the dependency (postgres, redis), 0 when it does not.
# TYPE santral_dependency_up gauge
santral_dependency_up{dep="postgres"} 0
santral_dependency_up{dep="redis"} 0
`
	if err := testutil.GatherAndCompare(reg, strings.NewReader(want), "santral_dependency_up"); err != nil {
		t.Fatal(err)
	}
}

// TestMonitorWithoutDatabase: the database warning comes first and the
// checks that need the database are skipped; Redis is reported too.
func TestMonitorWithoutDatabase(t *testing.T) {
	db, rdb := deadDeps(t)
	m := NewMonitor(db, rdb, "/nowhere")
	m.disk = func(string) (Disk, error) { return Disk{Used: 97, Free: 3}, nil }
	got := m.Check(context.Background())
	keys := make([]string, 0, len(got))
	for _, w := range got {
		keys = append(keys, w.Key)
	}
	if strings.Join(keys, ",") != "database,disk,redis" {
		t.Fatalf("warnings = %v", keys)
	}
}

// TestMonitorKeepsTheLastReport: within two minutes the report is not made
// again; after that a request makes a fresh one.
func TestMonitorKeepsTheLastReport(t *testing.T) {
	db, _ := deadDeps(t)
	m := NewMonitor(db, nil, "/nowhere")
	calls := 0
	m.disk = func(string) (Disk, error) { calls++; return Disk{Used: 1, Free: 9}, nil }
	now := time.Now()
	m.now = func() time.Time { return now }
	first := m.Current(context.Background())
	m.Current(context.Background())
	if calls != 1 {
		t.Fatalf("checked %d times within a minute", calls)
	}
	now = now.Add(3 * time.Minute)
	if again := m.Current(context.Background()); !again.CheckedAt.After(first.CheckedAt) || calls != 2 {
		t.Fatalf("a stale report was not made again (%d checks)", calls)
	}
}

func TestDiskGauge(t *testing.T) {
	db, _ := deadDeps(t)
	m := NewMonitor(db, nil, "/nowhere")
	m.disk = func(string) (Disk, error) { return Disk{Used: 3, Free: 1}, nil }
	if got := testutil.ToFloat64(m); got != 0.75 {
		t.Fatalf("disk gauge = %v", got)
	}
}
