// Package postgresql owns the shared GORM database handle.
package postgresql

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/toprakgureli/santral-c/backend/configs"
)

const dsn string = "host=%s port=%s user=%s password=%s dbname=%s sslmode=%s TimeZone=%s"

// MigratorName is the application name the migration connection shows in
// pg_stat_activity; deploy.sh looks for it to tell a long migration from a
// server that does not come up.
const MigratorName = "santral-migrate"

// DSN is the connection string of the server's own pool. Every query on it
// is stopped by PostgreSQL after the configured statement timeout, so a
// runaway query cannot hold a connection and a request for ever.
func DSN(c configs.Database) string {
	base := fmt.Sprintf(dsn, c.Host, c.Port, c.User, c.Password, c.Name, string(c.SSLMode), c.TimeZone)
	if c.StatementTimeout > 0 {
		base += fmt.Sprintf(" statement_timeout=%d", c.StatementTimeout.Milliseconds())
	}
	return base + " application_name=santral"
}

// MigratorDSN is the connection string for the migrations: no statement
// timeout (an index on a big table may take minutes) and its own name.
func MigratorDSN(c configs.Database) string {
	return fmt.Sprintf(dsn, c.Host, c.Port, c.User, c.Password, c.Name, string(c.SSLMode), c.TimeZone) +
		" statement_timeout=0 application_name=" + MigratorName
}

// OpenMigrator opens a small separate pool for the migrations. The caller
// closes it when they are done.
func OpenMigrator(c configs.Database) (*sql.DB, error) {
	conn, err := gorm.Open(postgres.Open(MigratorDSN(c)), &gorm.Config{Logger: gormlogger.Default.LogMode(gormlogger.Silent)})
	if err != nil {
		return nil, fmt.Errorf("migration connection could not be opened: %w", err)
	}
	sqlDB, err := conn.DB()
	if err != nil {
		return nil, fmt.Errorf("migration handle could not be retrieved: %w", err)
	}
	sqlDB.SetMaxOpenConns(2)
	return sqlDB, nil
}

var (
	db   *gorm.DB
	once sync.Once
)

// Connect opens the shared database once and verifies it.
func Connect(c configs.Database) error {
	var err error
	once.Do(func() {
		level := gormlogger.Silent
		if c.Debug {
			level = gormlogger.Info
		}
		conn, openErr := gorm.Open(postgres.Open(DSN(c)), &gorm.Config{Logger: gormlogger.Default.LogMode(level)})
		if openErr != nil {
			err = fmt.Errorf("database connection could not be opened: %w", openErr)
			return
		}
		sqlDB, handleErr := conn.DB()
		if handleErr != nil {
			err = fmt.Errorf("database handle could not be retrieved: %w", handleErr)
			return
		}
		Pool(sqlDB, c.MaxConns)
		if pingErr := sqlDB.Ping(); pingErr != nil {
			err = fmt.Errorf("database could not be pinged: %w", pingErr)
			return
		}
		db = conn
	})
	return err
}

// defaultMaxConns is used when the configuration names no limit.
const defaultMaxConns = 40

// Pool sets how many connections the server keeps and opens at most. A
// request that finds every connection busy waits for one to come free.
func Pool(sqlDB *sql.DB, maxConns int) {
	if maxConns <= 0 {
		maxConns = defaultMaxConns
	}
	sqlDB.SetMaxOpenConns(maxConns)
	sqlDB.SetMaxIdleConns(min(10, maxConns))
	sqlDB.SetConnMaxLifetime(5 * time.Minute)
	sqlDB.SetConnMaxIdleTime(5 * time.Minute)
}

// Long runs fn in a transaction whose statements may each take up to limit
// instead of the pool's statement timeout. It is for background work that
// legitimately runs long (clearing a big table the first time), never for
// a request.
func Long(ctx context.Context, db *gorm.DB, limit time.Duration, fn func(tx *gorm.DB) error) error {
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(fmt.Sprintf("SET LOCAL statement_timeout = %d", limit.Milliseconds())).Error; err != nil {
			return fmt.Errorf("statement timeout could not be raised: %w", err)
		}
		return fn(tx)
	})
}

// Get returns the shared database handle.
func Get() *gorm.DB { return db }
