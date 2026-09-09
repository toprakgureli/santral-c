// Package postgresql owns the shared GORM database handle.
package postgresql

import (
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
		sqlDB.SetMaxIdleConns(10)
		sqlDB.SetMaxOpenConns(100)
		sqlDB.SetConnMaxLifetime(5 * time.Minute)
		sqlDB.SetConnMaxIdleTime(5 * time.Minute)
		if pingErr := sqlDB.Ping(); pingErr != nil {
			err = fmt.Errorf("database could not be pinged: %w", pingErr)
			return
		}
		db = conn
	})
	return err
}

// Get returns the shared database handle.
func Get() *gorm.DB { return db }
