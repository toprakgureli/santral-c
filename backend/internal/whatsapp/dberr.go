package whatsapp

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"runtime"
)

// warnDB logs a database error the caller chose not to return: a side
// effect or a best-effort mark whose failure must not undo the main work,
// but must not go unnoticed either. The log names the calling line.
func warnDB(ctx context.Context, err error) {
	if err == nil {
		return
	}
	at := "unknown"
	if _, file, line, ok := runtime.Caller(1); ok {
		at = fmt.Sprintf("%s:%d", filepath.Base(file), line)
	}
	slog.WarnContext(ctx, "whatsapp database step failed", "at", at, "error", err)
}
