package migrations_test

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/toprakgureli/santral-c/backend/internal/testdb"
	"github.com/toprakgureli/santral-c/backend/migrations"
)

func invalidIndexes(t *testing.T, db *sql.DB, table string) []string {
	t.Helper()
	rows, err := db.Query(`SELECT c.relname FROM pg_index i JOIN pg_class c ON c.oid = i.indexrelid
		WHERE NOT i.indisvalid AND i.indrelid = $1::regclass ORDER BY 1`, table)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		out = append(out, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func indexExists(t *testing.T, db *sql.DB, name string) bool {
	t.Helper()
	var n int
	if err := db.QueryRow("SELECT count(*) FROM pg_class WHERE relname = $1 AND relkind = 'i'", name).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n > 0
}

// TestRepairIndexes leaves two indexes the way a stopped concurrent build
// leaves them, then repairs: one that can be built is rebuilt and kept,
// one that cannot (a unique index over duplicates) is dropped, and nothing
// stays invalid.
func TestRepairIndexes(t *testing.T) {
	gdb := testdb.Open(t)
	db, err := gdb.DB()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	table := fmt.Sprintf("repair_check_%d", time.Now().UnixNano()%1_000_000)
	exec := func(q string) {
		t.Helper()
		if _, err := db.ExecContext(ctx, q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	exec("CREATE TABLE " + table + " (id bigserial PRIMARY KEY, v int NOT NULL, w int NOT NULL)")
	t.Cleanup(func() { _, _ = db.Exec("DROP TABLE IF EXISTS " + table) })
	exec("INSERT INTO " + table + " (v, w) SELECT g % 10, g FROM generate_series(1, 300000) g")

	// A unique index over duplicate values fails half way and stays invalid.
	_, err = db.ExecContext(ctx, "CREATE UNIQUE INDEX CONCURRENTLY "+table+"_v_key ON "+table+" (v)")
	if err == nil {
		t.Fatal("the unique index over duplicates was built")
	}
	// A build stopped by a timeout stays invalid too, though it could be
	// built. One connection, so the timeout applies to the build.
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(ctx, "SET statement_timeout = 5"); err != nil {
		t.Fatal(err)
	}
	_, err = conn.ExecContext(ctx, "CREATE INDEX CONCURRENTLY "+table+"_w_idx ON "+table+" (w, v)")
	_, _ = conn.ExecContext(ctx, "RESET statement_timeout")
	_ = conn.Close()
	if err == nil {
		t.Skip("the build finished within 5 ms; it could not be stopped half way here")
	}
	if got := invalidIndexes(t, db, table); len(got) != 2 {
		t.Fatalf("invalid indexes before the repair = %v, want two", got)
	}

	if err := migrations.RepairIndexes(ctx, db); err != nil {
		t.Fatal(err)
	}
	if got := invalidIndexes(t, db, table); len(got) != 0 {
		t.Fatalf("still invalid after the repair: %v", got)
	}
	if !indexExists(t, db, table+"_w_idx") {
		t.Error("the index that could be built was dropped instead of rebuilt")
	}
	if indexExists(t, db, table+"_v_key") {
		t.Error("the unique index over duplicates is still there")
	}
	// Nothing left over from the failed rebuild.
	var left int
	if err := db.QueryRow("SELECT count(*) FROM pg_class WHERE relname LIKE $1", strings.ReplaceAll(table, "_", `\_`)+`%\_cc%`).Scan(&left); err != nil {
		t.Fatal(err)
	}
	if left != 0 {
		t.Errorf("%d leftover copies of a rebuild remain", left)
	}
}
