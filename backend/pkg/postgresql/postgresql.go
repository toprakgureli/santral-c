// Package postgresql owns the shared GORM database handle.
package postgresql

import (
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
		conn, openErr := gorm.Open(postgres.Open(fmt.Sprintf(dsn,
			c.Host, c.Port, c.User, c.Password, c.Name, string(c.SSLMode), c.TimeZone,
		)), &gorm.Config{Logger: gormlogger.Default.LogMode(level)})
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

// Get returns the shared database handle.
func Get() *gorm.DB { return db }
