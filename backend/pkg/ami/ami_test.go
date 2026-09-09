package ami

import (
	"bufio"
	"strings"
	"testing"
)

func TestReadMessage(t *testing.T) {
	raw := "Event: Hangup\r\nChannel: PJSIP/1001-00000001\r\nLinkedid: 1699999999.1\r\nCause: 16\r\n\r\n"
	msg, err := readMessage(bufio.NewReader(strings.NewReader(raw)))
	if err != nil {
		t.Fatalf("readMessage error: %v", err)
	}
	if msg.Name() != "Hangup" {
		t.Fatalf("Name = %q, want Hangup", msg.Name())
	}
	if got := msg.Get("linkedid"); got != "1699999999.1" {
		t.Fatalf("case-insensitive Get = %q, want 1699999999.1", got)
	}
	if got := msg.Get("Cause"); got != "16" {
		t.Fatalf("Cause = %q, want 16", got)
	}
}

func TestReadMessageSkipsLeadingBlankLines(t *testing.T) {
	raw := "\r\n\r\nEvent: Newchannel\r\nChannel: PJSIP/1002-2\r\n\r\n"
	msg, err := readMessage(bufio.NewReader(strings.NewReader(raw)))
	if err != nil {
		t.Fatalf("readMessage error: %v", err)
	}
	if msg.Name() != "Newchannel" {
		t.Fatalf("Name = %q, want Newchannel", msg.Name())
	}
}

func TestWriteAction(t *testing.T) {
	var sb strings.Builder
	w := bufio.NewWriter(&sb)
	if err := writeAction(w, map[string]string{"Action": "Ping"}); err != nil {
		t.Fatalf("writeAction error: %v", err)
	}
	out := sb.String()
	if !strings.Contains(out, "Action: Ping\r\n") || !strings.HasSuffix(out, "\r\n\r\n") {
		t.Fatalf("unexpected serialization: %q", out)
	}
}
