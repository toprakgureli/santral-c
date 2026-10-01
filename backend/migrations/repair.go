package migrations

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
)

// RepairIndexes rebuilds the indexes a cut-off concurrent build left
// unusable. CREATE INDEX CONCURRENTLY that is stopped half way (a deploy
// that gave up waiting, a restart) leaves an index PostgreSQL marks
// invalid: it is never used for reads, yet IF NOT EXISTS skips it, so the
// migration that should make it would never make it again.
//
// A leftover of a stopped REINDEX (named *_ccnew or *_ccold) is dropped,
// as PostgreSQL advises; any other invalid index is rebuilt in place, and
// dropped when even that fails (a unique index over duplicate rows, for
// example) so its migration can make it again or say why it cannot. Only
// one server runs at a time, so no index is being built by it while this
// runs.
func RepairIndexes(ctx context.Context, db *sql.DB) error {
	rows, err := db.QueryContext(ctx, `
		SELECT quote_ident(n.nspname) || '.' || quote_ident(c.relname), c.relname
		FROM pg_index i
		JOIN pg_class c ON c.oid = i.indexrelid
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE NOT i.indisvalid AND n.nspname = current_schema()
		ORDER BY c.relname`)
	if err != nil {
		return fmt.Errorf("invalid indexes could not be listed: %w", err)
	}
	type index struct{ qualified, name string }
	var list []index
	for rows.Next() {
		var ix index
		if err := rows.Scan(&ix.qualified, &ix.name); err != nil {
			_ = rows.Close()
			return fmt.Errorf("invalid index could not be read: %w", err)
		}
		list = append(list, ix)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("invalid indexes could not be listed: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("invalid indexes could not be listed: %w", err)
	}
	for _, ix := range list {
		if leftover(ix.name) {
			slog.WarnContext(ctx, "dropping the leftover of a stopped index rebuild", "index", ix.name)
			if _, err := db.ExecContext(ctx, "DROP INDEX CONCURRENTLY IF EXISTS "+ix.qualified); err != nil {
				return fmt.Errorf("leftover index %s could not be dropped: %w", ix.name, err)
			}
			continue
		}
		slog.WarnContext(ctx, "rebuilding an index a stopped build left unusable", "index", ix.name)
		_, err := db.ExecContext(ctx, "REINDEX INDEX CONCURRENTLY "+ix.qualified)
		if err == nil {
			continue
		}
		slog.ErrorContext(ctx, "the index could not be rebuilt; dropping it so its migration can make it again", "index", ix.name, "error", err)
		// A failed REINDEX CONCURRENTLY leaves its own *_ccnew copy.
		if _, err := db.ExecContext(ctx, "DROP INDEX CONCURRENTLY IF EXISTS "+ix.qualified); err != nil {
			return fmt.Errorf("invalid index %s could not be dropped: %w", ix.name, err)
		}
	}
	if len(list) > 0 {
		// A failed rebuild may have left a copy of its own.
		return dropLeftovers(ctx, db)
	}
	return nil
}

// dropLeftovers removes the *_ccnew and *_ccold copies a failed rebuild
// leaves behind.
func dropLeftovers(ctx context.Context, db *sql.DB) error {
	rows, err := db.QueryContext(ctx, `
		SELECT quote_ident(n.nspname) || '.' || quote_ident(c.relname), c.relname
		FROM pg_index i
		JOIN pg_class c ON c.oid = i.indexrelid
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE NOT i.indisvalid AND n.nspname = current_schema()`)
	if err != nil {
		return fmt.Errorf("invalid indexes could not be listed: %w", err)
	}
	var drop []string
	for rows.Next() {
		var qualified, name string
		if err := rows.Scan(&qualified, &name); err != nil {
			_ = rows.Close()
			return fmt.Errorf("invalid index could not be read: %w", err)
		}
		if leftover(name) {
			drop = append(drop, qualified)
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("invalid indexes could not be listed: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("invalid indexes could not be listed: %w", err)
	}
	for _, q := range drop {
		if _, err := db.ExecContext(ctx, "DROP INDEX CONCURRENTLY IF EXISTS "+q); err != nil {
			return fmt.Errorf("leftover index %s could not be dropped: %w", q, err)
		}
	}
	return nil
}

// leftover reports whether an index name is the temporary copy of a
// concurrent rebuild (name_ccnew, name_ccnew1, name_ccold, ...).
func leftover(name string) bool {
	for _, suffix := range []string{"_ccnew", "_ccold"} {
		i := strings.LastIndex(name, suffix)
		if i < 0 {
			continue
		}
		rest := name[i+len(suffix):]
		if strings.Trim(rest, "0123456789") == "" {
			return true
		}
	}
	return false
}
