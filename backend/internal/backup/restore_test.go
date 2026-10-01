package backup

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/pressly/goose/v3"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/toprakgureli/santral-c/backend/configs"
	"github.com/toprakgureli/santral-c/backend/internal/testdb"
	"github.com/toprakgureli/santral-c/backend/migrations"
)

// pgRestore is the program that reads a dump back; SANTRAL_PG_RESTORE may
// name another path, as SANTRAL_PG_DUMP does for pg_dump.
func pgRestore() string {
	if p := strings.TrimSpace(os.Getenv("SANTRAL_PG_RESTORE")); p != "" {
		return p
	}
	return "pg_restore"
}

// openScratch opens a scratch database made by testdb.Scratch.
func openScratch(t *testing.T, dsn string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}

// databaseConfig is the server configuration for a key=value address.
func databaseConfig(dsn string) configs.Database {
	cfg := configs.Database{
		Host:     testdb.Field(dsn, "host"),
		Port:     testdb.Field(dsn, "port"),
		User:     testdb.Field(dsn, "user"),
		Password: testdb.Field(dsn, "password"),
		Name:     testdb.Field(dsn, "dbname"),
		SSLMode:  "disable",
	}
	if cfg.Host == "" {
		cfg.Host = "localhost"
	}
	if cfg.Port == "" {
		cfg.Port = "5432"
	}
	return cfg
}

// TestBackupRestores is the drill the backups exist for: a database with
// the full schema and some rows in it is copied with the real pg_dump the
// server uses, the copy is read back with pg_restore into an empty
// database, and both hold the same rows. It runs where pg_dump and
// pg_restore are installed (CI installs them).
func TestBackupRestores(t *testing.T) {
	for _, program := range []string{pgDump(), pgRestore()} {
		if _, err := lookPath(program); err != nil {
			t.Skipf("%s is not installed here", program)
		}
	}
	srcDSN, _ := testdb.Scratch(t, "santral_backup_src")
	src := openScratch(t, srcDSN)
	sqlSrc, err := src.DB()
	if err != nil {
		t.Fatal(err)
	}
	goose.SetLogger(goose.NopLogger())
	if err := migrations.Run(sqlSrc); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO users (name, email, password, active, mfa_exempt, created_at, updated_at)
		 SELECT 'Yedek Kişi ' || g, 'restore-' || g || '@backup-test.local', '-', g % 3 <> 0, true, now(), now() FROM generate_series(1, 40) g`,
		`INSERT INTO contacts (name, company) SELECT 'Müşteri ' || g, 'Firma ' || (g % 7) FROM generate_series(1, 250) g`,
		`INSERT INTO call_logs (call_id, user_id, direction, peer_number, peer_key, disposition, started_at, ended_at, duration_seconds, hooks_done)
		 SELECT 'restore-' || g, u.id, 'outbound', '0555' || lpad(g::text, 7, '0'), '555' || lpad(g::text, 7, '0'), 'answered',
		        now() - g * interval '1 hour', now() - g * interval '1 hour' + interval '2 minutes', 120, true
		 FROM generate_series(1, 500) g JOIN users u ON u.email = 'restore-' || (1 + g % 40) || '@backup-test.local'`,
		`INSERT INTO system_settings (key, value, updated_at) VALUES ('restore_check', 'şğüİ "quoted" value', now())`,
	} {
		if err := src.Exec(q).Error; err != nil {
			t.Fatalf("source rows: %v", err)
		}
	}

	ctx := context.Background()
	dir, path, err := dump(ctx, databaseConfig(srcDSN))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	dstDSN, dstName := testdb.Scratch(t, "santral_backup_dst")
	dst := databaseConfig(dstDSN)
	cmd := exec.CommandContext(ctx, pgRestore(), "--no-owner", "--no-privileges", "--exit-on-error", //nolint:gosec // the test's own program and file
		"--host="+dst.Host, "--port="+dst.Port, "--username="+dst.User, "--dbname="+dstName, path)
	cmd.Env = append(os.Environ(), "PGPASSWORD="+dst.Password, "PGSSLMODE=disable")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("pg_restore: %v %s", err, stderr.String())
	}

	restored := openScratch(t, dstDSN)
	for _, table := range []string{"users", "contacts", "call_logs", "system_settings", "permissions", "goose_db_version"} {
		var want, got int64
		if err := src.Table(table).Count(&want).Error; err != nil {
			t.Fatal(err)
		}
		if err := restored.Table(table).Count(&got).Error; err != nil {
			t.Fatalf("%s in the restored copy: %v", table, err)
		}
		if got != want {
			t.Errorf("%s: %d rows restored, the source holds %d", table, got, want)
		}
	}
	var value string
	if err := restored.Raw("SELECT value FROM system_settings WHERE key = 'restore_check'").Scan(&value).Error; err != nil || value != `şğüİ "quoted" value` {
		t.Errorf("restored text = %q, %v", value, err)
	}
	// The restored copy keeps working: its id sequences continue after the
	// restored rows instead of colliding with them.
	if err := restored.Exec("INSERT INTO contacts (name) VALUES ('Geri yüklemeden sonra')").Error; err != nil {
		t.Errorf("a new row in the restored copy: %v", err)
	}
}
