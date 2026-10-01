package calllog

import (
	"testing"
	"time"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
)

// Only calls something follows wait for the phone system's record.
func TestHooksMatter(t *testing.T) {
	for _, c := range []struct {
		direction, disposition string
		want                   bool
	}{
		{"outbound", "answered", true},
		{"inbound", "answered", true},
		// the automatic "could not reach" entry
		{"outbound", "no_answer", true},
		{"outbound", "busy", true},
		// a queue call ringing many agents: nothing follows the rings
		{"inbound", "no_answer", false},
		{"inbound", "canceled", false},
		{"internal", "answered", false},
	} {
		if got := hooksMatter(models.CallLog{Direction: c.direction, Disposition: c.disposition}); got != c.want {
			t.Errorf("%s %s: hooksMatter = %v, want %v", c.direction, c.disposition, got, c.want)
		}
	}
}

func TestVerifyBackoff(t *testing.T) {
	want := []time.Duration{30 * time.Second, time.Minute, 2 * time.Minute, 4 * time.Minute, 8 * time.Minute, 16 * time.Minute, 20 * time.Minute, 20 * time.Minute}
	for i, w := range want {
		if got := verifyBackoff(i + 1); got != w {
			t.Errorf("after %d misses: %v, want %v", i+1, got, w)
		}
	}
	if got := verifyBackoff(200); got != 20*time.Minute {
		t.Errorf("after many misses: %v, want the 20 minute cap", got)
	}
}
