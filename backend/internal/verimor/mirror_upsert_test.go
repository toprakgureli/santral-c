package verimor

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
	"github.com/toprakgureli/santral-c/backend/internal/testdb"
)

// A page that holds the same call twice (it shifted while it was read, or
// the call rang several agents) is stored, keeping the answered record.
func TestUpsertCDRsKeepsEachCallOnce(t *testing.T) {
	db := testdb.Open(t)
	repo := NewRepository(db)
	stamp := time.Now().UnixNano()
	shifted, rang := fmt.Sprintf("shift-%d", stamp), fmt.Sprintf("rang-%d", stamp)
	t.Cleanup(func() { db.Exec("DELETE FROM pbx_cdrs WHERE call_uuid IN ?", []string{shifted, rang}) })
	start := time.Now().UTC().Format(stampLayout)
	page := []CDR{
		{CallUUID: shifted, StartStamp: start, Direction: "inbound", Duration: "00:00:40", TalkDuration: "00:00:30", AnswerStamp: start},
		{CallUUID: rang, StartStamp: start, Direction: "inbound", Duration: "00:00:20", Missed: true},
		{CallUUID: rang, StartStamp: start, Direction: "inbound", Duration: "00:01:10", TalkDuration: "00:01:00", AnswerStamp: start, RecordingPresent: true},
		{CallUUID: rang, StartStamp: start, Direction: "inbound", Duration: "00:00:15", Missed: true},
		{CallUUID: shifted, StartStamp: start, Direction: "inbound", Duration: "00:00:40", TalkDuration: "00:00:30", AnswerStamp: start},
	}
	n, err := repo.UpsertCDRs(context.Background(), page)
	if err != nil {
		t.Fatalf("UpsertCDRs() = %v", err)
	}
	if n != 2 {
		t.Fatalf("stored %d rows, want 2", n)
	}
	var row models.PBXCDR
	if err := db.Where("call_uuid = ?", rang).First(&row).Error; err != nil {
		t.Fatal(err)
	}
	if row.TalkDuration != "00:01:00" || !row.RecordingPresent || row.Missed {
		t.Fatalf("kept %+v, want the answered record with its recording", row)
	}
}
