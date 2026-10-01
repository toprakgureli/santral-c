package postgresql

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/toprakgureli/santral-c/backend/configs"
)

func TestStatementTimeoutIsSentWithEveryConnection(t *testing.T) {
	c := configs.Database{Host: "db", Port: "5432", User: "u", Password: "p", Name: "n", SSLMode: "disable", TimeZone: "UTC", StatementTimeout: 30 * time.Second}
	cfg, err := pgconn.ParseConfig(DSN(c))
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.RuntimeParams["statement_timeout"]; got != "30000" {
		t.Fatalf("pool statement_timeout = %q", got)
	}
	m, err := pgconn.ParseConfig(MigratorDSN(c))
	if err != nil {
		t.Fatal(err)
	}
	if m.RuntimeParams["statement_timeout"] != "0" || m.RuntimeParams["application_name"] != MigratorName {
		t.Fatalf("migration connection params = %v", m.RuntimeParams)
	}
}

// testConfig reads the test database's address into a configuration.
func testConfig(t *testing.T) configs.Database {
	t.Helper()
	dsn := os.Getenv("SANTRAL_TEST_DSN")
	if dsn == "" {
		t.Skip("SANTRAL_TEST_DSN is not set; skipping a test that needs PostgreSQL")
	}
	c := configs.Database{Port: "5432", SSLMode: "disable", TimeZone: "UTC"}
	for _, kv := range strings.Fields(dsn) {
		k, v, _ := strings.Cut(kv, "=")
		switch k {
		case "host":
			c.Host = v
		case "port":
			c.Port = v
		case "user":
			c.User = v
		case "password":
			c.Password = v
		case "dbname":
			c.Name = v
		case "sslmode":
			c.SSLMode = configs.SSLMode(v)
		}
	}
	return c
}

// TestSlowQueryIsStopped: a query longer than the statement timeout is
// stopped by PostgreSQL, while Long lets background work run longer.
func TestSlowQueryIsStopped(t *testing.T) {
	c := testConfig(t)
	c.StatementTimeout = 300 * time.Millisecond
	db, err := gorm.Open(postgres.Open(DSN(c)), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	ctx := context.Background()
	if err := db.WithContext(ctx).Exec("SELECT pg_sleep(1)").Error; err == nil || !strings.Contains(err.Error(), "statement timeout") {
		t.Fatalf("a one second query was not stopped: %v", err)
	}
	err = Long(ctx, db, 5*time.Second, func(tx *gorm.DB) error { return tx.Exec("SELECT pg_sleep(1)").Error })
	if err != nil {
		t.Fatalf("background work was stopped: %v", err)
	}
	// The raised limit stayed inside that transaction.
	var limit string
	if err := db.WithContext(ctx).Raw("SHOW statement_timeout").Scan(&limit).Error; err != nil || limit != "300ms" {
		t.Fatalf("statement_timeout after Long = %q %v", limit, err)
	}
}
