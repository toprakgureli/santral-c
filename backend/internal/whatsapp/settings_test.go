package whatsapp

import (
	"testing"
	"time"
)

func at(s string) time.Time {
	t, err := time.ParseInLocation("2006-01-02 15:04", s, istanbul)
	if err != nil {
		panic(err)
	}
	return t
}

func TestHoursElapsedCountsOnlyWorkingTime(t *testing.T) {
	h := defaultSettings().Hours
	h.Enabled = true
	// 2026-09-25 is a Friday.
	cases := []struct {
		from, to string
		want     time.Duration
	}{
		{"2026-09-25 10:00", "2026-09-25 10:20", 20 * time.Minute},
		{"2026-09-25 17:50", "2026-09-28 09:10", 20 * time.Minute}, // over the weekend
		{"2026-09-25 22:00", "2026-09-26 12:00", 0},                // Saturday is closed
		{"2026-09-28 07:00", "2026-09-28 09:15", 15 * time.Minute},
	}
	for _, c := range cases {
		if got := h.Elapsed(at(c.from), at(c.to)); got != c.want {
			t.Errorf("Elapsed(%s, %s) = %v, want %v", c.from, c.to, got, c.want)
		}
	}
	h.Holidays = []string{"2026-09-28"}
	if got := h.Elapsed(at("2026-09-28 09:00"), at("2026-09-28 12:00")); got != 0 {
		t.Errorf("a holiday must not count, got %v", got)
	}
	if h.Open(at("2026-09-28 10:00")) {
		t.Error("a holiday must be closed")
	}
	if !h.Open(at("2026-09-25 09:00")) || h.Open(at("2026-09-25 18:00")) {
		t.Error("the day's edges are wrong")
	}
	h.Enabled = false
	if got := h.Elapsed(at("2026-09-26 10:00"), at("2026-09-26 10:30")); got != 30*time.Minute {
		t.Errorf("without hours every minute counts, got %v", got)
	}
}
