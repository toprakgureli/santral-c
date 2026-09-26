package whatsapp

import (
	"encoding/json"
	"fmt"
	"regexp"
	"time"
)

// TimeSpan is a stretch of the week in Turkey time: some weekdays (0 is
// Monday, none means every day) from one clock time to another. A span that
// ends earlier than it starts runs past midnight, 22:00 to 06:00 for
// example; the days then say which evenings it starts on.
type TimeSpan struct {
	Days []int  `json:"days"`
	From string `json:"from"`
	To   string `json:"to"`
}

var clockRe = regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d$`)

func validClock(s string) bool { return clockRe.MatchString(s) }

func (sp TimeSpan) onDay(idx int) bool {
	if len(sp.Days) == 0 {
		return true
	}
	for _, d := range sp.Days {
		if d == idx {
			return true
		}
	}
	return false
}

// Covers reports whether t falls inside the span.
func (sp TimeSpan) Covers(t time.Time) bool {
	if !validClock(sp.From) || !validClock(sp.To) {
		return false
	}
	lt := t.In(istanbul)
	idx := (int(lt.Weekday()) + 6) % 7
	m := lt.Hour()*60 + lt.Minute()
	from, to := minuteOf(sp.From), minuteOf(sp.To)
	switch {
	case from == to: // the whole day
		return sp.onDay(idx)
	case from < to:
		return sp.onDay(idx) && m >= from && m < to
	}
	// past midnight: the evening part belongs to today, the morning part to
	// the day before
	if m >= from {
		return sp.onDay(idx)
	}
	return m < to && sp.onDay((idx+6)%7)
}

func (sp TimeSpan) check() error {
	if !validClock(sp.From) || !validClock(sp.To) {
		return fmt.Errorf("saatler 09:00 gibi yazılmalı")
	}
	for _, d := range sp.Days {
		if d < 0 || d > 6 {
			return fmt.Errorf("gün tanınmadı")
		}
	}
	return nil
}

// BotSchedule says when a chatbot answers: always, in the device's working
// hours, outside them, or in hours picked for this chatbot.
type BotSchedule struct {
	Mode  string     `json:"mode"` // always | hours | off_hours | custom
	Spans []TimeSpan `json:"spans"`
}

func parseSchedule(raw string) BotSchedule {
	var sc BotSchedule
	_ = json.Unmarshal([]byte(raw), &sc)
	switch sc.Mode {
	case "hours", "off_hours", "custom":
	default:
		sc.Mode = "always"
	}
	if sc.Spans == nil {
		sc.Spans = []TimeSpan{}
	}
	return sc
}

func (sc BotSchedule) check() error {
	if sc.Mode != "custom" {
		return nil
	}
	if len(sc.Spans) == 0 {
		return fmt.Errorf("en az bir saat aralığı ekleyin")
	}
	for _, sp := range sc.Spans {
		if err := sp.check(); err != nil {
			return err
		}
	}
	return nil
}

// Fits reports whether the chatbot answers at t on a device with these
// working hours.
func (sc BotSchedule) Fits(h HoursSettings, t time.Time) bool {
	switch sc.Mode {
	case "hours":
		return !h.Enabled || h.Open(t)
	case "off_hours":
		return h.Enabled && !h.Open(t)
	case "custom":
		for _, sp := range sc.Spans {
			if sp.Covers(t) {
				return true
			}
		}
		return false
	}
	return true
}
