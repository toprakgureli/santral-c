// Package testdb gives integration tests a real PostgreSQL database.
//
// Tests that need one call Open. They run only when SANTRAL_TEST_DSN names a
// database (for example
// "host=localhost user=santral password=... dbname=santral_test sslmode=disable"),
// so the plain unit tests still run anywhere. The schema is brought up to
// date first; each test cleans up the rows it adds.
package testdb

import (
	"os"
	"sync"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/toprakgureli/santral-c/backend/migrations"
)

// EnvDSN is the environment variable holding the test database address.
const EnvDSN = "SANTRAL_TEST_DSN"

var (
	once    sync.Once
	shared  *gorm.DB
	openErr error
)

// Open returns the test database, or skips the test when none is set up.
func Open(t testing.TB) *gorm.DB {
	t.Helper()
	dsn := os.Getenv(EnvDSN)
	if dsn == "" {
		t.Skipf("%s is not set; skipping a test that needs PostgreSQL", EnvDSN)
	}
	once.Do(func() {
		db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Discard})
		if err != nil {
			openErr = err
			return
		}
		sqlDB, err := db.DB()
		if err != nil {
			openErr = err
			return
		}
		if err := migrations.Run(sqlDB); err != nil {
			openErr = err
			return
		}
		shared = db
	})
	if openErr != nil {
		t.Fatalf("test database: %v", openErr)
	}
	return shared
}
