package shift

import (
	"testing"
	"time"
)

func TestAutoEndFor(t *testing.T) {
	cases := []struct {
		name    string
		started string
		want    string
	}{
		{"morning start closes the same evening", "2026-09-14T09:05:00+03:00", "2026-09-14T19:20:00+03:00"},
		{"start right before the cutoff closes that evening", "2026-09-14T19:19:59+03:00", "2026-09-14T19:20:00+03:00"},
		{"start at the cutoff rolls to the next evening", "2026-09-14T19:20:00+03:00", "2026-09-15T19:20:00+03:00"},
		{"late night start rolls to the next evening", "2026-09-14T23:30:00+03:00", "2026-09-15T19:20:00+03:00"},
		{"UTC input is read in Istanbul time", "2026-09-14T16:30:00Z", "2026-09-15T19:20:00+03:00"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			started, err := time.Parse(time.RFC3339, tc.started)
			if err != nil {
				t.Fatal(err)
			}
			want, err := time.Parse(time.RFC3339, tc.want)
			if err != nil {
				t.Fatal(err)
			}
			if got := autoEndFor(started); !got.Equal(want) {
				t.Fatalf("autoEndFor(%s) = %s, want %s", tc.started, got.Format(time.RFC3339), want.Format(time.RFC3339))
			}
		})
	}
}

func TestReminderFollowsAutoEnd(t *testing.T) {
	cases := []struct{ started, want string }{
		{"2026-09-14T09:05:00+03:00", "2026-09-14T18:30:00+03:00"},
		{"2026-09-14T23:30:00+03:00", "2026-09-15T18:30:00+03:00"},
	}
	for _, tc := range cases {
		started, _ := time.Parse(time.RFC3339, tc.started)
		want, _ := time.Parse(time.RFC3339, tc.want)
		if got := at(autoEndFor(started), reminderHour, reminderMinute); !got.Equal(want) {
			t.Fatalf("reminder for %s = %s, want %s", tc.started, got.Format(time.RFC3339), want.Format(time.RFC3339))
		}
	}
}
