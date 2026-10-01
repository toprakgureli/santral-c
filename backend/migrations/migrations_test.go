package migrations_test

import (
	"context"
	"testing"

	"github.com/pressly/goose/v3"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/toprakgureli/santral-c/backend/internal/testdb"
	"github.com/toprakgureli/santral-c/backend/migrations"
)

// TestMigrationsFromEmpty brings a brand-new database up to date, the way
// a fresh install does, and checks that no index was left half built: an
// index built concurrently that failed stays behind marked invalid, is
// never used, and blocks the next try with the same name.
func TestMigrationsFromEmpty(t *testing.T) {
	dsn, _ := testdb.Scratch(t, "santral_migrate")
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	goose.SetLogger(goose.NopLogger())
	if err := migrations.Run(sqlDB); err != nil {
		t.Fatalf("migrations on an empty database: %v", err)
	}
	// A second run finds nothing to do.
	if err := migrations.Run(sqlDB); err != nil {
		t.Fatalf("migrations on an up-to-date database: %v", err)
	}

	ctx := context.Background()
	rows, err := sqlDB.QueryContext(ctx, `SELECT c.relname FROM pg_index i JOIN pg_class c ON c.oid = i.indexrelid
		WHERE NOT i.indisvalid OR NOT i.indisready`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		t.Errorf("index %s is invalid after the migrations", name)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}

	// The tables the panel cannot start without are there.
	for _, table := range []string{"users", "roles", "permissions", "sessions", "call_logs", "wa_channels", "wa_messages", "chat_messages", "backup_runs"} {
		var found bool
		if err := sqlDB.QueryRowContext(ctx, "SELECT to_regclass($1) IS NOT NULL", "public."+table).Scan(&found); err != nil {
			t.Fatal(err)
		}
		if !found {
			t.Errorf("table %s is missing after the migrations", table)
		}
	}
	var applied int
	if err := sqlDB.QueryRowContext(ctx, "SELECT count(*) FROM goose_db_version WHERE is_applied AND version_id > 0").Scan(&applied); err != nil {
		t.Fatal(err)
	}
	if applied == 0 {
		t.Error("goose recorded no migration as applied")
	}
}
