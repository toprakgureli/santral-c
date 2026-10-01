package backup

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/toprakgureli/santral-c/backend/configs"
)

// lookPath finds a program on the PATH, and dumpDatabase writes the dump;
// tests replace them.
var (
	lookPath     = exec.LookPath
	dumpDatabase = dump
)

// pgDump is the program that writes the dump; SANTRAL_PG_DUMP may name
// another path (a version that matches the server, for example).
func pgDump() string {
	if p := strings.TrimSpace(os.Getenv("SANTRAL_PG_DUMP")); p != "" {
		return p
	}
	return "pg_dump"
}

// dump writes the whole database in PostgreSQL's custom format (what
// pg_restore reads) to a new file in a private temporary folder and
// returns its path. The caller removes the folder.
func dump(ctx context.Context, db configs.Database) (dir, path string, err error) {
	dir, err = os.MkdirTemp("", "santral-backup-")
	if err != nil {
		return "", "", fmt.Errorf("temporary folder could not be made: %w", err)
	}
	path = filepath.Join(dir, "santral.dump")
	args := []string{"--format=custom", "--no-owner", "--no-privileges", "--file=" + path,
		"--host=" + db.Host, "--port=" + db.Port, "--username=" + db.User, "--dbname=" + db.Name}
	cmd := exec.CommandContext(ctx, pgDump(), args...) //nolint:gosec // the program and its arguments come from the server's own configuration
	// The password goes through the environment, never the command line.
	cmd.Env = append(os.Environ(), "PGPASSWORD="+db.Password, "PGSSLMODE="+sslMode(db))
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		_ = os.RemoveAll(dir)
		msg := strings.TrimSpace(stderr.String())
		if len(msg) > 400 {
			msg = msg[:400]
		}
		return "", "", fmt.Errorf("pg_dump çalışmadı: %w %s", err, msg)
	}
	return dir, path, nil
}

func sslMode(db configs.Database) string {
	if db.SSLMode == "" {
		return "disable"
	}
	return string(db.SSLMode)
}
