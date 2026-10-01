package ops

import (
	"context"
	"testing"
	"time"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
	"github.com/toprakgureli/santral-c/backend/internal/testdb"
	"github.com/toprakgureli/santral-c/backend/internal/whatsapp/store"
)

// TestFailedNoticesCountForAnHour: a notice the worker gives up on gets
// the time it failed, and the warning counts only the last hour's.
func TestFailedNoticesCountForAnHour(t *testing.T) {
	db := testdb.Open(t)
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	var ids []uint
	t.Cleanup(func() { db.Exec("DELETE FROM wa_webhook_events WHERE id IN ?", ids) })
	add := func() uint {
		ev := models.WAWebhookEvent{Payload: `{"object":"ops-test"}`, Status: "pending", ReceivedAt: time.Now(), NextTryAt: time.Now()}
		if err := db.Create(&ev).Error; err != nil {
			t.Fatal(err)
		}
		ids = append(ids, ev.ID)
		return ev.ID
	}
	recent := func() int64 {
		var n int64
		if err := sqlDB.QueryRowContext(ctx, "SELECT count(*) FROM wa_webhook_events WHERE status = 'failed' AND failed_at > now() - interval '1 hour'").Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	repo := store.New(db)
	base := recent()

	old := add()
	if err := repo.WebhookEventFailed(ctx, old, "failed", 8, "boom", time.Now()); err != nil {
		t.Fatal(err)
	}
	db.Exec("UPDATE wa_webhook_events SET failed_at = now() - interval '2 hours' WHERE id = ?", old)
	retrying := add()
	if err := repo.WebhookEventFailed(ctx, retrying, "pending", 1, "boom", time.Now()); err != nil {
		t.Fatal(err)
	}
	if got := recent(); got != base {
		t.Fatalf("an old failure or a retry counted: %d, want %d", got, base)
	}

	fresh := add()
	if err := repo.WebhookEventFailed(ctx, fresh, "failed", 8, "boom", time.Now()); err != nil {
		t.Fatal(err)
	}
	if got := recent(); got != base+1 {
		t.Fatalf("a fresh failure did not count: %d, want %d", got, base+1)
	}
	m := NewMonitor(sqlDB, nil, "/nowhere")
	m.disk = func(string) (Disk, error) { return Disk{Used: 1, Free: 9}, nil }
	found := false
	for _, w := range m.Check(ctx) {
		if w.Key == "webhook" {
			found = true
		}
		if w.Key == "database" {
			t.Fatalf("the database was reported down: %+v", w)
		}
	}
	if !found {
		t.Fatal("the fresh failure gave no warning")
	}
}
