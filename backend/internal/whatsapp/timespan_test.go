package whatsapp

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
)

// trAt is a moment in Turkey time; 2026-09-28 is a Monday.
func trAt(day int, hh, mm int) time.Time {
	return time.Date(2026, 9, 28+day, hh, mm, 0, 0, istanbul)
}

func TestTimeSpanCovers(t *testing.T) {
	weekdays := []int{0, 1, 2, 3, 4}
	cases := []struct {
		name string
		sp   TimeSpan
		t    time.Time
		want bool
	}{
		{"inside", TimeSpan{Days: weekdays, From: "09:00", To: "18:00"}, trAt(0, 10, 0), true},
		{"end is outside", TimeSpan{Days: weekdays, From: "09:00", To: "18:00"}, trAt(0, 18, 0), false},
		{"weekend", TimeSpan{Days: weekdays, From: "09:00", To: "18:00"}, trAt(5, 10, 0), false},
		{"every day", TimeSpan{From: "12:00", To: "13:30"}, trAt(6, 12, 45), true},
		{"night, evening part", TimeSpan{Days: []int{4}, From: "22:00", To: "06:00"}, trAt(4, 23, 0), true},
		{"night, morning after", TimeSpan{Days: []int{4}, From: "22:00", To: "06:00"}, trAt(5, 5, 30), true},
		{"night, wrong morning", TimeSpan{Days: []int{4}, From: "22:00", To: "06:00"}, trAt(4, 5, 30), false},
		{"whole day", TimeSpan{Days: []int{6}, From: "00:00", To: "00:00"}, trAt(6, 15, 0), true},
		{"utc input", TimeSpan{From: "09:00", To: "10:00"}, time.Date(2026, 9, 28, 6, 30, 0, 0, time.UTC), true},
		{"bad clock", TimeSpan{From: "9", To: "10:00"}, trAt(0, 9, 30), false},
	}
	for _, c := range cases {
		if got := c.sp.Covers(c.t); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestBotScheduleFits(t *testing.T) {
	var h HoursSettings
	h.Enabled = true
	for i := 0; i < 5; i++ {
		h.Days[i] = DayOpen{Open: true, From: "09:00", To: "18:00"}
	}
	work, night := trAt(0, 10, 0), trAt(0, 20, 0)
	if !(BotSchedule{Mode: "hours"}).Fits(h, work) || (BotSchedule{Mode: "hours"}).Fits(h, night) {
		t.Error("hours mode should follow working hours")
	}
	if (BotSchedule{Mode: "off_hours"}).Fits(h, work) || !(BotSchedule{Mode: "off_hours"}).Fits(h, night) {
		t.Error("off_hours mode should be the opposite")
	}
	lunch := BotSchedule{Mode: "custom", Spans: []TimeSpan{{From: "12:00", To: "13:00"}}}
	if lunch.Fits(h, work) || !lunch.Fits(h, trAt(0, 12, 30)) {
		t.Error("custom mode should follow its spans")
	}
	if (BotSchedule{Mode: "custom"}).check() == nil {
		t.Error("custom mode without spans should be refused")
	}
}

func TestConditionTimeBetween(t *testing.T) {
	g := &BotGraph{
		Nodes: []BotNode{
			{ID: "s", Type: "start"},
			{ID: "c", Type: "condition", Data: BotData{Rules: []BotRule{{Op: "time_between", From: "12:00", To: "13:30", Days: []int{0, 1, 2, 3, 4}}}}},
			{ID: "y", Type: "message", Data: BotData{Text: "öğle arası"}},
			{ID: "n", Type: "message", Data: BotData{Text: "buyrun"}},
		},
		Edges: []BotEdge{{From: "s", To: "c"}, {From: "c", Port: "yes", To: "y"}, {From: "c", Port: "no", To: "n"}},
	}
	for _, c := range []struct {
		t    time.Time
		want string
	}{{trAt(1, 12, 15), "öğle arası"}, {trAt(1, 14, 0), "buyrun"}, {trAt(6, 12, 15), "buyrun"}} {
		io := &simIO{at: c.t}
		step(g, &botState{Vars: map[string]string{}}, nil, io)
		if len(io.Out) == 0 || io.Out[0].Text != c.want {
			t.Errorf("at %v: got %+v, want %q", c.t, io.Out, c.want)
		}
	}
}

func TestSimGateSunday(t *testing.T) {
	var h HoursSettings
	h.Enabled = true
	for i := 0; i < 6; i++ {
		h.Days[i] = DayOpen{Open: true, From: "09:00", To: "18:00"}
	}
	sunday := trAt(6, 11, 0) // closed all day
	open := h.Open(sunday)
	if open {
		t.Fatal("Sunday should be outside working hours")
	}
	always := &models.WABot{Trigger: "entry", Schedule: `{"mode":"always"}`}
	if skip, note := simGate(always, h, open, sunday, "Destek"); skip != "" || note == "" {
		t.Errorf("always-on bot should greet with a note, got skip=%q note=%q", skip, note)
	}
	inHours := &models.WABot{Trigger: "entry", Schedule: `{"mode":"hours"}`}
	if skip, _ := simGate(inHours, h, open, sunday, "Destek"); skip == "" {
		t.Error("working-hours bot should not greet on Sunday")
	}
	after := &models.WABot{Trigger: "after_hours"}
	if skip, note := simGate(after, h, open, sunday, "Destek"); skip != "" || note != "" {
		t.Error("after-hours bot should greet on Sunday")
	}
	monday := trAt(0, 11, 0)
	if skip, _ := simGate(after, h, h.Open(monday), monday, "Destek"); skip == "" {
		t.Error("after-hours bot should not greet on Monday morning")
	}
}

func TestRatingAnswer(t *testing.T) {
	var m hookMessage
	if err := json.Unmarshal([]byte(`{"type":"interactive","interactive":{"type":"list_reply","list_reply":{"id":"opt:rate-42-5","title":"5 - Çok iyi"}}}`), &m); err != nil {
		t.Fatal(err)
	}
	if tid, score, ok := ratingAnswer(&m); !ok || tid != 42 || score != 5 {
		t.Errorf("got %d %d %v", tid, score, ok)
	}
	_ = json.Unmarshal([]byte(`{"type":"interactive","interactive":{"type":"list_reply","list_reply":{"id":"opt:o2","title":"Satış"}}}`), &m)
	if _, _, ok := ratingAnswer(&m); ok {
		t.Error("a menu choice is not a rating")
	}
}
