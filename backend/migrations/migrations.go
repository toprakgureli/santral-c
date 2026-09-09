// Package migrations embeds and applies the SQL schema migrations.
package migrations

import (
	"database/sql"
	"embed"
	"fmt"

	"github.com/pressly/goose/v3"
)

//go:embed *.sql
var fsys embed.FS

// Run applies all pending migrations.
func Run(db *sql.DB) error {
	goose.SetBaseFS(fsys)
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("migration dialect could not be set: %w", err)
	}
	if err := goose.Up(db, "."); err != nil {
		return fmt.Errorf("migrations could not be applied: %w", err)
	}
	return nil
}
