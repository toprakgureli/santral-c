// Package device holds one WhatsApp device's own settings: read receipts,
// the greeting, distribution, waiting time, working hours, the survey and
// the chatbot and opt-out words.
package device

import (
	"encoding/json"
	"strings"

	"github.com/toprakgureli/santral-c/backend/internal/whatsapp/hours"
)

// Settings is one device's own configuration. It is stored on the
// device, so every device keeps its own and a new one starts from the
// defaults below. Each section is guarded by its own permission.
type Settings struct {
	// Blue ticks towards the customer when an agent opens the chat.
	ReadReceipts bool `json:"readReceipts"`

	Greeting     Greeting     `json:"greeting"`
	Distribution Distribution `json:"distribution"`

	// Minutes a customer may wait for an answer before the ticket shows in
	// "Cevap Bekleyenler" for everyone. 0 turns the list off.
	WaitingMinutes int `json:"waitingMinutes"`

	Hours  hours.Week `json:"hours"`
	Survey Survey     `json:"survey"`

	// Minutes of silence after which a customer's chatbot session ends.
	BotTimeoutMinutes int `json:"botTimeoutMinutes"`
	// Words that take the customer from the chatbot to a person.
	HumanKeywords []string `json:"humanKeywords"`
	// Words with which a customer stops marketing messages.
	OptOutKeywords []string `json:"optOutKeywords"`
	OptOutReply    string   `json:"optOutReply"`
}

// Greeting is the "Karşıla" message an agent sends when they first
// take up a chat. {ad}, {unvan} and {musteri} are filled in.
type Greeting struct {
	Enabled bool   `json:"enabled"`
	Text    string `json:"text"`
	// Sent instead of the text when the 24-hour window is closed.
	Template     string `json:"template"`
	TemplateLang string `json:"templateLang"`
	// Whether an agent who joins to help also sends it.
	ForHelpers bool `json:"forHelpers"`
}

// Distribution controls the automatic hand-out of new tickets.
type Distribution struct {
	Enabled bool `json:"enabled"`
	// The most open tickets one agent holds at once; 0 means no limit.
	MaxOpen int `json:"maxOpen"`
}

// Survey is what is sent when a ticket is resolved.
type Survey struct {
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
	// A customer who got the survey is not asked again for this many
	// hours, however many conversations are closed meanwhile. 0 asks at
	// every close.
	RepeatHours int `json:"repeatHours"`
}

// Default is what a new device starts with.
func Default() Settings {
	s := Settings{
		ReadReceipts: true,
		Greeting: Greeting{
			Enabled: false,
			Text:    "Merhaba {musteri}, ben {unvan} {ad}. Sizinle ben ilgileniyorum.",
		},
		Distribution:      Distribution{Enabled: true},
		WaitingMinutes:    15,
		BotTimeoutMinutes: 30,
		HumanKeywords:     []string{"temsilci", "müşteri temsilcisi", "yetkili", "insan"},
		OptOutKeywords:    []string{"DUR", "STOP"},
		OptOutReply:       "Kampanya mesajlarımızı artık almayacaksınız. Destek için bize yazmaya devam edebilirsiniz.",
		Survey: Survey{
			Mode:        "off",
			Text:        "Görüşmemizi değerlendirir misiniz? {link}",
			AlertBelow:  2,
			RepeatHours: 24,
		},
	}
	for i := 0; i < 5; i++ {
		s.Hours.Days[i] = hours.Day{Open: true, From: "09:00", To: "18:00"}
	}
	s.Hours.Days[5] = hours.Day{Open: false, From: "10:00", To: "16:00"}
	s.Hours.Days[6] = hours.Day{Open: false, From: "10:00", To: "16:00"}
	return s
}

func Parse(raw string) Settings {
	s := Default()
	if strings.TrimSpace(raw) == "" || raw == "{}" {
		s.Normalize()
		return s
	}
	_ = json.Unmarshal([]byte(raw), &s)
	s.Normalize()
	return s
}

// Normalize keeps lists as lists, so the panel never gets null for them.
func (s *Settings) Normalize() {
	if s.Hours.Holidays == nil {
		s.Hours.Holidays = []string{}
	}
	if s.HumanKeywords == nil {
		s.HumanKeywords = []string{}
	}
	if s.OptOutKeywords == nil {
		s.OptOutKeywords = []string{}
	}
	if s.Survey.RepeatHours < 0 {
		s.Survey.RepeatHours = 0
	}
	if s.Survey.RepeatHours > 24*90 {
		s.Survey.RepeatHours = 24 * 90
	}
}

func (s Settings) Encode() string {
	b, _ := json.Marshal(s)
	return string(b)
}
