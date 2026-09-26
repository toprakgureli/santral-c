package whatsapp

import (
	"encoding/json"
	"strings"
	"time"
)

// istanbul is the panel's time zone (no daylight saving).
var istanbul = time.FixedZone("+03", 3*3600)

// ChannelSettings is one device's own configuration. It is stored on the
// device, so every device keeps its own and a new one starts from the
// defaults below. Each section is guarded by its own permission.
type ChannelSettings struct {
	// Blue ticks towards the customer when an agent opens the chat.
	ReadReceipts bool `json:"readReceipts"`

	Greeting     GreetingSettings     `json:"greeting"`
	Distribution DistributionSettings `json:"distribution"`

	// Minutes a customer may wait for an answer before the ticket shows in
	// "Cevap Bekleyenler" for everyone. 0 turns the list off.
	WaitingMinutes int `json:"waitingMinutes"`

	Hours  HoursSettings  `json:"hours"`
	Survey SurveySettings `json:"survey"`

	// Minutes of silence after which a customer's chatbot session ends.
	BotTimeoutMinutes int `json:"botTimeoutMinutes"`
	// Words that take the customer from the chatbot to a person.
	HumanKeywords []string `json:"humanKeywords"`
	// Words with which a customer stops marketing messages.
	OptOutKeywords []string `json:"optOutKeywords"`
	OptOutReply    string   `json:"optOutReply"`
}

// GreetingSettings is the "Karşıla" message an agent sends when they first
// take up a chat. {ad}, {unvan} and {musteri} are filled in.
type GreetingSettings struct {
	Enabled bool   `json:"enabled"`
	Text    string `json:"text"`
	// Sent instead of the text when the 24-hour window is closed.
	Template     string `json:"template"`
	TemplateLang string `json:"templateLang"`
	// Whether an agent who joins to help also sends it.
	ForHelpers bool `json:"forHelpers"`
}

// DistributionSettings controls the automatic hand-out of new tickets.
type DistributionSettings struct {
	Enabled bool `json:"enabled"`
	// The most open tickets one agent holds at once; 0 means no limit.
	MaxOpen int `json:"maxOpen"`
}

// HoursSettings is the device's working week and holidays.
type HoursSettings struct {
	Enabled  bool       `json:"enabled"`
	Days     [7]DayOpen `json:"days"` // Monday first
	Holidays []string   `json:"holidays"`
}

// DayOpen is one weekday's hours, "09:00" to "18:00".
type DayOpen struct {
	Open bool   `json:"open"`
	From string `json:"from"`
	To   string `json:"to"`
}

// SurveySettings is what is sent when a ticket is resolved.
type SurveySettings struct {
	Mode string `json:"mode"` // off | tally | native
	// Tally form link; ticket, agent and device go in as hidden fields.
	URL string `json:"url"`
	// Tally signing secret, stored encrypted.
	SecretEnc string `json:"secretEnc,omitempty"`
	Text      string `json:"text"`
	// Sent instead of the text when the 24-hour window is closed.
	Template     string `json:"template"`
	TemplateLang string `json:"templateLang"`
	// A low score (this or below) alerts the managers.
	AlertBelow int `json:"alertBelow"`
}

// defaultSettings is what a new device starts with.
func defaultSettings() ChannelSettings {
	s := ChannelSettings{
		ReadReceipts: true,
		Greeting: GreetingSettings{
			Enabled: false,
			Text:    "Merhaba {musteri}, ben {unvan} {ad}. Sizinle ben ilgileniyorum.",
		},
		Distribution:      DistributionSettings{Enabled: true},
		WaitingMinutes:    15,
		BotTimeoutMinutes: 30,
		HumanKeywords:     []string{"temsilci", "müşteri temsilcisi", "yetkili", "insan"},
		OptOutKeywords:    []string{"DUR", "STOP"},
		OptOutReply:       "Kampanya mesajlarımızı artık almayacaksınız. Destek için bize yazmaya devam edebilirsiniz.",
		Survey: SurveySettings{
			Mode:       "off",
			Text:       "Görüşmemizi değerlendirir misiniz? {link}",
			AlertBelow: 2,
		},
	}
	for i := 0; i < 5; i++ {
		s.Hours.Days[i] = DayOpen{Open: true, From: "09:00", To: "18:00"}
	}
	s.Hours.Days[5] = DayOpen{Open: false, From: "10:00", To: "16:00"}
	s.Hours.Days[6] = DayOpen{Open: false, From: "10:00", To: "16:00"}
	return s
}

func parseSettings(raw string) ChannelSettings {
	s := defaultSettings()
	if strings.TrimSpace(raw) == "" || raw == "{}" {
		return s
	}
	_ = json.Unmarshal([]byte(raw), &s)
	return s
}

func (s ChannelSettings) encode() string {
	b, _ := json.Marshal(s)
	return string(b)
}

// ---------------------------------------------------------------- hours

func minuteOf(hhmm string) int {
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
func (h HoursSettings) window(day time.Time) (int, int, bool) {
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
	from, to := minuteOf(d.From), minuteOf(d.To)
	if to <= from {
		return 0, 0, false
	}
	return from, to, true
}

// Open reports whether the device is within working hours at t.
func (h HoursSettings) Open(t time.Time) bool {
	lt := t.In(istanbul)
	from, to, ok := h.window(lt)
	if !ok {
		return false
	}
	m := lt.Hour()*60 + lt.Minute()
	return m >= from && m < to
}

// Elapsed counts the working time between two moments, so a message that
// arrived at night starts counting in the morning.
func (h HoursSettings) Elapsed(from, to time.Time) time.Duration {
	if !to.After(from) {
		return 0
	}
	if !h.Enabled {
		return to.Sub(from)
	}
	var total time.Duration
	cur := from.In(istanbul)
	end := to.In(istanbul)
	for day := 0; day < 60 && cur.Before(end); day++ {
		midnight := time.Date(cur.Year(), cur.Month(), cur.Day(), 0, 0, 0, 0, istanbul)
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
