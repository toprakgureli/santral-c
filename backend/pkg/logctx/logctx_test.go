package logctx

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"
)

func TestHandlerAddsRequestAndUser(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(NewHandler(slog.NewJSONHandler(&buf, nil)))
	ctx := WithUser(WithRequest(context.Background(), "req-1"), 42)
	log.InfoContext(ctx, "hello")

	var line map[string]any
	if err := json.Unmarshal(buf.Bytes(), &line); err != nil {
		t.Fatal(err)
	}
	if line["request_id"] != "req-1" || line["user_id"] != float64(42) {
		t.Fatalf("line = %v", line)
	}

	buf.Reset()
	log.Info("no context")
	if bytes.Contains(buf.Bytes(), []byte("request_id")) {
		t.Fatal("a line without context got a request id")
	}
}
