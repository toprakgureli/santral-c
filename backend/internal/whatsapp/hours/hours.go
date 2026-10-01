// Package hours holds the time rules of the WhatsApp module: a device's
// working week and holidays, time spans on chosen weekdays, and a
// chatbot's schedule, all in Istanbul time.
package hours

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Span is a stretch of the week in Turkey time: some weekdays (0 is
// Monday, none means every day) from one clock time to another. A span that
// ends earlier than it starts runs past midnight, 22:00 to 06:00 for
// example; the days then say which evenings it starts on.
type Span struct {
	Days []int  `json:"days"`
	From string `json:"from"`
	To   string `json:"to"`
}

var clockRe = regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d$`)

// ValidClock reports whether s is a clock time such as 09:30.
func ValidClock(s string) bool { return clockRe.MatchString(s) }

func (sp Span) onDay(idx int) bool {
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
func (sp Span) Covers(t time.Time) bool {
	if !ValidClock(sp.From) || !ValidClock(sp.To) {
		return false
	}
	lt := t.In(Zone)
	idx := (int(lt.Weekday()) + 6) % 7
	m := lt.Hour()*60 + lt.Minute()
	from, to := MinuteOf(sp.From), MinuteOf(sp.To)
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

// Check reports why a span cannot be saved, or nil.
func (sp Span) Check() error {
	if !ValidClock(sp.From) || !ValidClock(sp.To) {
		return fmt.Errorf("saatler 09:00 gibi yazılmalı")
	}
	for _, d := range sp.Days {
		if d < 0 || d > 6 {
			return fmt.Errorf("gün tanınmadı")
		}
	}
	return nil
}

// Schedule says when a chatbot answers: always, in the device's working
// hours, outside them, or in hours picked for this chatbot.
type Schedule struct {
	Mode  string `json:"mode"` // always | hours | off_hours | custom
	Spans []Span `json:"spans"`
}

// ParseSchedule reads a stored schedule; anything unreadable means always.
func ParseSchedule(raw string) Schedule {
	var sc Schedule
	_ = json.Unmarshal([]byte(raw), &sc)
	switch sc.Mode {
	case "hours", "off_hours", "custom":
	default:
		sc.Mode = "always"
	}
	if sc.Spans == nil {
		sc.Spans = []Span{}
	}
	return sc
}

// Check reports why a schedule cannot be saved, or nil.
func (sc Schedule) Check() error {
	if sc.Mode != "custom" {
		return nil
	}
	if len(sc.Spans) == 0 {
		return fmt.Errorf("en az bir saat aralığı ekle")
	}
	for _, sp := range sc.Spans {
		if err := sp.Check(); err != nil {
			return err
		}
	}
	return nil
}

// Fits reports whether the chatbot answers at t on a device with these
// working hours.
func (sc Schedule) Fits(h Week, t time.Time) bool {
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

// Zone is the panel's time zone (no daylight saving).
var Zone = time.FixedZone("+03", 3*3600)

// MinuteOf turns "HH:MM" into minutes after midnight.
func MinuteOf(hhmm string) int {
	var h, m int
	if len(hhmm) >= 4 {
		for i, p := range strings.SplitN(hhmm, ":", 2) {
			n := 0
			for _, r := range p {
				if r >= '0' && r <= '9' {
					n = n*10 + int(r-'0')
				}
			}
			if i == 0 {
				h = n
			} else {
				m = n
			}
		}
	}
	return h*60 + m
}

// window returns the open span of a local day, in minutes from midnight.
func (h Week) window(day time.Time) (int, int, bool) {
	if !h.Enabled {
		return 0, 24 * 60, true
	}
	key := day.Format("2006-01-02")
	for _, d := range h.Holidays {
		if d == key {
			return 0, 0, false
		}
	}
	idx := (int(day.Weekday()) + 6) % 7 // Monday first
	d := h.Days[idx]
	if !d.Open {
		return 0, 0, false
	}
	from, to := MinuteOf(d.From), MinuteOf(d.To)
	if to <= from {
		return 0, 0, false
	}
	return from, to, true
}

// Open reports whether the device is within working hours at t.
func (h Week) Open(t time.Time) bool {
	lt := t.In(Zone)
	from, to, ok := h.window(lt)
	if !ok {
		return false
	}
	m := lt.Hour()*60 + lt.Minute()
	return m >= from && m < to
}

// Elapsed counts the working time between two moments, so a message that
// arrived at night starts counting in the morning.
func (h Week) Elapsed(from, to time.Time) time.Duration {
	if !to.After(from) {
		return 0
	}
	if !h.Enabled {
		return to.Sub(from)
	}
	var total time.Duration
	cur := from.In(Zone)
	end := to.In(Zone)
	for day := 0; day < 60 && cur.Before(end); day++ {
		midnight := time.Date(cur.Year(), cur.Month(), cur.Day(), 0, 0, 0, 0, Zone)
		next := midnight.AddDate(0, 0, 1)
		if f, t, ok := h.window(midnight); ok {
			open := midnight.Add(time.Duration(f) * time.Minute)
			close := midnight.Add(time.Duration(t) * time.Minute)
			a := maxTime(open, cur)
			b := minTime(close, end)
			if b.After(a) {
				total += b.Sub(a)
			}
		}
		cur = next
	}
	return total
}

func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

// Week is the device's working week and holidays.
type Week struct {
	Enabled  bool     `json:"enabled"`
	Days     [7]Day   `json:"days"` // Monday first
	Holidays []string `json:"holidays"`
}

// Day is one weekday's hours, "09:00" to "18:00".
type Day struct {
	Open bool   `json:"open"`
	From string `json:"from"`
	To   string `json:"to"`
}
