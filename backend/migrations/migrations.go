// Package migrations embeds and applies the SQL schema migrations.
package migrations

import (
	"context"
	"database/sql"
	"embed"
	"fmt"

	"github.com/pressly/goose/v3"
)

//go:embed *.sql
var fsys embed.FS

// Run repairs indexes a stopped concurrent build left unusable, then
// applies all pending migrations.
func Run(db *sql.DB) error {
	if err := RepairIndexes(context.Background(), db); err != nil {
		return err
	}
	goose.SetBaseFS(fsys)
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("migration dialect could not be set: %w", err)
	}
	if err := goose.Up(db, "."); err != nil {
		return fmt.Errorf("migrations could not be applied: %w", err)
	}
	return nil
}
