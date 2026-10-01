// Package testdb gives integration tests a real PostgreSQL database.
//
// Tests that need one call Open. They run only when SANTRAL_TEST_DSN names a
// database (for example
// "host=localhost user=santral password=... dbname=santral_test sslmode=disable"),
// so the plain unit tests still run anywhere. The schema is brought up to
// date first; each test cleans up the rows it adds.
package testdb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

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
		if err := migrate(sqlDB); err != nil {
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

// migrateLock keeps test binaries of different packages, which run at the
// same time, from bringing the schema up to date together.
const migrateLock = 731_0012

func migrate(db *sql.DB) error {
	ctx := context.Background()
	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("test database connection: %w", err)
	}
	defer func() { _ = conn.Close() }()
	// Wait for the lock by asking again and again rather than with a
	// blocking call: a waiting statement holds a snapshot, and the
	// migrations that build indexes concurrently wait for every snapshot to
	// end, so a blocked waiter and the lock holder would wait on each other
	// for ever.
	deadline := time.Now().Add(10 * time.Minute)
	for {
		var got bool
		if err := conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock($1)", migrateLock).Scan(&got); err != nil {
			return fmt.Errorf("migration lock: %w", err)
		}
		if got {
			break
		}
		if time.Now().After(deadline) {
			return errors.New("migration lock: another test binary held it for ten minutes")
		}
		time.Sleep(200 * time.Millisecond)
	}
	defer func() { _, _ = conn.ExecContext(ctx, "SELECT pg_advisory_unlock($1)", migrateLock) }()
	return migrations.Run(db)
}
