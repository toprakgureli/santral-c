package telemetry

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/toprakgureli/santral-c/backend/internal/testdb"
)

// TestDatabaseGaugesRead runs every gauge's query against the real schema:
// one that fails would silently go missing from every scrape.
func TestDatabaseGaugesRead(t *testing.T) {
	db := testdb.Open(t)
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	if n := testutil.CollectAndCount(&queryCollector{db: sqlDB}); n != len(gauges) {
		t.Fatalf("%d of %d gauges could be read", n, len(gauges))
	}
}
